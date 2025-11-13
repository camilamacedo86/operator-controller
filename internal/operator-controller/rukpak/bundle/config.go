// Package bundle validates configuration for different bundle types.
//
// How it works:
//
// Each bundle type (like registry+v1 or Helm) knows what configuration it accepts.
// When a user provides configuration, we validate it before creating a Config object.
// Once created, a Config is guaranteed to be valid - you never need to check it again.
//
// The validation uses JSON Schema:
//  1. Bundle provides its schema (what config is valid)
//  2. We validate the user's config against that schema
//  3. If valid, we create a Config object
//  4. If invalid, we return a helpful error message
//
// Design choices:
//
//   - Validation happens once, when creating the Config. There's no Validate() method
//     because once you have a Config, it's already been validated.
//
//   - Config doesn't hold onto the schema. We only need the schema during validation,
//     not after the Config is created.
//
//   - You can't create a Config directly. You must go through UnmarshalConfig so that
//     validation always happens.
package bundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"sigs.k8s.io/yaml"
)

const (
	// configSchemaID is a name we use to identify the config schema when compiling it.
	// Think of it like a file name - it just needs to be consistent.
	configSchemaID = "config-schema.json"

	// OwnNamespace mode: watchNamespace must equal install namespace
	formatOwnNamespaceInstallMode = "ownNamespaceInstallMode"
	// SingleNamespace mode: watchNamespace must differ from install namespace
	formatSingleNamespaceInstallMode = "singleNamespaceInstallMode"
)

// ConfigSchemaProvider lets each bundle type describe what configuration it accepts.
//
// Different bundle types provide schemas in different ways:
//   - registry+v1: builds schema from the operator's install modes
//   - Helm: reads schema from values.schema.json in the chart
//   - registry+v2: (future) will have its own way
//
// Note: We don't store this in the Config struct. We only need it when validating
// the user's input. After that, the Config has the validated data and doesn't need
// the schema anymore.
type ConfigSchemaProvider interface {
	// Get returns a JSON Schema describing what configuration is valid.
	// Returns nil if this bundle type doesn't need configuration validation.
	Get() (map[string]any, error)
}

// Config holds validated configuration data from a bundle.
//
// Different bundle types have different configuration options, so we store
// the data in a flexible format and provide accessor methods to get values out.
//
// Why there's no Validate() method:
// We validate configuration when creating a Config. If you have a Config object,
// it's already been validated - you don't need to check it again. The unexported
// 'data' field means you can't create a Config yourself; you have to use
// UnmarshalConfig, which does the validation.
type Config struct {
	data map[string]any
}

// newConfig creates a Config from already-validated data.
// This is unexported so all Configs must be created through UnmarshalConfig,
// which ensures validation always happens.
func newConfig(data map[string]any) *Config {
	return &Config{data: data}
}

// GetWatchNamespace returns the watchNamespace value if present in the configuration.
// Returns nil if watchNamespace is not set or is explicitly set to null.
func (c *Config) GetWatchNamespace() *string {
	if c == nil || c.data == nil {
		return nil
	}
	val, exists := c.data["watchNamespace"]
	if !exists {
		return nil
	}
	// Handle explicit null
	if val == nil {
		return nil
	}
	// Extract string value
	if str, ok := val.(string); ok {
		return &str
	}
	// This handles cases where schema validation wasn't applied
	str := fmt.Sprintf("%v", val)
	return &str
}

// UnmarshalConfig takes user configuration, validates it, and creates a Config object.
// This is the only way to create a Config.
//
// What it does:
//  1. Checks the user's configuration against the schema (if provided)
//  2. If valid, creates a Config object
//  3. If invalid, returns an error explaining what's wrong
//
// Parameters:
//   - bytes: the user's configuration in YAML or JSON. If nil, we treat it as empty ({})
//   - schema: describes what configuration is valid. If nil, we skip validation
//   - installNamespace: the namespace where the operator will be installed. We use this
//     to validate namespace constraints (e.g., OwnNamespace mode requires watchNamespace
//     to equal installNamespace)
//
// If the user doesn't provide any configuration but the bundle requires some fields
// (like watchNamespace), validation will fail with a helpful error.
func UnmarshalConfig(bytes []byte, schema map[string]any, installNamespace string) (*Config, error) {
	// nil config becomes {} so we can validate required fields
	if bytes == nil {
		bytes = []byte("{}")
	}

	// Step 1: Validate against the schema if provided
	if schema != nil {
		if err := validateConfigWithSchema(bytes, schema, installNamespace); err != nil {
			return nil, fmt.Errorf("invalid configuration: %w", err)
		}
	}

	// Step 2: Parse into Config struct
	// We use yaml.Unmarshal to parse the validated config into an opaque map.
	// Schema validation has already ensured correctness.
	var configData map[string]any
	if err := yaml.Unmarshal(bytes, &configData); err != nil {
		return nil, fmt.Errorf("error unmarshalling configuration: %w", formatUnmarshalError(err))
	}

	return newConfig(configData), nil
}

