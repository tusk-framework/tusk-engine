package components

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

const BuiltInResilienceComponentName = "tusk.resilience"

type InvocationHandler func(context.Context, InvocationRequest) (InvocationResponse, error)

// FuncServiceInvocationProvider adapts an in-process handler to the stable
// service-invocation contract without selecting a network transport.
type FuncServiceInvocationProvider struct {
	descriptor Descriptor
	handler    InvocationHandler
}

func NewFuncServiceInvocationProvider(descriptor Descriptor, handler InvocationHandler) (*FuncServiceInvocationProvider, error) {
	if handler == nil {
		return nil, fmt.Errorf("service invocation handler is required")
	}
	if err := descriptor.Validate(); err != nil {
		return nil, fmt.Errorf("service invocation descriptor: %w", err)
	}
	if !declaresCapability(descriptor, CapabilityServiceInvocation) {
		return nil, fmt.Errorf("service invocation descriptor must declare capability %q", CapabilityServiceInvocation)
	}
	return &FuncServiceInvocationProvider{descriptor: descriptor, handler: handler}, nil
}

func (p *FuncServiceInvocationProvider) Descriptor() Descriptor { return p.descriptor }

func (p *FuncServiceInvocationProvider) Configure(ctx context.Context, _ Configuration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (p *FuncServiceInvocationProvider) Health(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (p *FuncServiceInvocationProvider) Invoke(ctx context.Context, request InvocationRequest) (InvocationResponse, error) {
	return p.handler(ctx, request)
}

type ResilienceProvider struct {
	descriptor Descriptor
	policy     InvocationPolicy
}

func (p *ResilienceProvider) Descriptor() Descriptor { return p.descriptor }

func (p *ResilienceProvider) Configure(ctx context.Context, configuration Configuration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	policy := defaultInvocationPolicy()
	var err error
	if policy.Deadline, err = configuredDuration(configuration, "deadline", policy.Deadline); err != nil {
		return err
	}
	if policy.Retry.MaxAttempts, err = configuredInteger(configuration, "max_attempts", policy.Retry.MaxAttempts); err != nil {
		return err
	}
	if policy.Retry.InitialBackoff, err = configuredDuration(configuration, "initial_backoff", policy.Retry.InitialBackoff); err != nil {
		return err
	}
	if policy.Retry.MaxBackoff, err = configuredDuration(configuration, "max_backoff", policy.Retry.MaxBackoff); err != nil {
		return err
	}
	if policy.CircuitFailureThreshold, err = configuredInteger(configuration, "failure_threshold", policy.CircuitFailureThreshold); err != nil {
		return err
	}
	if policy.CircuitResetTimeout, err = configuredDuration(configuration, "reset_timeout", policy.CircuitResetTimeout); err != nil {
		return err
	}
	if err := policy.Validate(); err != nil {
		return fmt.Errorf("resilience policy: %w", err)
	}
	p.policy = policy
	return nil
}

func (p *ResilienceProvider) Health(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (p *ResilienceProvider) Policy() InvocationPolicy { return p.policy }

func (p *ResilienceProvider) NewInvoker(provider ServiceInvocationProvider) (*Invoker, error) {
	return NewInvoker(provider, p.policy)
}

func ResilienceRegistration() Registration {
	return Registration{
		Descriptor: resilienceDescriptor(),
		New: func() (Provider, error) {
			return &ResilienceProvider{
				descriptor: resilienceDescriptor(),
				policy:     defaultInvocationPolicy(),
			}, nil
		},
	}
}

func DefaultRegistrations() []Registration {
	return []Registration{ResilienceRegistration()}
}

func resilienceDescriptor() Descriptor {
	return Descriptor{
		Name:          BuiltInResilienceComponentName,
		Version:       "1.0.0",
		SchemaVersion: "v1",
		Capabilities:  []Capability{CapabilityResilience},
		Schema: Schema{Fields: map[string]FieldSchema{
			"deadline":          {Type: FieldDuration},
			"max_attempts":      {Type: FieldInteger},
			"initial_backoff":   {Type: FieldDuration},
			"max_backoff":       {Type: FieldDuration},
			"failure_threshold": {Type: FieldInteger},
			"reset_timeout":     {Type: FieldDuration},
		}},
		Health: HealthOnStartup,
	}
}

func defaultInvocationPolicy() InvocationPolicy {
	return InvocationPolicy{
		Deadline: 5 * time.Second,
		Retry: RetryPolicy{
			MaxAttempts:    3,
			InitialBackoff: 50 * time.Millisecond,
			MaxBackoff:     500 * time.Millisecond,
		},
		CircuitFailureThreshold: 3,
		CircuitResetTimeout:     30 * time.Second,
	}
}

func configuredDuration(configuration Configuration, name string, fallback time.Duration) (time.Duration, error) {
	value, ok := configuration[name]
	if !ok {
		return fallback, nil
	}
	switch typed := value.(type) {
	case time.Duration:
		return typed, nil
	case string:
		duration, err := time.ParseDuration(typed)
		if err != nil {
			return 0, fmt.Errorf("resilience field %q: invalid duration: %w", name, err)
		}
		return duration, nil
	default:
		return 0, fmt.Errorf("resilience field %q: expected duration string", name)
	}
}

func configuredInteger(configuration Configuration, name string, fallback int) (int, error) {
	value, ok := configuration[name]
	if !ok {
		return fallback, nil
	}
	if !isInteger(value) {
		return 0, fmt.Errorf("resilience field %q: expected integer", name)
	}
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int8:
		return int(typed), nil
	case int16:
		return int(typed), nil
	case int32:
		return int(typed), nil
	case int64:
		return int(typed), nil
	case uint:
		return int(typed), nil
	case uint8:
		return int(typed), nil
	case uint16:
		return int(typed), nil
	case uint32:
		return int(typed), nil
	case uint64:
		if uint64(int(typed)) != typed {
			return 0, fmt.Errorf("resilience field %q: integer is out of range", name)
		}
		return int(typed), nil
	case float32:
		return int(typed), nil
	case float64:
		return int(typed), nil
	default:
		return 0, fmt.Errorf("resilience field %q: unsupported integer type %s", name, strconv.Quote(fmt.Sprintf("%T", value)))
	}
}

func declaresCapability(descriptor Descriptor, wanted Capability) bool {
	for _, capability := range descriptor.Capabilities {
		if capability == wanted {
			return true
		}
	}
	return false
}
