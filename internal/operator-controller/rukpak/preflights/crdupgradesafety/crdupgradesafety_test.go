package crdupgradesafety

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCategorizeValidationError_SpecificMessages(t *testing.T) {
	tests := []struct {
		name           string
		errStr         string
		context        string
		expectedOutput string
	}{
		{
			name:           "Required field addition",
			errStr:         "required field 'newField' added to schema",
			context:        "v1beta1.spec",
			expectedOutput: "Required field added (v1beta1.spec): Make the new field optional or provide a default. Required-field additions break existing CRs and are rejected by OLM's safety check.",
		},
		{
			name:           "Field removal",
			errStr:         "existing field 'oldField' removed from schema",
			context:        "argocds.argoproj.io",
			expectedOutput: "Field removal detected (argocds.argoproj.io): The OLM preflight blocked our CRD update because it isn't backwards-compatible. Please rework the change to be additive: avoid removing fields.",
		},
		{
			name:           "Enum constraint tightening",
			errStr:         "enum constraint tightened from [a,b,c] to [a,b]",
			context:        "v1.spec.mode",
			expectedOutput: "Enum restriction tightened (v1.spec.mode): a,b,c → a,b. Avoid narrowing enums; only additive relaxations are allowed.",
		},
		{
			name:           "Default value changed",
			errStr:         "default value changed from 'false' to 'true'",
			context:        "applications.argoproj.io.spec.syncPolicy",
			expectedOutput: "Default changed (applications.argoproj.io.spec.syncPolicy): false → true. Keep the old default, or introduce the new behavior via a new field or version.",
		},
		{
			name:           "Type changed",
			errStr:         "field type changed from string to integer",
			context:        "v1alpha1.status.replicas",
			expectedOutput: "Type changed (v1alpha1.status.replicas): string → integer. The OLM preflight blocked our CRD update because it isn't backwards-compatible. Don't change types in place - add a new CRD version instead.",
		},
		{
			name:           "Version removal",
			errStr:         "stored version v1alpha1 removed",
			context:        "prometheuses.monitoring.coreos.com",
			expectedOutput: "Version removal/scope change (prometheuses.monitoring.coreos.com): stored version removed (v1alpha1)",
		},
		{
			name:           "Generic compatibility issue",
			errStr:         "some other backwards compatibility failure",
			context:        "custom.example.com",
			expectedOutput: "Backwards-compatibility issue (custom.example.com): The OLM preflight blocked our CRD update because it isn't backwards-compatible. Please rework the change to be additive: avoid removing or tightening fields, don't change types in place, and keep defaults stable.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := categorizeValidationError(tc.errStr, tc.context)

			// Print the result so we can see the actual output
			t.Logf("Error Type: %s", tc.name)
			t.Logf("Input Error: %s", tc.errStr)
			t.Logf("Context: %s", tc.context)
			t.Logf("Output Message: %s", result)
			t.Logf("Length: %d characters", len(result))
			t.Log("---")

			require.Equal(t, tc.expectedOutput, result)
		})
	}
}