// validateConfigWithSchema checks if the user's config matches the schema.
//
// We create a fresh validator each time because the namespace constraints depend on
// which namespace this specific ClusterExtension is being installed into. Each
// ClusterExtension might have a different installNamespace, so we can't reuse validators.
func validateConfigWithSchema(configBytes []byte, schema map[string]any, installNamespace string) error {
	var configData interface{}
	if err := yaml.Unmarshal(configBytes, &configData); err != nil {
		return formatUnmarshalError(err)
	}

	compiler := jsonschema.NewCompiler()

	// Register custom formats for validation
	compiler.RegisterFormat(&jsonschema.Format{
		Name: formatOwnNamespaceInstallMode,
		Validate: func(value interface{}) error {
			// Check it equals install namespace (if installNamespace is set)
			// If installNamespace is empty, we can't validate the constraint properly,
			// so we skip validation and accept any value. This is a fallback for edge
			// cases where the install namespace isn't known yet.
			if installNamespace == "" {
				return nil
			}
			str, ok := value.(string)
			if !ok {
				return fmt.Errorf("value must be a string")
			}
			if str != installNamespace {
				return fmt.Errorf("invalid value %q: watchNamespace must be %q (the namespace where the operator is installed) because this operator only supports OwnNamespace install mode", str, installNamespace)
			}
			return nil
		},
	})
	compiler.RegisterFormat(&jsonschema.Format{
		Name: formatSingleNamespaceInstallMode,
		Validate: func(value interface{}) error {
			// Check it does NOT equal install namespace (if installNamespace is set)
			// If installNamespace is empty, we can't validate the constraint properly,
			// so we skip validation and accept any value. This is a fallback for edge
			// cases where the install namespace isn't known yet.
			if installNamespace == "" {
				return nil
			}
			str, ok := value.(string)
			if !ok {
				return fmt.Errorf("value must be a string")
			}
			if str == installNamespace {
				return fmt.Errorf("invalid value %q: watchNamespace must be different from %q (the install namespace) because this operator uses SingleNamespace install mode to watch a different namespace", str, installNamespace)
			}
			return nil
		},
	})

	if err := compiler.AddResource(configSchemaID, schema); err != nil {
		return fmt.Errorf("failed to load schema: %w", err)
	}

	compiledSchema, err := compiler.Compile(configSchemaID)
	if err != nil {
		return fmt.Errorf("failed to compile schema: %w", err)
	}

	if err := compiledSchema.Validate(configData); err != nil {
		return formatSchemaError(err)
	}

	return nil
}

