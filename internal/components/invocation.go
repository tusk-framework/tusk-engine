package components

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrInvocationDeadline = errors.New("component invocation deadline exceeded")

type IdempotencyMode string

const (
	IdempotencyUnknown IdempotencyMode = "unknown"
	Idempotent         IdempotencyMode = "idempotent"
	IdempotentWithKey  IdempotencyMode = "idempotent_with_key"
	NonIdempotent      IdempotencyMode = "non_idempotent"
)

type InvocationRequest struct {
	Service        string
	Method         string
	Path           string
	Headers        map[string][]string
	Body           []byte
	Idempotency    IdempotencyMode
	IdempotencyKey string
}

type InvocationResponse struct {
	Status  int
	Headers map[string][]string
	Body    []byte
}

type ServiceInvocationProvider interface {
	Provider
	Invoke(context.Context, InvocationRequest) (InvocationResponse, error)
}

type RetryPolicy struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

type InvocationPolicy struct {
	Deadline                time.Duration
	Retry                   RetryPolicy
	CircuitFailureThreshold int
	CircuitResetTimeout     time.Duration
}

func (p InvocationPolicy) Validate() error {
	if p.Deadline <= 0 || p.Deadline > 5*time.Minute {
		return errors.New("invocation deadline must be positive and bounded")
	}
	if p.Retry.MaxAttempts < 1 || p.Retry.MaxAttempts > 5 {
		return errors.New("retry max attempts must be between 1 and 5")
	}
	if p.Retry.InitialBackoff < 0 || p.Retry.MaxBackoff < p.Retry.InitialBackoff {
		return errors.New("retry backoff values are invalid")
	}
	if p.CircuitFailureThreshold < 1 || p.CircuitResetTimeout <= 0 {
		return errors.New("circuit policy is invalid")
	}
	return nil
}

type Invoker struct {
	provider ServiceInvocationProvider
	policy   InvocationPolicy
	breaker  *CircuitBreaker
	sleep    func(context.Context, time.Duration) error
}

func NewInvoker(provider ServiceInvocationProvider, policy InvocationPolicy) (*Invoker, error) {
	if provider == nil {
		return nil, errors.New("service invocation provider is required")
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	breaker, err := NewCircuitBreaker(policy.CircuitFailureThreshold, policy.CircuitResetTimeout)
	if err != nil {
		return nil, err
	}
	return &Invoker{provider: provider, policy: policy, breaker: breaker, sleep: sleepContext}, nil
}

func (i *Invoker) Invoke(ctx context.Context, request InvocationRequest) (InvocationResponse, error) {
	callContext, cancel := context.WithTimeout(ctx, i.policy.Deadline)
	defer cancel()
	var response InvocationResponse
	err := i.breaker.Call(callContext, func(callContext context.Context) error {
		attempts := i.policy.Retry.MaxAttempts
		if !retrySafe(request) {
			attempts = 1
		}
		var err error
		backoff := i.policy.Retry.InitialBackoff
		for attempt := 0; attempt < attempts; attempt++ {
			if contextErr := callContext.Err(); contextErr != nil {
				return fmt.Errorf("%w: %v", ErrInvocationDeadline, contextErr)
			}
			response, err = i.provider.Invoke(callContext, request)
			if err == nil {
				return nil
			}
			if attempt+1 == attempts {
				return err
			}
			if backoff > 0 {
				if err := i.sleep(callContext, backoff); err != nil {
					return fmt.Errorf("%w: %v", ErrInvocationDeadline, err)
				}
			}
			if backoff == 0 {
				continue
			}
			backoff *= 2
			if backoff > i.policy.Retry.MaxBackoff {
				backoff = i.policy.Retry.MaxBackoff
			}
		}
		return err
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(callContext.Err(), context.DeadlineExceeded) {
			return InvocationResponse{}, fmt.Errorf("%w: %v", ErrInvocationDeadline, err)
		}
		return InvocationResponse{}, err
	}
	return response, nil
}

func retrySafe(request InvocationRequest) bool {
	return request.Idempotency == Idempotent ||
		(request.Idempotency == IdempotentWithKey && request.IdempotencyKey != "")
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
