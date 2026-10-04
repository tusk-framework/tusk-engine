package components

import (
	"strings"
	"testing"
)

func TestSchemaValidatesRequiredKnownTypedConfiguration(t *testing.T) {
	schema := Schema{Fields: map[string]FieldSchema{
		"endpoint": {Type: FieldString, Required: true},
		"enabled":  {Type: FieldBoolean},
		"timeout":  {Type: FieldDuration},
		"retries":  {Type: FieldInteger},
	}}

	if err := schema.Validate(Configuration{
		"endpoint": "https://example.test",
		"enabled":  true,
		"timeout":  "2s",
		"retries":  2,
	}); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestSchemaRejectsMissingUnknownAndWrongTypedFields(t *testing.T) {
	schema := Schema{Fields: map[string]FieldSchema{
		"endpoint": {Type: FieldString, Required: true},
		"retries":  {Type: FieldInteger},
	}}

	tests := []struct {
		name string
		cfg  Configuration
		want string
	}{
		{name: "missing required", cfg: Configuration{}, want: "endpoint"},
		{name: "unknown field", cfg: Configuration{"endpoint": "x", "other": true}, want: "unknown"},
		{name: "wrong type", cfg: Configuration{"endpoint": 12}, want: "endpoint"},
		{name: "non integral number", cfg: Configuration{"endpoint": "x", "retries": 1.5}, want: "retries"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := schema.Validate(tt.cfg)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tt.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestConfigurationRedactedCopiesAndHidesSecretValues(t *testing.T) {
	configuration := Configuration{"endpoint": "https://example.test", "token": "top-secret"}
	redacted := configuration.Redacted([]string{"token"})

	if redacted["token"] != RedactedValue {
		t.Fatalf("redacted token = %v, want %q", redacted["token"], RedactedValue)
	}
	if redacted["endpoint"] != configuration["endpoint"] {
		t.Fatalf("redacted endpoint = %v, want %v", redacted["endpoint"], configuration["endpoint"])
	}
	redacted["endpoint"] = "changed"
	if configuration["endpoint"] == "changed" {
		t.Fatal("Redacted() returned the original map")
	}
}
