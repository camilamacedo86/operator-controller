package crdupgradesafety

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"helm.sh/helm/v3/pkg/release"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1client "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/crdify/pkg/config"
	"sigs.k8s.io/crdify/pkg/runner"
	"sigs.k8s.io/crdify/pkg/validations"
	"sigs.k8s.io/crdify/pkg/validations/property"

	"github.com/operator-framework/operator-controller/internal/operator-controller/rukpak/util"
)

// Local summary type used only by CRD upgrade safety
type preflightMessageSummary struct {
	CheckName     string
	Issues        []string
	CriticalCount int
	BreakingCount int
}

func newPreflightSummary() *preflightMessageSummary {
	return &preflightMessageSummary{CheckName: "CRD Upgrade Safety"}
}

func (s *preflightMessageSummary) AddCriticalIssue(issue string) {
	s.CriticalCount++
	s.Issues = append(s.Issues, issue)
}

func (s *preflightMessageSummary) AddBreakingIssue(issue string) {
	s.BreakingCount++
	s.Issues = append(s.Issues, issue)
}

func (s *preflightMessageSummary) AddIssue(issue string) {
	// non-blocking (minor) still listed but not counted in totals
	s.Issues = append(s.Issues, issue)
}

func (s *preflightMessageSummary) GenerateMessage() string {
	total := s.CriticalCount + s.BreakingCount
	if total == 0 {
		return fmt.Sprintf("%s\nTotal: 0\nIssues: none", s.CheckName)
	}
	header := fmt.Sprintf("%s\nTotal: %d", s.CheckName, total)
	parts := []string{}
	if s.CriticalCount > 0 {
		parts = append(parts, fmt.Sprintf("%d critical", s.CriticalCount))
	}
	if s.BreakingCount > 0 {
		parts = append(parts, fmt.Sprintf("%d breaking", s.BreakingCount))
	}
	if len(parts) > 0 {
		header = fmt.Sprintf("%s (%s)", header, strings.Join(parts, ", "))
	}
	bullets := make([]string, 0, len(s.Issues))
	for _, issue := range s.Issues {
		bullets = append(bullets, "- "+issue)
	}
	return fmt.Sprintf("%s\nIssues:\n%s", header, strings.Join(bullets, "\n"))
}

// precompiled patterns to extract from → to details where available
var (
	reFromToType    = regexp.MustCompile(`type changed from (\S+) to (\S+)`)
	reDefaultChange = regexp.MustCompile(`default value changed from '([^']*)' to '([^']*)'`)
	reEnumTight     = regexp.MustCompile(`enum constraint tightened.* from \[([^\]]*)\] to \[([^\]]*)\]`)
	reScopeChange   = regexp.MustCompile(`scope changed from "([^"]+)" to "([^"]+)"`)
	reMinIncreased  = regexp.MustCompile(`minimum value.* increased from ([^ ]+) to ([^ ]+)`)
	reMaxDecreased  = regexp.MustCompile(`maximum value.* decreased from ([^ ]+) to ([^ ]+)`)
	reStoredRemoved = regexp.MustCompile(`stored version "?([^" ]+)"? removed`)
)

func arrow(from, to string) string { return fmt.Sprintf("%s → %s", from, to) }

type Option func(p *Preflight)

func WithConfig(cfg *config.Config) Option {
	return func(p *Preflight) {
		p.config = cfg
	}
}

func WithRegistry(reg validations.Registry) Option {
	return func(p *Preflight) {
		p.registry = reg
	}
}

type Preflight struct {
	crdClient apiextensionsv1client.CustomResourceDefinitionInterface
	config    *config.Config
	registry  validations.Registry
}

func NewPreflight(crdCli apiextensionsv1client.CustomResourceDefinitionInterface, opts ...Option) *Preflight {
	p := &Preflight{
		crdClient: crdCli,
		config:    defaultConfig(),
		registry:  defaultRegistry(),
	}

	for _, o := range opts {
		o(p)
	}

	return p
}

func (p *Preflight) Install(ctx context.Context, rel *release.Release) error {
	return p.runPreflight(ctx, rel)
}

func (p *Preflight) Upgrade(ctx context.Context, rel *release.Release) error {
	return p.runPreflight(ctx, rel)
}