// formatSchemaError converts technical JSON schema errors into messages users can understand.
//
// Uses the structured ValidationError API instead of fragile string parsing for maintainability.
func formatSchemaError(err error) error {
	schemaErr := &jsonschema.ValidationError{}
	ok := errors.As(err, &schemaErr)
	if !ok {
		return err
	}

	// The actual validation failures are in the Causes
	if len(schemaErr.Causes) == 0 {
		// Fallback if no causes (shouldn't happen, but be safe)
		return fmt.Errorf("configuration validation failed: %s", schemaErr.Error())
	}

	// When there are multiple causes, prioritize based on which gives better user feedback
	// additionalProperties > required > type
	cause := schemaErr.Causes[0]
	for _, c := range schemaErr.Causes {
		keywords := c.ErrorKind.KeywordPath()
		if len(keywords) > 0 && keywords[len(keywords)-1] == "additionalProperties" {
			cause = c
			break
		}
	}

	keyword := cause.ErrorKind.KeywordPath()

	// Extract field name from InstanceLocation if available
	var fieldName string
	if len(cause.InstanceLocation) > 0 {
		fieldName = cause.InstanceLocation[0]
	}

	// Check the keyword to determine error type
	if len(keyword) > 0 {
		switch keyword[len(keyword)-1] {
		case "additionalProperties":
			// Unknown field error - extract field name from the error message
			msg := cause.Error()
			if idx := strings.Index(msg, "additional properties '"); idx != -1 {
				remaining := msg[idx+len("additional properties '"):]
				if endIdx := strings.Index(remaining, "'"); endIdx != -1 {
					fieldName = remaining[:endIdx]
				}
			}
			if fieldName != "" {
				return fmt.Errorf("unknown field %q", fieldName)
			}
			return errors.New("unknown field")

		case "required":
			// Missing required field - extract from error message
			msg := cause.Error()
			if idx := strings.Index(msg, "missing property '"); idx != -1 {
				remaining := msg[idx+len("missing property '"):]
				if endIdx := strings.Index(remaining, "'"); endIdx != -1 {
					fieldName = remaining[:endIdx]
				}
			}
			if fieldName != "" {
				return fmt.Errorf("required field %q is missing", fieldName)
			}
			return errors.New("required field is missing")

		case "type":
			// Type mismatch
			msg := cause.Error()

			// Check for null -> string case (treat as missing required field)
			if strings.Contains(msg, "got null, want string") {
				if fieldName != "" {
					return fmt.Errorf("required field %q is missing", fieldName)
				}
				return errors.New("required field is missing")
			}

			// Check for top-level type error (input is not an object)
			if len(cause.InstanceLocation) == 0 && strings.Contains(msg, "want object") {
				return errors.New("input is not a valid JSON object")
			}

			// Extract type mismatch info
			var gotType, wantType string
			if idx := strings.Index(msg, "got "); idx != -1 {
				remaining := msg[idx+len("got "):]
				if endIdx := strings.Index(remaining, ","); endIdx != -1 {
					gotType = remaining[:endIdx]
				}
			}
			if idx := strings.Index(msg, "want "); idx != -1 {
				wantType = strings.TrimSpace(msg[idx+len("want "):])
			}

			if fieldName != "" && gotType != "" && wantType != "" {
				return fmt.Errorf("invalid value type for field %q: expected %q but got %q", fieldName, wantType, gotType)
			}

		case "format":
			// Format validation (our custom formats for install modes)
			msg := cause.Error()

			// Include field location and "configuration validation failed" prefix
			// to match test expectations
			if fieldName != "" {
				// Strip the "at '/fieldName': " prefix if present
				if idx := strings.Index(msg, ": "); idx != -1 {
					return fmt.Errorf("configuration validation failed at '/%s': %s", fieldName, msg[idx+2:])
				}
				return fmt.Errorf("configuration validation failed at '/%s': %s", fieldName, msg)
			}

			// Fallback without field name
			if idx := strings.Index(msg, ": "); idx != -1 {
				return fmt.Errorf("configuration validation failed: %s", msg[idx+2:])
			}
			return fmt.Errorf("configuration validation failed: %s", msg)
		}
	}

	// Couldn't determine specific error type, return the raw message
	msg := cause.Error()
	// Strip "at '/fieldName': " prefix for cleaner output
	if idx := strings.Index(msg, ": "); idx != -1 {
		return fmt.Errorf("configuration validation failed: %s", msg[idx+2:])
	}
	return fmt.Errorf("configuration validation failed: %s", msg)
}

// formatUnmarshalError makes YAML/JSON parsing errors easier to understand.
func formatUnmarshalError(err error) error {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		if typeErr.Field == "" {
			return errors.New("input is not a valid JSON object")
		}
		return fmt.Errorf("invalid value type for field %q: expected %q but got %q",
			typeErr.Field, typeErr.Type.String(), typeErr.Value)
	}

	// Unwrap to core error and strip "json:" or "yaml:" prefix
	current := err
	for {
		unwrapped := errors.Unwrap(current)
		if unwrapped == nil {
			parts := strings.Split(current.Error(), ":")
			coreMessage := strings.TrimSpace(parts[len(parts)-1])
			return errors.New(coreMessage)
		}
		current = unwrapped
	}
}
