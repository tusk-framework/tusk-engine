package engine

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/tusk-framework/tusk-engine/internal/components"
	"github.com/tusk-framework/tusk-engine/internal/config"
	"github.com/tusk-framework/tusk-engine/internal/control"
	engineRuntime "github.com/tusk-framework/tusk-engine/internal/runtime"
)

type Runtime interface {
	Start(context.Context) error
	WaitReady(context.Context, engineRuntime.ReadinessProbe, time.Duration) error
	Stop(context.Context) error
}

type ControlPlane interface {
	Start() error
	WaitReady(context.Context) error
	Stop(context.Context) error
}

type ControlFactory func([]components.Descriptor) (ControlPlane, error)

type Options struct {
	Registry            *components.Registry
	Configurations      map[string]components.Configuration
	Runtime             Runtime
	Probe               engineRuntime.ReadinessProbe
	ProbeInterval       time.Duration
	StartupTimeout      time.Duration
	Control             ControlPlane
	ControlFactory      ControlFactory
	ControlEnabled      bool
	ControlReadyTimeout time.Duration
}

type Engine struct {
	mu                  sync.Mutex
	registry            *components.Registry
	configurations      map[string]components.Configuration
	runtime             Runtime
	probe               engineRuntime.ReadinessProbe
	probeInterval       time.Duration
	startupTimeout      time.Duration
	control             ControlPlane
	controlFactory      ControlFactory
	controlEnabled      bool
	controlReadyTimeout time.Duration
	controlErrors       chan error
	starting            bool
	started             bool
	runtimeStarted      bool
	controlStarted      bool
}

func New(options Options) (*Engine, error) {
	if options.Registry == nil {
		var err error
		options.Registry, err = components.NewRegistry(components.DefaultRegistrations()...)
		if err != nil {
			return nil, fmt.Errorf("create default component registry: %w", err)
		}
	}
	if options.Runtime == nil {
		return nil, errors.New("runtime is required")
	}
	if options.Probe == nil {
		return nil, errors.New("runtime readiness probe is required")
	}
	if options.ProbeInterval <= 0 {
		return nil, errors.New("runtime readiness probe interval must be positive")
	}
	if options.StartupTimeout <= 0 {
		return nil, errors.New("runtime startup timeout must be positive")
	}
	if options.ControlEnabled && options.Control == nil && options.ControlFactory == nil {
		return nil, errors.New("control plane is required when enabled")
	}
	if options.ControlReadyTimeout <= 0 {
		options.ControlReadyTimeout = 5 * time.Second
	}
	return &Engine{
		registry:            options.Registry,
		configurations:      options.Configurations,
		runtime:             options.Runtime,
		probe:               options.Probe,
		probeInterval:       options.ProbeInterval,
		startupTimeout:      options.StartupTimeout,
		control:             options.Control,
		controlFactory:      options.ControlFactory,
		controlEnabled:      options.ControlEnabled,
		controlReadyTimeout: options.ControlReadyTimeout,
	}, nil
}

func NewFromConfig(cfg *config.Config, options Options) (*Engine, error) {
	if cfg == nil {
		return nil, errors.New("configuration is required")
	}
	if err := cfg.Control.Validate(); err != nil {
		return nil, fmt.Errorf("invalid control configuration: %w", err)
	}
	if err := cfg.Runtime.Validate(); err != nil {
		return nil, fmt.Errorf("invalid runtime configuration: %w", err)
	}
	options.Configurations = cfg.Components
	options.ControlEnabled = cfg.Control.Enabled
	return New(options)
}

func (e *Engine) Registry() *components.Registry { return e.registry }

func (e *Engine) ControlErrors() <-chan error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.controlErrors
}