func (p *Preflight) runPreflight(ctx context.Context, rel *release.Release) error {
	if rel == nil {
		return nil
	}

	relObjects, err := util.ManifestObjects(strings.NewReader(rel.Manifest), fmt.Sprintf("%s-release-manifest", rel.Name))
	if err != nil {
		return fmt.Errorf("parsing release %q objects: %w", rel.Name, err)
	}

	runner, err := runner.New(p.config, p.registry)
	if err != nil {
		return fmt.Errorf("creating CRD validation runner: %w", err)
	}

	validateErrors := make([]error, 0, len(relObjects))
	for _, obj := range relObjects {
		if obj.GetObjectKind().GroupVersionKind() != apiextensionsv1.SchemeGroupVersion.WithKind("CustomResourceDefinition") {
			continue
		}

		newCrd := &apiextensionsv1.CustomResourceDefinition{}
		uMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return fmt.Errorf("converting object %q to unstructured: %w", obj.GetName(), err)
		}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(uMap, newCrd)
		if err != nil {
			return fmt.Errorf("converting unstructured to CRD object: %w", err)
		}

		oldCrd, err := p.crdClient.Get(ctx, newCrd.Name, metav1.GetOptions{})
		if err != nil {
			// if there is no existing CRD, there is nothing to break
			// so it is immediately successful.
			if apierrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("getting existing resource for CRD %q: %w", newCrd.Name, err)
		}

		results := runner.Run(oldCrd, newCrd)
		if results.HasFailures() {
			summary := summarizeValidationFailures(results)
			validateErrors = append(validateErrors, fmt.Errorf("CRD %q upgrade blocked: %s", newCrd.Name, summary))
		}
	}

	return errors.Join(validateErrors...)
}

func defaultConfig() *config.Config {
	return &config.Config{
		// Ignore served version validations if conversion policy is set.
		Conversion: config.ConversionPolicyIgnore,
		// Fail-closed by default
		UnhandledEnforcement: config.EnforcementPolicyError,
		// Use the default validation configurations as they are
		// the strictest possible.
		Validations: []config.ValidationConfig{
			// Do not enforce the description validation
			// because OLM should not block on field description changes.
			{
				Name:        "description",
				Enforcement: config.EnforcementPolicyNone,
			},
			{
				Name:        "enum",
				Enforcement: config.EnforcementPolicyError,
				Configuration: map[string]interface{}{
					"additionPolicy": property.AdditionPolicyAllow,
				},
			},
		},
	}
}

func defaultRegistry() validations.Registry {
	return runner.DefaultRegistry()
}

// summarizeValidationFailures creates a concise, meaningful summary of CRD validation failures
func summarizeValidationFailures(results *runner.Results) string {
	if results == nil {
		return "The OLM preflight blocked our CRD update because it isn't backwards-compatible. Please rework the change to be additive: avoid removing or tightening fields, don't change types in place, and keep defaults stable. If this is a semantic change, add a new CRD version and keep the old one served until we migrate."
	}

	summary := newPreflightSummary()

	// Process CRD-wide validation errors
	for _, result := range results.CRDValidation {
		for _, err := range result.Errors {
			addCategorizedIssue(summary, err, result.Name)
		}
	}

	// Process same version errors (breaking changes)
	for version, propertyResults := range results.SameVersionValidation {
		for property, comparisonResults := range propertyResults {
			for _, result := range comparisonResults {
				for _, err := range result.Errors {
					context := fmt.Sprintf("%s.%s.%s", version, property, result.Name)
					addCategorizedIssue(summary, err, context)
				}
			}
		}
	}

	// Process served version errors
	for _, propertyResults := range results.ServedVersionValidation {
		for _, comparisonResults := range propertyResults {
			for _, result := range comparisonResults {
				for _, err := range result.Errors {
					context := fmt.Sprintf("served version: %s", result.Name)
					addCategorizedIssue(summary, err, context)
				}
			}
		}
	}

	// Compute per-severity counts from finalized issue messages
	var criticalCount, breakingCount, otherCount int
	for _, issue := range summary.Issues {
		l := strings.ToLower(issue)
		switch {
		// critical
		case strings.HasPrefix(l, "field removal detected"):
			criticalCount++
		case strings.HasPrefix(l, "required field added"):
			criticalCount++
		case strings.HasPrefix(l, "version removal/scope change"):
			criticalCount++
		// breaking
		case strings.HasPrefix(l, "type changed"):
			breakingCount++
		case strings.HasPrefix(l, "enum restriction tightened"), strings.HasPrefix(l, "enum restriction added"):
			breakingCount++
		case strings.HasPrefix(l, "default changed"), strings.HasPrefix(l, "default added"), strings.HasPrefix(l, "default removed"):
			breakingCount++
		case strings.HasPrefix(l, "minimum increased"), strings.HasPrefix(l, "maximum decreased"), strings.HasPrefix(l, "constraint added"):
			breakingCount++
		default:
			otherCount++
		}
	}
	total := len(summary.Issues)
	if total == 0 {
		return "CRD Upgrade Safety\nTotal: 0\nIssues: none"
	}
	parts := []string{}
	if criticalCount > 0 {
		parts = append(parts, fmt.Sprintf("%d critical", criticalCount))
	}
	if breakingCount > 0 {
		parts = append(parts, fmt.Sprintf("%d breaking", breakingCount))
	}
	if otherCount > 0 {
		parts = append(parts, fmt.Sprintf("%d other", otherCount))
	}
	header := fmt.Sprintf("CRD Upgrade Safety\nTotal: %d (%s)", total, strings.Join(parts, ", "))
	bullets := make([]string, 0, total)
	for _, issue := range summary.Issues {
		bullets = append(bullets, "- "+issue)
	}
	return fmt.Sprintf("%s\nIssues:\n%s", header, strings.Join(bullets, "\n"))
}