func TestSummarizeValidationFailures_RealCRDScenarios(t *testing.T) {
	tests := []struct {
		name           string
		setupFunc      func() string
		expectedOutput string
	}{
		{
			name: "ArgoCD CRD field removal scenario",
			setupFunc: func() string {
				// Simulate what happens when ArgoCD removes a field
				summary := newPreflightSummary()
				summary.AddCriticalIssue("Field removal detected (argocds.argoproj.io): The OLM preflight blocked our CRD update because it isn't backwards-compatible. Please rework the change to be additive: avoid removing fields.")
				summary.AddCriticalIssue("Required field added (argocds.argoproj.io): Make the new field optional or provide a default. Required-field additions break existing CRs and are rejected by OLM's safety check.")
				summary.AddBreakingIssue("Type changed (applications.argoproj.io): The OLM preflight blocked our CRD update because it isn't backwards-compatible. Don't change types in place - add a new CRD version instead.")
				return summary.GenerateMessage()
			},
			expectedOutput: `CRD Upgrade Safety
Total: 3 (2 critical, 1 breaking)
Issues:
- Field removal detected (argocds.argoproj.io): The OLM preflight blocked our CRD update because it isn't backwards-compatible. Please rework the change to be additive: avoid removing fields.
- Required field added (argocds.argoproj.io): Make the new field optional or provide a default. Required-field additions break existing CRs and are rejected by OLM's safety check.
- Type changed (applications.argoproj.io): The OLM preflight blocked our CRD update because it isn't backwards-compatible. Don't change types in place - add a new CRD version instead.`,
		},
		{
			name: "Prometheus CRD enum tightening scenario",
			setupFunc: func() string {
				summary := newPreflightSummary()
				summary.AddBreakingIssue("Enum/range constraint tightened (prometheuses.monitoring.coreos.com): Please avoid narrowing enums or ranges; only additive relaxations are allowed (add enum values, lower mins, raise maxes).")
				summary.AddBreakingIssue("Default changed (servicemonitors.monitoring.coreos.com): Changing defaults is flagged by the preflight. Keep the old default, or introduce the new behavior via a new field or version.")
				return summary.GenerateMessage()
			},
			expectedOutput: `CRD Upgrade Safety
Total: 2 (2 breaking)
Issues:
- Enum/range constraint tightened (prometheuses.monitoring.coreos.com): Please avoid narrowing enums or ranges; only additive relaxations are allowed (add enum values, lower mins, raise maxes).
- Default changed (servicemonitors.monitoring.coreos.com): Changing defaults is flagged by the preflight. Keep the old default, or introduce the new behavior via a new field or version.`,
		},
		{
			name: "Version removal critical scenario",
			setupFunc: func() string {
				summary := newPreflightSummary()
				summary.AddCriticalIssue("Version removal/scope change (operators.coreos.com): We can't remove a stored/served version or change CRD scope; Kubernetes blocks that at the API level. We need a migrate-first plan.")
				return summary.GenerateMessage()
			},
			expectedOutput: `CRD Upgrade Safety
Total: 1 (1 critical)
Issues:
- Version removal/scope change (operators.coreos.com): We can't remove a stored/served version or change CRD scope; Kubernetes blocks that at the API level. We need a migrate-first plan.`,
		},
		{
			name: "No validation results - unknown failure",
			setupFunc: func() string {
				return summarizeValidationFailures(nil)
			},
			expectedOutput: "The OLM preflight blocked our CRD update because it isn't backwards-compatible. Please rework the change to be additive: avoid removing or tightening fields, don't change types in place, and keep defaults stable. If this is a semantic change, add a new CRD version and keep the old one served until we migrate.",
		},
		{
			name: "Mixed CRD validation issues scenario",
			setupFunc: func() string {
				summary := newPreflightSummary()
				summary.AddCriticalIssue("Field removal detected (custom-operators.example.com): The OLM preflight blocked our CRD update because it isn't backwards-compatible. Please rework the change to be additive: avoid removing fields.")
				summary.AddCriticalIssue("Required field added (custom-operators.example.com): Make the new field optional or provide a default. Required-field additions break existing CRs and are rejected by OLM's safety check.")
				summary.AddBreakingIssue("Type changed (custom-operators.example.com): The OLM preflight blocked our CRD update because it isn't backwards-compatible. Don't change types in place - add a new CRD version instead.")
				summary.AddBreakingIssue("Default changed (custom-operators.example.com): Changing defaults is flagged by the preflight. Keep the old default, or introduce the new behavior via a new field or version.")
				summary.AddBreakingIssue("Enum/range constraint tightened (custom-operators.example.com): Please avoid narrowing enums or ranges; only additive relaxations are allowed (add enum values, lower mins, raise maxes).")
				return summary.GenerateMessage()
			},
			expectedOutput: `CRD Upgrade Safety
Total: 5 (2 critical, 3 breaking)
Issues:
- Field removal detected (custom-operators.example.com): The OLM preflight blocked our CRD update because it isn't backwards-compatible. Please rework the change to be additive: avoid removing fields.
- Required field added (custom-operators.example.com): Make the new field optional or provide a default. Required-field additions break existing CRs and are rejected by OLM's safety check.
- Type changed (custom-operators.example.com): The OLM preflight blocked our CRD update because it isn't backwards-compatible. Don't change types in place - add a new CRD version instead.
- Default changed (custom-operators.example.com): Changing defaults is flagged by the preflight. Keep the old default, or introduce the new behavior via a new field or version.
- Enum/range constraint tightened (custom-operators.example.com): Please avoid narrowing enums or ranges; only additive relaxations are allowed (add enum values, lower mins, raise maxes).`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.setupFunc()

			// Print the result so we can see the actual output
			t.Logf("Scenario: %s", tc.name)
			t.Logf("Output: %s", result)
			t.Logf("Length: %d characters", len(result))
			t.Log("---")

			require.Equal(t, tc.expectedOutput, result)
		})
	}
}