func (e *Engine) Start(ctx context.Context) (<-chan error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	e.mu.Lock()
	if e.starting || e.started {
		state := "started"
		if e.starting {
			state = "starting"
		}
		e.mu.Unlock()
		return nil, fmt.Errorf("cannot start Engine from state %q", state)
	}
	e.starting = true
	e.mu.Unlock()

	if err := e.registry.Activate(ctx, e.configurations); err != nil {
		e.finishFailedStart()
		return nil, fmt.Errorf("activate components: %w", err)
	}

	if e.controlEnabled {
		if e.control == nil && e.controlFactory != nil {
			controlPlane, err := e.controlFactory(e.registry.Descriptors())
			if err != nil {
				e.finishFailedStart()
				return nil, fmt.Errorf("create control plane: %w", err)
			}
			if controlPlane == nil {
				e.finishFailedStart()
				return nil, errors.New("create control plane: factory returned nil control plane")
			}
			e.mu.Lock()
			e.control = controlPlane
			e.mu.Unlock()
		}
		e.mu.Lock()
		e.controlStarted = true
		e.controlErrors = make(chan error, 1)
		controlErrors := e.controlErrors
		e.mu.Unlock()
		go func() { controlErrors <- e.control.Start() }()
		runtime.Gosched()
		readyContext, cancel := context.WithTimeout(ctx, e.controlReadyTimeout)
		err := e.control.WaitReady(readyContext)
		cancel()
		if err != nil {
			return nil, e.failStart(ctx, fmt.Errorf("control plane readiness: %w", err), false, true)
		}
	}

	if err := e.runtime.Start(ctx); err != nil {
		return nil, e.failStart(ctx, fmt.Errorf("start runtime: %w", err), false, e.controlEnabled)
	}
	e.mu.Lock()
	e.runtimeStarted = true
	e.mu.Unlock()

	readyContext, cancel := context.WithTimeout(ctx, e.startupTimeout)
	err := e.runtime.WaitReady(readyContext, e.probe, e.probeInterval)
	cancel()
	if err != nil {
		return nil, e.failStart(ctx, fmt.Errorf("runtime readiness: %w", err), true, e.controlEnabled)
	}

	e.mu.Lock()
	e.starting = false
	e.started = true
	controlErrors := e.controlErrors
	e.mu.Unlock()
	return controlErrors, nil
}

func (e *Engine) Snapshot() control.RuntimeSnapshot {
	if provider, ok := e.runtime.(interface {
		Snapshot() control.RuntimeSnapshot
	}); ok {
		return provider.Snapshot()
	}
	return control.RuntimeSnapshot{}
}

func (e *Engine) Wait(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-e.ControlErrors():
			if err == nil {
				return errors.New("control plane stopped unexpectedly")
			}
			return fmt.Errorf("control plane failed: %w", err)
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func (e *Engine) Stop(ctx context.Context) error {
	e.mu.Lock()
	if e.starting {
		e.mu.Unlock()
		return errors.New("cannot stop Engine while starting")
	}
	runtimeStarted := e.runtimeStarted
	controlStarted := e.controlStarted
	e.runtimeStarted = false
	e.controlStarted = false
	e.started = false
	e.mu.Unlock()
	return e.stopServices(ctx, runtimeStarted, controlStarted)
}

func (e *Engine) failStart(ctx context.Context, startErr error, runtimeStarted, controlStarted bool) error {
	cleanupErr := e.stopServices(ctx, runtimeStarted, controlStarted)
	e.finishFailedStart()
	if cleanupErr != nil {
		return errors.Join(startErr, cleanupErr)
	}
	return startErr
}

func (e *Engine) stopServices(ctx context.Context, runtimeStarted, controlStarted bool) error {
	var cleanup []error
	if runtimeStarted {
		if err := e.runtime.Stop(ctx); err != nil {
			cleanup = append(cleanup, fmt.Errorf("stop runtime: %w", err))
		}
	}
	if controlStarted {
		if err := e.control.Stop(ctx); err != nil {
			cleanup = append(cleanup, fmt.Errorf("stop control plane: %w", err))
		}
	}
	return errors.Join(cleanup...)
}

func (e *Engine) finishFailedStart() {
	e.mu.Lock()
	e.starting = false
	e.started = false
	e.runtimeStarted = false
	e.controlStarted = false
	e.mu.Unlock()
}
