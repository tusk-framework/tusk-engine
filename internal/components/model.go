package components

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type Capability string

const (
	CapabilityServiceInvocation Capability = "service.invocation"
	CapabilityResilience        Capability = "resilience"
	CapabilityDiscovery         Capability = "discovery"
	CapabilitySecrets           Capability = "secrets"
	CapabilityConfiguration     Capability = "configuration"
	CapabilityState             Capability = "state"
	CapabilityPubSub            Capability = "pubsub"
	CapabilityBindings          Capability = "bindings"
	CapabilityJobs              Capability = "jobs"
)

type HealthBehavior string

const (
	HealthOnStartup HealthBehavior = "startup"
	HealthOnDemand  HealthBehavior = "on_demand"
	HealthDisabled  HealthBehavior = "disabled"
)

type Descriptor struct {
	Name          string         `json:"name"`
	Version       string         `json:"version"`
	SchemaVersion string         `json:"schema_version"`
	Capabilities  []Capability   `json:"capabilities"`
	Schema        Schema         `json:"schema"`
	Health        HealthBehavior `json:"health"`
	SecretFields  []string       `json:"secret_fields,omitempty"`
}

func (d Descriptor) Validate() error {
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("component name is required")
	}
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(d.Version) {
		return fmt.Errorf("component %q version must be semantic", d.Name)
	}
	if d.SchemaVersion != "v1" {
		return fmt.Errorf("component %q schema version %q is unsupported", d.Name, d.SchemaVersion)
	}
	if len(d.Capabilities) == 0 {
		return fmt.Errorf("component %q must declare at least one capability", d.Name)
	}
	seen := make(map[Capability]struct{}, len(d.Capabilities))
	for _, capability := range d.Capabilities {
		if _, exists := seen[capability]; exists {
			return fmt.Errorf("component %q has duplicate capability %q", d.Name, capability)
		}
		seen[capability] = struct{}{}
	}
	if d.Health != HealthOnStartup && d.Health != HealthOnDemand && d.Health != HealthDisabled {
		return fmt.Errorf("component %q has invalid health behavior %q", d.Name, d.Health)
	}
	if err := d.Schema.ValidateDeclaration(); err != nil {
		return fmt.Errorf("component %q schema: %w", d.Name, err)
	}
	for _, secret := range d.SecretFields {
		field, ok := d.Schema.Fields[secret]
		if !ok || !field.Secret {
			return fmt.Errorf("component %q secret field %q is not secret in schema", d.Name, secret)
		}
	}
	return nil
}

type Configuration map[string]any

const RedactedValue = "[REDACTED]"

func (c Configuration) Redacted(secretFields []string) Configuration {
	redacted := make(Configuration, len(c))
	for key, value := range c {
		redacted[key] = value
	}
	for _, field := range secretFields {
		if _, exists := redacted[field]; exists {
			redacted[field] = RedactedValue
		}
	}
	return redacted
}

type Provider interface {
	Descriptor() Descriptor
	Configure(context.Context, Configuration) error
	Health(context.Context) error
}

type ProviderFactory func() (Provider, error)

type Registration struct {
	Descriptor Descriptor
	New        ProviderFactory
}

func sortedCapabilities(capabilities []Capability) []Capability {
	copyOf := append([]Capability(nil), capabilities...)
	sort.Slice(copyOf, func(i, j int) bool { return copyOf[i] < copyOf[j] })
	return copyOf
}
