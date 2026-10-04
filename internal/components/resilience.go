package components

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrCircuitOpen = errors.New("component circuit breaker is open")

type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half_open"
)

type CircuitBreaker struct {
	mu           sync.Mutex
	state        CircuitState
	failures     int
	threshold    int
	resetTimeout time.Duration
	now          func() time.Time
	probeActive  bool
	openedAt     time.Time
}

func NewCircuitBreaker(threshold int, resetTimeout time.Duration) (*CircuitBreaker, error) {
	if threshold < 1 || threshold > 100 {
		return nil, errors.New("circuit failure threshold must be between 1 and 100")
	}
	if resetTimeout <= 0 || resetTimeout > 10*time.Minute {
		return nil, errors.New("circuit reset timeout must be positive and bounded")
	}
	return &CircuitBreaker{
		state:        CircuitClosed,
		threshold:    threshold,
		resetTimeout: resetTimeout,
		now:          time.Now,
	}, nil
}

func (b *CircuitBreaker) State() CircuitState {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.transitionLocked()
	return b.state
}

func (b *CircuitBreaker) allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.transitionLocked()
	switch b.state {
	case CircuitOpen:
		return ErrCircuitOpen
	case CircuitHalfOpen:
		if b.probeActive {
			return ErrCircuitOpen
		}
		b.probeActive = true
	}
	return nil
}

func (b *CircuitBreaker) complete(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err == nil {
		b.state = CircuitClosed
		b.failures = 0
		b.probeActive = false
		return
	}
	if b.state == CircuitHalfOpen {
		b.state = CircuitOpen
		b.openedAt = b.now()
		b.probeActive = false
		return
	}
	b.failures++
	if b.failures >= b.threshold {
		b.state = CircuitOpen
		b.openedAt = b.now()
	}
}

func (b *CircuitBreaker) transitionLocked() {
	if b.state == CircuitOpen && b.now().Sub(b.openedAt) >= b.resetTimeout {
		b.state = CircuitHalfOpen
		b.probeActive = false
	}
}

func (b *CircuitBreaker) Call(ctx context.Context, call func(context.Context) error) error {
	if err := b.allow(); err != nil {
		return err
	}
	err := call(ctx)
	b.complete(err)
	return err
}
