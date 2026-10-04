package components

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type invocationProvider struct {
	descriptor Descriptor
	calls      atomic.Int32
	fail       atomic.Int32
}

func (p *invocationProvider) Descriptor() Descriptor                         { return p.descriptor }
func (p *invocationProvider) Configure(context.Context, Configuration) error { return nil }
func (p *invocationProvider) Health(context.Context) error                   { return nil }
func (p *invocationProvider) Invoke(ctx context.Context, _ InvocationRequest) (InvocationResponse, error) {
	p.calls.Add(1)
	if p.fail.Load() > 0 {
		p.fail.Add(-1)
		return InvocationResponse{}, errors.New("transient")
	}
	return InvocationResponse{Status: 200}, nil
}

func invocationPolicy() InvocationPolicy {
	return InvocationPolicy{
		Deadline:                time.Second,
		Retry:                   RetryPolicy{MaxAttempts: 3, InitialBackoff: 0, MaxBackoff: time.Millisecond},
		CircuitFailureThreshold: 2,
		CircuitResetTimeout:     time.Second,
	}
}

func TestInvokerRetriesOnlySafeOperations(t *testing.T) {
	provider := &invocationProvider{descriptor: validDescriptor()}
	provider.fail.Store(2)
	invoker, err := NewInvoker(provider, invocationPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := invoker.Invoke(context.Background(), InvocationRequest{Idempotency: Idempotent}); err != nil {
		t.Fatalf("idempotent Invoke() error = %v", err)
	}
	if got := provider.calls.Load(); got != 3 {
		t.Fatalf("idempotent calls = %d, want 3", got)
	}

	provider.fail.Store(2)
	provider.calls.Store(0)
	if _, err := invoker.Invoke(context.Background(), InvocationRequest{Idempotency: NonIdempotent}); err == nil {
		t.Fatal("non-idempotent Invoke() unexpectedly succeeded")
	}
	if got := provider.calls.Load(); got != 1 {
		t.Fatalf("non-idempotent calls = %d, want 1", got)
	}
}

func TestInvokerStopsAtDeadline(t *testing.T) {
	provider := &invocationProvider{descriptor: validDescriptor()}
	provider.fail.Store(10)
	policy := invocationPolicy()
	policy.Deadline = 10 * time.Millisecond
	policy.Retry.InitialBackoff = 20 * time.Millisecond
	policy.Retry.MaxBackoff = 20 * time.Millisecond
	invoker, err := NewInvoker(provider, policy)
	if err != nil {
		t.Fatal(err)
	}
	_, err = invoker.Invoke(context.Background(), InvocationRequest{Idempotency: Idempotent})
	if err == nil || !strings.Contains(err.Error(), ErrInvocationDeadline.Error()) {
		t.Fatalf("Invoke() error = %v, want deadline error", err)
	}
}

func TestCircuitBreakerOpensAfterLogicalFailures(t *testing.T) {
	provider := &invocationProvider{descriptor: validDescriptor()}
	provider.fail.Store(10)
	policy := invocationPolicy()
	policy.Retry.MaxAttempts = 1
	policy.CircuitFailureThreshold = 2
	invoker, err := NewInvoker(provider, policy)
	if err != nil {
		t.Fatal(err)
	}
	request := InvocationRequest{Idempotency: NonIdempotent}
	for range 2 {
		if _, err := invoker.Invoke(context.Background(), request); err == nil {
			t.Fatal("expected provider failure")
		}
	}
	calls := provider.calls.Load()
	if _, err := invoker.Invoke(context.Background(), request); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("third Invoke() error = %v, want circuit open", err)
	}
	if provider.calls.Load() != calls {
		t.Fatal("circuit-open invocation reached provider")
	}
}