// addCategorizedIssue categorizes an error and adds it to the summary with appropriate severity
func addCategorizedIssue(summary *preflightMessageSummary, errStr, context string) {
	message := categorizeValidationError(errStr, context)
	summary.AddIssue(message)
}

// determineErrorSeverity determines if an error is critical, breaking, or minor based on its content
func determineErrorSeverity(errStr string) string {
	lowerErr := strings.ToLower(errStr)
	// Critical: deletions/required/version/scope
	if strings.Contains(lowerErr, "existing field") && strings.Contains(lowerErr, "removed") {
		return "critical"
	}
	if strings.Contains(lowerErr, "required") && (strings.Contains(lowerErr, "added") || strings.Contains(lowerErr, "new")) {
		return "critical"
	}
	if strings.Contains(lowerErr, "stored version") && strings.Contains(lowerErr, "removed") {
		return "critical"
	}
	if strings.Contains(lowerErr, "served version") && strings.Contains(lowerErr, "removed") {
		return "critical"
	}
	if strings.Contains(lowerErr, "scope changed") {
		return "critical"
	}
	// Breaking: type/enums/defaults/constraints
	if strings.Contains(lowerErr, "type changed") || (strings.Contains(lowerErr, "type") && strings.Contains(lowerErr, "changed")) {
		return "breaking"
	}
	if strings.Contains(lowerErr, "enum constraint tightened") || (strings.Contains(lowerErr, "enum") && (strings.Contains(lowerErr, "removed") || strings.Contains(lowerErr, "restricted"))) {
		return "breaking"
	}
	if strings.Contains(lowerErr, "default value changed") || (strings.Contains(lowerErr, "default") && (strings.Contains(lowerErr, "changed") || strings.Contains(lowerErr, "added") || strings.Contains(lowerErr, "removed"))) {
		return "breaking"
	}
	if (strings.Contains(lowerErr, "minimum") || strings.Contains(lowerErr, "maximum") || strings.Contains(lowerErr, "minlength") || strings.Contains(lowerErr, "maxlength") || strings.Contains(lowerErr, "minitems") || strings.Contains(lowerErr, "maxitems")) && (strings.Contains(lowerErr, "increased") || strings.Contains(lowerErr, "decreased") || strings.Contains(lowerErr, "added")) {
		return "breaking"
	}
	return "minor"
}

