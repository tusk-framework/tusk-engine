package components

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

type Registry struct {
	mu            sync.RWMutex
	registrations map[string]Registration
	providers     map[string]Provider
	ready         bool
}

func NewRegistry(registrations ...Registration) (*Registry, error) {
	registry := &Registry{registrations: make(map[string]Registration)}
	for _, registration := range registrations {
		if err := registration.Descriptor.Validate(); err != nil {
			return nil, err
		}
		if registration.New == nil {
			return nil, fmt.Errorf("component %q factory is required", registration.Descriptor.Name)
		}
		name := registration.Descriptor.Name
		if _, exists := registry.registrations[name]; exists {
			return nil, fmt.Errorf("duplicate component registration %q", name)
		}
		registry.registrations[name] = registration
	}
	return registry, nil
}

func (r *Registry) Activate(ctx context.Context, configurations map[string]Configuration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	temporary := make(map[string]Provider, len(r.registrations))
	names := make([]string, 0, len(r.registrations))
	for name := range r.registrations {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		registration := r.registrations[name]
		configuration := Configuration{}
		if configurations != nil && configurations[name] != nil {
			configuration = configurations[name]
		}
		if err := registration.Descriptor.Schema.Validate(configuration); err != nil {
			return fmt.Errorf("component %q configuration: %w", name, err)
		}
		provider, err := registration.New()
		if err != nil {
			return fmt.Errorf("create component %q: %w", name, err)
		}
		if provider == nil {
			return fmt.Errorf("create component %q: factory returned nil provider", name)
		}
		if err := provider.Configure(ctx, configuration); err != nil {
			return fmt.Errorf("configure component %q: %w", name, err)
		}
		if registration.Descriptor.Health == HealthOnStartup {
			if err := provider.Health(ctx); err != nil {
				return fmt.Errorf("health component %q: %w", name, err)
			}
		}
		temporary[name] = provider
	}

	r.mu.Lock()
	r.providers = temporary
	r.ready = true
	r.mu.Unlock()
	return nil
}

func (r *Registry) Ready() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ready
}

func (r *Registry) Resolve(name string, capability Capability) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.ready {
		return nil, fmt.Errorf("component registry is not ready")
	}
	provider, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("component %q is not registered", name)
	}
	for _, declared := range provider.Descriptor().Capabilities {
		if declared == capability {
			return provider, nil
		}
	}
	return nil, fmt.Errorf("component %q does not provide capability %q", name, capability)
}

func (r *Registry) Health(ctx context.Context, name string) error {
	r.mu.RLock()
	provider, ok := r.providers[name]
	registration, registered := r.registrations[name]
	r.mu.RUnlock()
	if !r.Ready() {
		return fmt.Errorf("component registry is not ready")
	}
	if !ok || !registered {
		return fmt.Errorf("component %q is not registered", name)
	}
	if registration.Descriptor.Health == HealthDisabled {
		return fmt.Errorf("component %q health checks are disabled", name)
	}
	return provider.Health(ctx)
}

func (r *Registry) Descriptors() []Descriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	descriptors := make([]Descriptor, 0, len(r.registrations))
	for _, registration := range r.registrations {
		descriptor := registration.Descriptor
		descriptor.Capabilities = sortedCapabilities(descriptor.Capabilities)
		descriptor.SecretFields = append([]string(nil), descriptor.SecretFields...)
		descriptors = append(descriptors, descriptor)
	}
	sort.Slice(descriptors, func(i, j int) bool { return descriptors[i].Name < descriptors[j].Name })
	return descriptors
}
