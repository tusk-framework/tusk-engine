package components

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	BuiltInInvocationComponentName = "in-process-service-invocation"
	BuiltInResilienceComponentName = "default-resilience"
)

var ErrServiceUnavailable = errors.New("service invocation service unavailable")

type InvocationHandler func(context.Context, InvocationRequest) (InvocationResponse, error)

type FuncServiceInvocationProvider struct {
	descriptor Descriptor
	handler    InvocationHandler
}

func NewFuncServiceInvocationProvider(descriptor Descriptor, handler InvocationHandler) (*FuncServiceInvocationProvider, error) {
	if handler == nil {
		return nil, errors.New("service invocation handler is required")
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

func (p *FuncServiceInvocationProvider) Configure(context.Context, Configuration) error { return nil }

func (p *FuncServiceInvocationProvider) Health(ctx context.Context) error {
	return ctx.Err()
}

func (p *FuncServiceInvocationProvider) Invoke(ctx context.Context, request InvocationRequest) (InvocationResponse, error) {
	if err := ctx.Err(); err != nil {
		return InvocationResponse{}, err
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Minute)
		defer cancel()
	}
	return p.handler(ctx, request)
}

type HandlerServiceInvocationProvider struct {
	descriptor Descriptor
	mu         sync.RWMutex
	handlers   map[string]InvocationHandler
}

func NewHandlerServiceInvocationProvider(descriptor Descriptor) (*HandlerServiceInvocationProvider, error) {
	if err := descriptor.Validate(); err != nil {
		return nil, fmt.Errorf("service invocation descriptor: %w", err)
	}
	if !declaresCapability(descriptor, CapabilityServiceInvocation) {
		return nil, fmt.Errorf("service invocation descriptor must declare capability %q", CapabilityServiceInvocation)
	}
	return &HandlerServiceInvocationProvider{
		descriptor: descriptor,
		handlers:   make(map[string]InvocationHandler),
	}, nil
}

func (p *HandlerServiceInvocationProvider) Descriptor() Descriptor { return p.descriptor }

func (p *HandlerServiceInvocationProvider) Configure(context.Context, Configuration) error {
	return nil
}

func (p *HandlerServiceInvocationProvider) Health(ctx context.Context) error { return ctx.Err() }

func (p *HandlerServiceInvocationProvider) Register(service string, handler InvocationHandler) error {
	if service == "" {
		return errors.New("service name is required")
	}
	if handler == nil {
		return errors.New("service invocation handler is required")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers[service] = handler
	return nil
}

func (p *HandlerServiceInvocationProvider) Invoke(ctx context.Context, request InvocationRequest) (InvocationResponse, error) {
	p.mu.RLock()
	handler := p.handlers[request.Service]
	p.mu.RUnlock()
	if handler == nil {
		return InvocationResponse{}, fmt.Errorf("%w: %s", ErrServiceUnavailable, request.Service)
	}
	return handler(ctx, cloneInvocationRequest(request))
}

func cloneInvocationRequest(request InvocationRequest) InvocationRequest {
	request.Body = append([]byte(nil), request.Body...)
	sourceHeaders := request.Headers
	if sourceHeaders != nil {
		request.Headers = make(map[string][]string, len(sourceHeaders))
		for name, values := range sourceHeaders {
			request.Headers[name] = append([]string(nil), values...)
		}
	}
	return request
}

type ResilienceProvider struct {
	mu     sync.RWMutex
	policy InvocationPolicy
}

func (p *ResilienceProvider) Descriptor() Descriptor { return resilienceDescriptor() }

func (p *ResilienceProvider) Configure(ctx context.Context, configuration Configuration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	policy := defaultInvocationPolicy()
	var err error
	if policy.Deadline, err = durationValue(configuration, "deadline", policy.Deadline); err != nil {
		return err
	}
	if policy.Retry.MaxAttempts, err = integerValue(configuration, "max_attempts", policy.Retry.MaxAttempts); err != nil {
		return err
	}
	if policy.Retry.InitialBackoff, err = durationValue(configuration, "initial_backoff", policy.Retry.InitialBackoff); err != nil {
		return err
	}
	if policy.Retry.MaxBackoff, err = durationValue(configuration, "max_backoff", policy.Retry.MaxBackoff); err != nil {
		return err
	}
	if policy.CircuitFailureThreshold, err = integerValue(configuration, "failure_threshold", policy.CircuitFailureThreshold); err != nil {
		return err
	}
	if policy.CircuitFailureThreshold, err = integerValue(configuration, "circuit_failure_threshold", policy.CircuitFailureThreshold); err != nil {
		return err
	}
	if policy.CircuitResetTimeout, err = durationValue(configuration, "reset_timeout", policy.CircuitResetTimeout); err != nil {
		return err
	}
	if policy.CircuitResetTimeout, err = durationValue(configuration, "circuit_reset_timeout", policy.CircuitResetTimeout); err != nil {
		return err
	}
	if err := policy.Validate(); err != nil {
		return fmt.Errorf("resilience policy: %w", err)
	}
	p.mu.Lock()
	p.policy = policy
	p.mu.Unlock()
	return nil
}

func (p *ResilienceProvider) Health(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.RLock()
	policy := p.policy
	p.mu.RUnlock()
	return policy.Validate()
}

func (p *ResilienceProvider) Policy() InvocationPolicy {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.policy
}

func (p *ResilienceProvider) NewInvoker(provider ServiceInvocationProvider) (*Invoker, error) {
	return NewInvoker(provider, p.Policy())
}

func DefaultRegistrations() []Registration {
	invocation := invocationDescriptor()
	resilience := resilienceDescriptor()
	return []Registration{
		{
			Descriptor: invocation,
			New: func() (Provider, error) {
				return NewHandlerServiceInvocationProvider(invocation)
			},
		},
		{
			Descriptor: resilience,
			New: func() (Provider, error) {
				return &ResilienceProvider{policy: defaultInvocationPolicy()}, nil
			},
		},
	}
}

func ResilienceRegistration() Registration {
	descriptor := resilienceDescriptor()
	return Registration{
		Descriptor: descriptor,
		New: func() (Provider, error) {
			return &ResilienceProvider{policy: defaultInvocationPolicy()}, nil
		},
	}
}

func invocationDescriptor() Descriptor {
	return Descriptor{
		Name:          BuiltInInvocationComponentName,
		Version:       "1.0.0",
		SchemaVersion: "v1",
		Capabilities:  []Capability{CapabilityServiceInvocation},
		Schema:        Schema{Fields: map[string]FieldSchema{}},
		Health:        HealthOnStartup,
	}
}

func resilienceDescriptor() Descriptor {
	return Descriptor{
		Name:          BuiltInResilienceComponentName,
		Version:       "1.0.0",
		SchemaVersion: "v1",
		Capabilities:  []Capability{CapabilityResilience},
		Schema: Schema{Fields: map[string]FieldSchema{
			"deadline":                  {Type: FieldDuration},
			"max_attempts":              {Type: FieldInteger},
			"initial_backoff":           {Type: FieldDuration},
			"max_backoff":               {Type: FieldDuration},
			"failure_threshold":         {Type: FieldInteger},
			"reset_timeout":             {Type: FieldDuration},
			"circuit_failure_threshold": {Type: FieldInteger},
			"circuit_reset_timeout":     {Type: FieldDuration},
		}},
		Health: HealthOnStartup,
	}
}

func defaultInvocationPolicy() InvocationPolicy {
	return InvocationPolicy{
		Deadline:                5 * time.Second,
		Retry:                   RetryPolicy{MaxAttempts: 3, InitialBackoff: 50 * time.Millisecond, MaxBackoff: 500 * time.Millisecond},
		CircuitFailureThreshold: 5,
		CircuitResetTimeout:     30 * time.Second,
	}
}

func durationValue(configuration Configuration, name string, fallback time.Duration) (time.Duration, error) {
	value, present := configuration[name]
	if !present {
		return fallback, nil
	}
	switch value := value.(type) {
	case time.Duration:
		return value, nil
	case string:
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return 0, fmt.Errorf("%s must be a duration: %w", name, err)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("%s must be a duration", name)
	}
}

func integerValue(configuration Configuration, name string, fallback int) (int, error) {
	value, present := configuration[name]
	if !present {
		return fallback, nil
	}
	if !isInteger(value) {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	switch value := value.(type) {
	case int:
		return value, nil
	case int8:
		return int(value), nil
	case int16:
		return int(value), nil
	case int32:
		return int(value), nil
	case int64:
		return int(value), nil
	case uint:
		return int(value), nil
	case uint8:
		return int(value), nil
	case uint16:
		return int(value), nil
	case uint32:
		return int(value), nil
	case uint64:
		return int(value), nil
	case float32:
		return int(value), nil
	case float64:
		return int(value), nil
	default:
		return 0, fmt.Errorf("%s must be an integer", name)
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
