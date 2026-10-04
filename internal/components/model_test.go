package components

import (
	"context"
	"strings"
	"testing"
)

func validDescriptor() Descriptor {
	return Descriptor{
		Name:          "http-invocation",
		Version:       "1.2.3",
		SchemaVersion: "v1",
		Capabilities:  []Capability{CapabilityServiceInvocation, CapabilityResilience},
		Schema: Schema{Fields: map[string]FieldSchema{
			"endpoint": {Type: FieldString, Required: true},
			"token":    {Type: FieldString, Secret: true},
		}},
		Health:       HealthOnStartup,
		SecretFields: []string{"token"},
	}
}

func TestDescriptorValidationAcceptsVersionedComponent(t *testing.T) {
	if err := validDescriptor().Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestDescriptorValidationRejectsInvalidDeclarations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Descriptor)
		want   string
	}{
		{name: "missing name", mutate: func(d *Descriptor) { d.Name = "" }, want: "name"},
		{name: "missing version", mutate: func(d *Descriptor) { d.Version = "v1" }, want: "version"},
		{name: "unsupported schema", mutate: func(d *Descriptor) { d.SchemaVersion = "v2" }, want: "schema version"},
		{name: "no capabilities", mutate: func(d *Descriptor) { d.Capabilities = nil }, want: "capability"},
		{name: "duplicate capability", mutate: func(d *Descriptor) { d.Capabilities = []Capability{CapabilityResilience, CapabilityResilience} }, want: "duplicate"},
		{name: "undeclared secret", mutate: func(d *Descriptor) { d.SecretFields = []string{"missing"} }, want: "secret field"},
		{name: "secret mismatch", mutate: func(d *Descriptor) { d.Schema.Fields["token"] = FieldSchema{Type: FieldString} }, want: "secret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			descriptor := validDescriptor()
			tt.mutate(&descriptor)
			err := descriptor.Validate()
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tt.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

type modelProvider struct{ descriptor Descriptor }

func (p modelProvider) Descriptor() Descriptor { return p.descriptor }
func (modelProvider) Configure(context.Context, Configuration) error { return nil }
func (modelProvider) Health(context.Context) error                    { return nil }