// categorizeValidationError provides specific, actionable messages based on the type of CRD validation failure
func categorizeValidationError(errStr, context string) string {
	lowerErr := strings.ToLower(errStr)

	// Version/scope changes (check first to avoid false matches)
	if m := reStoredRemoved.FindStringSubmatch(lowerErr); len(m) == 2 {
		return fmt.Sprintf("Version removal/scope change (%s): stored version removed (%s)", context, m[1])
	}
	if m := reScopeChange.FindStringSubmatch(lowerErr); len(m) == 3 {
		return fmt.Sprintf("Version removal/scope change (%s): scope %s", context, arrow(m[1], m[2]))
	}
	if (strings.Contains(lowerErr, "stored version") || strings.Contains(lowerErr, "served version")) && strings.Contains(lowerErr, "removed") {
		return fmt.Sprintf("Version removal/scope change (%s): stored/served version removed", context)
	}

	// Required field addition
	if strings.Contains(lowerErr, "required") && (strings.Contains(lowerErr, "added") || strings.Contains(lowerErr, "new")) {
		return fmt.Sprintf("Required field added (%s): Make the new field optional or provide a default. Required-field additions break existing CRs and are rejected by OLM's safety check.", context)
	}

	// Field removal
	if strings.Contains(lowerErr, "removal") || strings.Contains(lowerErr, "removed") || strings.Contains(lowerErr, "existing field") {
		return fmt.Sprintf("Field removal detected (%s): The OLM preflight blocked our CRD update because it isn't backwards-compatible. Please rework the change to be additive: avoid removing fields.", context)
	}

	// Enum/range tightening
	if m := reEnumTight.FindStringSubmatch(lowerErr); len(m) == 3 {
		return fmt.Sprintf("Enum restriction tightened (%s): %s. Avoid narrowing enums; only additive relaxations are allowed.", context, arrow(m[1], m[2]))
	}
	if strings.Contains(lowerErr, "enum values removed") || (strings.Contains(lowerErr, "enum") && strings.Contains(lowerErr, "removed")) || strings.Contains(lowerErr, "enum restriction added") {
		return fmt.Sprintf("Enum restriction added (%s): Avoid adding new enum restrictions or removing existing enum values.", context)
	}

	// Default value changes
	if m := reDefaultChange.FindStringSubmatch(lowerErr); len(m) == 3 {
		return fmt.Sprintf("Default changed (%s): %s. Keep the old default, or introduce the new behavior via a new field or version.", context, arrow(m[1], m[2]))
	}
	if (strings.Contains(lowerErr, "default") && strings.Contains(lowerErr, "added")) || strings.Contains(lowerErr, "default value added") {
		return fmt.Sprintf("Default added (%s): Adding a new default may change existing behavior. Prefer introducing a new field or version.", context)
	}
	if (strings.Contains(lowerErr, "default") && strings.Contains(lowerErr, "removed")) || strings.Contains(lowerErr, "default value removed") {
		return fmt.Sprintf("Default removed (%s): Removing a default may break existing behavior. Keep the existing default or introduce a new field.", context)
	}

	// Type changes
	if m := reFromToType.FindStringSubmatch(lowerErr); len(m) == 3 {
		return fmt.Sprintf("Type changed (%s): %s. The OLM preflight blocked our CRD update because it isn't backwards-compatible. Don't change types in place - add a new CRD version instead.", context, arrow(m[1], m[2]))
	}
	if strings.Contains(lowerErr, "type") && (strings.Contains(lowerErr, "changed") || strings.Contains(lowerErr, "different")) {
		return fmt.Sprintf("Type changed (%s): The OLM preflight blocked our CRD update because it isn't backwards-compatible. Don't change types in place - add a new CRD version instead.", context)
	}

	// Numeric constraints
	if m := reMinIncreased.FindStringSubmatch(lowerErr); len(m) == 3 {
		return fmt.Sprintf("Minimum increased (%s): %s. Increasing minimums is prohibited; only decreases are allowed.", context, arrow(m[1], m[2]))
	}
	if m := reMaxDecreased.FindStringSubmatch(lowerErr); len(m) == 3 {
		return fmt.Sprintf("Maximum decreased (%s): %s. Decreasing maximums is prohibited; only increases are allowed.", context, arrow(m[1], m[2]))
	}
	if (strings.Contains(lowerErr, "minimum") || strings.Contains(lowerErr, "maximum") || strings.Contains(lowerErr, "minlength") || strings.Contains(lowerErr, "maxlength") || strings.Contains(lowerErr, "minitems") || strings.Contains(lowerErr, "maxitems")) && strings.Contains(lowerErr, "added") {
		return fmt.Sprintf("Constraint added (%s): Adding min/max constraints to previously unconstrained fields is prohibited.", context)
	}

	// Generic backwards-compatibility failure
	return fmt.Sprintf("Backwards-compatibility issue (%s): The OLM preflight blocked our CRD update because it isn't backwards-compatible. Please rework the change to be additive: avoid removing or tightening fields, don't change types in place, and keep defaults stable.", context)
}
