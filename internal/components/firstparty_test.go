package components

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewFuncServiceInvocationProviderRejectsNilHandler(t *testing.T) {
	_, err := NewFuncServiceInvocationProvider(validDescriptor(), nil)
	if err == nil || !strings.Contains(err.Error(), "handler") {
		t.Fatalf("NewFuncServiceInvocationProvider() error = %v, want handler validation", err)
	}
}

func TestNewFuncServiceInvocationProviderRequiresInvocationCapability(t *testing.T) {
	_, err := NewFuncServiceInvocationProvider(resilienceDescriptor(), func(context.Context, InvocationRequest) (InvocationResponse, error) {
		return InvocationResponse{}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "capability") {
		t.Fatalf("NewFuncServiceInvocationProvider() error = %v, want capability validation", err)
	}
}

func TestServiceInvocationProvidersRejectMissingCapability(t *testing.T) {
	descriptor := validDescriptor()
	descriptor.Capabilities = []Capability{CapabilityResilience}
	if _, err := NewFuncServiceInvocationProvider(descriptor, func(context.Context, InvocationRequest) (InvocationResponse, error) {
		return InvocationResponse{}, nil
	}); err == nil || !strings.Contains(err.Error(), "service.invocation") {
		t.Fatalf("NewFuncServiceInvocationProvider() error = %v, want capability validation", err)
	}
	if _, err := NewHandlerServiceInvocationProvider(descriptor); err == nil || !strings.Contains(err.Error(), "service.invocation") {
		t.Fatalf("NewHandlerServiceInvocationProvider() error = %v, want capability validation", err)
	}
}

func TestFuncServiceInvocationProviderInvokesConfiguredHandler(t *testing.T) {
	descriptor := validDescriptor()
	wantErr := errors.New("handler error")
	provider, err := NewFuncServiceInvocationProvider(descriptor, func(ctx context.Context, request InvocationRequest) (InvocationResponse, error) {
		if request.Service != "billing" || request.Method != "POST" {
			t.Fatalf("request = %+v, want service and method preserved", request)
		}
		if ctx == nil {
			t.Fatal("handler context is nil")
		}
		return InvocationResponse{}, wantErr
	})
	if err != nil {
		t.Fatalf("NewFuncServiceInvocationProvider() error = %v", err)
	}
	if provider.Descriptor().Name != descriptor.Name {
		t.Fatalf("descriptor = %+v, want %+v", provider.Descriptor(), descriptor)
	}
	_, err = provider.Invoke(context.WithValue(context.Background(), struct{}{}, "value"), InvocationRequest{
		Service: "billing",
		Method:  "POST",
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Invoke() error = %v, want %v", err, wantErr)
	}
}

func TestDefaultRegistrationsIncludeConfiguredResilienceProvider(t *testing.T) {
	registrations := DefaultRegistrations()
	var resilience Registration
	found := false
	for _, registration := range registrations {
		if registration.Descriptor.Name == BuiltInResilienceComponentName {
			resilience = registration
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("DefaultRegistrations() = %#v, want %q", registrations, BuiltInResilienceComponentName)
	}
	provider, err := resilience.New()
	if err != nil {
		t.Fatalf("resilience factory error = %v", err)
	}
	if err := provider.Configure(context.Background(), nil); err != nil {
		t.Fatalf("resilience Configure() error = %v", err)
	}
	resilienceProvider, ok := provider.(*ResilienceProvider)
	if !ok {
		t.Fatalf("provider type = %T, want *ResilienceProvider", provider)
	}
	if got := resilienceProvider.Policy(); got != defaultInvocationPolicy() {
		t.Fatalf("default policy = %+v, want %+v", got, defaultInvocationPolicy())
	}
}

func TestResilienceProviderAcceptsBoundedOverrides(t *testing.T) {
	registration := ResilienceRegistration()
	provider, err := registration.New()
	if err != nil {
		t.Fatal(err)
	}
	configuration := Configuration{
		"deadline":          "2s",
		"max_attempts":      4,
		"initial_backoff":   "10ms",
		"max_backoff":       "250ms",
		"failure_threshold": 5,
		"reset_timeout":     "45s",
	}
	if err := provider.Configure(context.Background(), configuration); err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
	got := provider.(*ResilienceProvider).Policy()
	want := InvocationPolicy{
		Deadline:                2 * time.Second,
		Retry:                   RetryPolicy{MaxAttempts: 4, InitialBackoff: 10 * time.Millisecond, MaxBackoff: 250 * time.Millisecond},
		CircuitFailureThreshold: 5,
		CircuitResetTimeout:     45 * time.Second,
	}
	if got != want {
		t.Fatalf("policy = %+v, want %+v", got, want)
	}
}

func TestResilienceProviderRejectsInvalidConfiguration(t *testing.T) {
	for _, configuration := range []Configuration{
		{"deadline": "not-a-duration"},
		{"max_attempts": "three"},
		{"max_attempts": 6},
		{"failure_threshold": 0},
	} {
		provider, err := ResilienceRegistration().New()
		if err != nil {
			t.Fatal(err)
		}
		if err := provider.Configure(context.Background(), configuration); err == nil {
			t.Fatalf("Configure(%#v) succeeded unexpectedly", configuration)
		}
	}
}

func TestResilienceProviderCreatesInvokerForSubstitutedProvider(t *testing.T) {
	provider, err := ResilienceRegistration().New()
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.Configure(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	invocationProvider := &invocationProvider{descriptor: validDescriptor()}
	invoker, err := provider.(*ResilienceProvider).NewInvoker(invocationProvider)
	if err != nil {
		t.Fatalf("NewInvoker() error = %v", err)
	}
	if _, err := invoker.Invoke(context.Background(), InvocationRequest{Idempotency: Idempotent}); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
}

func TestHandlerServiceInvocationProviderCopiesCallerRequest(t *testing.T) {
	descriptor := invocationDescriptor()
	provider, err := NewHandlerServiceInvocationProvider(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.Register("billing", func(_ context.Context, request InvocationRequest) (InvocationResponse, error) {
		if len(request.Headers["X-Test"]) != 1 {
			t.Fatalf("copied headers = %#v", request.Headers)
		}
		request.Body[0] = 'x'
		request.Headers["X-Test"][0] = "changed"
		return InvocationResponse{Status: 200}, nil
	}); err != nil {
		t.Fatal(err)
	}

	body := []byte("body")
	headers := map[string][]string{"X-Test": []string{"original"}}
	if _, err := provider.Invoke(context.Background(), InvocationRequest{
		Service: "billing",
		Headers: headers,
		Body:    body,
	}); err != nil {
		t.Fatal(err)
	}
	if string(body) != "body" || headers["X-Test"][0] != "original" {
		t.Fatalf("caller request mutated: body=%q headers=%v", body, headers)
	}
}
