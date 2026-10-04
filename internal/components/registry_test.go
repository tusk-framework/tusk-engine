package components

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type registryProvider struct {
	descriptor Descriptor
	configure  error
	health     error
}

func (p *registryProvider) Descriptor() Descriptor                         { return p.descriptor }
func (p *registryProvider) Configure(context.Context, Configuration) error { return p.configure }
func (p *registryProvider) Health(context.Context) error                   { return p.health }

func registrationFor(descriptor Descriptor, provider func() *registryProvider) Registration {
	return Registration{
		Descriptor: descriptor,
		New:        func() (Provider, error) { return provider(), nil },
	}
}

func TestRegistryActivatesOnlyAfterAllProvidersValidateAndBecomeHealthy(t *testing.T) {
	descriptor := validDescriptor()
	created := 0
	registry, err := NewRegistry(Registration{
		Descriptor: descriptor,
		New: func() (Provider, error) {
			created++
			return &registryProvider{descriptor: descriptor}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	if registry.Ready() {
		t.Fatal("registry is ready before activation")
	}

	err = registry.Activate(context.Background(), map[string]Configuration{
		descriptor.Name: {"endpoint": "https://example.test", "token": "secret"},
	})
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	if !registry.Ready() || created != 1 {
		t.Fatalf("registry ready = %v, created = %d", registry.Ready(), created)
	}
	if _, err := registry.Resolve(descriptor.Name, CapabilityServiceInvocation); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
}

func TestRegistryRejectsConfigurationBeforeCreatingProvider(t *testing.T) {
	descriptor := validDescriptor()
	created := 0
	descriptor.Schema = Schema{Fields: map[string]FieldSchema{"endpoint": {Type: FieldString, Required: true}}}
	descriptor.SecretFields = nil
	registry, err := NewRegistry(Registration{
		Descriptor: descriptor,
		New: func() (Provider, error) {
			created++
			return &registryProvider{descriptor: descriptor}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = registry.Activate(context.Background(), map[string]Configuration{})
	if err == nil || !strings.Contains(err.Error(), "endpoint") {
		t.Fatalf("Activate() error = %v, want endpoint validation", err)
	}
	if created != 0 || registry.Ready() {
		t.Fatalf("invalid config created=%d ready=%v", created, registry.Ready())
	}
}

func TestRegistryDoesNotPublishPartialActivationOnHealthFailure(t *testing.T) {
	first := validDescriptor()
	first.Name = "first"
	second := validDescriptor()
	second.Name = "second"
	second.Health = HealthOnStartup
	registry, err := NewRegistry(
		registrationFor(first, func() *registryProvider { return &registryProvider{descriptor: first} }),
		registrationFor(second, func() *registryProvider {
			return &registryProvider{descriptor: second, health: errors.New("dependency unavailable")}
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := registry.Activate(context.Background(), map[string]Configuration{
		first.Name:  {"endpoint": "https://first.test", "token": "secret"},
		second.Name: {"endpoint": "https://second.test", "token": "secret"},
	}); err == nil || !strings.Contains(err.Error(), "health") {
		t.Fatalf("Activate() error = %v, want health failure", err)
	}
	if registry.Ready() {
		t.Fatal("registry published a partial activation")
	}
	if _, err := registry.Resolve(first.Name, CapabilityServiceInvocation); err == nil {
		t.Fatal("partially activated provider was resolvable")
	}
}

func TestRegistryHealthBehaviorAndCapabilityResolution(t *testing.T) {
	descriptor := validDescriptor()
	descriptor.Health = HealthOnDemand
	provider := &registryProvider{descriptor: descriptor}
	registry, err := NewRegistry(Registration{
		Descriptor: descriptor,
		New:        func() (Provider, error) { return provider, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Activate(context.Background(), map[string]Configuration{
		descriptor.Name: {"endpoint": "https://example.test", "token": "secret"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Health(context.Background(), descriptor.Name); err != nil {
		t.Fatalf("on-demand Health() error = %v", err)
	}
	if _, err := registry.Resolve(descriptor.Name, CapabilityState); err == nil || !strings.Contains(err.Error(), "capability") {
		t.Fatalf("Resolve() error = %v, want capability failure", err)
	}

	disabled := descriptor
	disabled.Name = "disabled"
	disabled.Health = HealthDisabled
	registry, err = NewRegistry(registrationFor(disabled, func() *registryProvider { return &registryProvider{descriptor: disabled} }))
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Activate(context.Background(), map[string]Configuration{
		disabled.Name: {"endpoint": "https://example.test", "token": "secret"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Health(context.Background(), disabled.Name); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("Health() error = %v, want disabled error", err)
	}
}

func TestRegistryRejectsDuplicateNames(t *testing.T) {
	first := validDescriptor()
	second := validDescriptor()
	registry, err := NewRegistry(
		registrationFor(first, func() *registryProvider { return &registryProvider{descriptor: first} }),
		registrationFor(second, func() *registryProvider { return &registryProvider{descriptor: second} }),
	)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("NewRegistry() error = %v, want duplicate error", err)
	}
	if registry != nil {
		t.Fatal("NewRegistry() returned a registry after duplicate registration")
	}
}