func TestDetermineErrorSeverity(t *testing.T) {
	tests := []struct {
		name             string
		errStr           string
		expectedSeverity string
	}{
		{
			name:             "Critical - field removal",
			errStr:           "existing field 'spec.route' removed from schema",
			expectedSeverity: "critical",
		},
		{
			name:             "Critical - required field",
			errStr:           "required field 'spec.mandatory' added to schema",
			expectedSeverity: "critical",
		},
		{
			name:             "Critical - version removal",
			errStr:           "stored version v1alpha1 removed",
			expectedSeverity: "critical",
		},
		{
			name:             "Breaking - type change",
			errStr:           "field type changed from string to integer",
			expectedSeverity: "breaking",
		},
		{
			name:             "Breaking - enum constraint",
			errStr:           "enum constraint tightened from [a,b,c] to [a,b]",
			expectedSeverity: "breaking",
		},
		{
			name:             "Breaking - default change",
			errStr:           "default value changed from false to true",
			expectedSeverity: "breaking",
		},
		{
			name:             "Minor - description change",
			errStr:           "field description updated",
			expectedSeverity: "minor",
		},
		{
			name:             "Minor - unknown issue",
			errStr:           "some other validation issue",
			expectedSeverity: "minor",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := determineErrorSeverity(tc.errStr)

			t.Logf("Error: %s", tc.errStr)
			t.Logf("Expected Severity: %s", tc.expectedSeverity)
			t.Logf("Actual Severity: %s", result)

			require.Equal(t, tc.expectedSeverity, result)
		})
	}
}

func TestCRDUpgradeSafetyMessages_LengthValidation(t *testing.T) {
	// Test that all our generated messages are within reasonable bounds
	// Even the longest specific error messages should be much shorter than the generic dumps

	testCases := []string{
		"existing field 'spec.very.long.field.path.that.could.be.nested.deeply.in.the.schema.structure' removed from schema",
		"required field 'spec.another.extremely.long.field.name.that.represents.a.complex.configuration.option' added to schema",
		"field type changed from string to object in spec.source.helm.values.with.a.very.long.path.name",
		"default value changed from 'previous-very-long-default-value' to 'new-very-long-default-value' in spec.configuration.advanced.settings",
		"enum constraint tightened from [VeryLongEnumValue1,VeryLongEnumValue2,VeryLongEnumValue3] to [VeryLongEnumValue1] in spec.mode.selection",
		"stored version v1alpha1-with-long-version-name removed from custom-resource-definition.with.very.long.name.example.com",
	}

	for i, errStr := range testCases {
		t.Run(fmt.Sprintf("LongMessage_%d", i+1), func(t *testing.T) {
			context := "very.long.crd.name.with.multiple.segments.example.com"
			result := categorizeValidationError(errStr, context)

			t.Logf("Input: %s", errStr)
			t.Logf("Output: %s", result)
			t.Logf("Length: %d characters", len(result))

			// Even our longest specific messages should be reasonable
			require.Less(t, len(result), 500, "Specific error message should be concise")
			require.Greater(t, len(result), 50, "Message should be informative")
		})
	}
}
