package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tusk-framework/tusk-engine/internal/components"
	"github.com/tusk-framework/tusk-engine/internal/config"
	engineRuntime "github.com/tusk-framework/tusk-engine/internal/runtime"
)

func TestStartActivatesComponentsBeforeControlAndRuntime(t *testing.T) {
	events := []string{}
	descriptor := lifecycleDescriptor("test-component")
	registry, err := components.NewRegistry(components.Registration{
		Descriptor: descriptor,
		New: func() (components.Provider, error) {
			events = append(events, "factory")
			return &lifecycleProvider{descriptor: descriptor, events: &events}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &recordingRuntime{events: &events}
	control := newRecordingControl(&events, nil)
	engine, err := New(Options{
		Registry:            registry,
		Configurations:      map[string]components.Configuration{descriptor.Name: {}},
		Runtime:             runtime,
		Probe:               recordingProbe{},
		ProbeInterval:       time.Millisecond,
		StartupTimeout:      time.Second,
		Control:             control,
		ControlEnabled:      true,
		ControlReadyTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := engine.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	want := []string{"factory", "configure", "health", "control-start", "control-ready", "runtime-start", "runtime-ready"}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", events, want)
	}
	if err := engine.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestStartDoesNotStartServicesWhenComponentActivationFails(t *testing.T) {
	events := []string{}
	descriptor := lifecycleDescriptor("broken-component")
	registry, err := components.NewRegistry(components.Registration{
		Descriptor: descriptor,
		New: func() (components.Provider, error) {
			events = append(events, "factory")
			return &lifecycleProvider{
				descriptor:   descriptor,
				events:       &events,
				configureErr: errors.New("invalid component configuration"),
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &recordingRuntime{events: &events}
	control := newRecordingControl(&events, nil)
	engine, err := New(Options{
		Registry:       registry,
		Runtime:        runtime,
		Probe:          recordingProbe{},
		ProbeInterval:  time.Millisecond,
		StartupTimeout: time.Second,
		Control:        control,
		ControlEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "activate components") {
		t.Fatalf("Start() error = %v, want activation failure", err)
	}
	if strings.Contains(strings.Join(events, ","), "control-start") || strings.Contains(strings.Join(events, ","), "runtime-start") {
		t.Fatalf("services started after activation failure: %v", events)
	}
	if registry.Ready() {
		t.Fatal("registry became ready after activation failure")
	}
}

func TestStartStopsControlWhenControlReadinessFails(t *testing.T) {
	events := []string{}
	descriptor := lifecycleDescriptor("healthy-component")
	registry, err := components.NewRegistry(components.Registration{
		Descriptor: descriptor,
		New: func() (components.Provider, error) {
			return &lifecycleProvider{descriptor: descriptor, events: &events}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	control := newRecordingControl(&events, errors.New("control unavailable"))
	engine, err := New(Options{
		Registry:            registry,
		Runtime:             &recordingRuntime{events: &events},
		Probe:               recordingProbe{},
		ProbeInterval:       time.Millisecond,
		StartupTimeout:      time.Second,
		Control:             control,
		ControlEnabled:      true,
		ControlReadyTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "control plane") {
		t.Fatalf("Start() error = %v, want control failure", err)
	}
	if containsEvent(events, "runtime-start") {
		t.Fatalf("runtime started after control failure: %v", events)
	}
	if !containsEvent(events, "control-stop") {
		t.Fatalf("control was not stopped after readiness failure: %v", events)
	}
}

func TestStartStopsStartedServicesWhenRuntimeReadinessFails(t *testing.T) {
	events := []string{}
	descriptor := lifecycleDescriptor("healthy-component")
	registry, err := components.NewRegistry(components.Registration{
		Descriptor: descriptor,
		New: func() (components.Provider, error) {
			return &lifecycleProvider{descriptor: descriptor, events: &events}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &recordingRuntime{events: &events, readyErr: errors.New("runtime failed readiness")}
	control := newRecordingControl(&events, nil)
	engine, err := New(Options{
		Registry:       registry,
		Runtime:        runtime,
		Probe:          recordingProbe{},
		ProbeInterval:  time.Millisecond,
		StartupTimeout: time.Second,
		Control:        control,
		ControlEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "runtime readiness") {
		t.Fatalf("Start() error = %v, want runtime readiness failure", err)
	}
	if !containsEvent(events, "runtime-stop") || !containsEvent(events, "control-stop") {
		t.Fatalf("started services were not cleaned up: %v", events)
	}
}

func TestStopIsIdempotentAfterSuccessfulStart(t *testing.T) {
	events := []string{}
	descriptor := lifecycleDescriptor("healthy-component")
	registry, err := components.NewRegistry(components.Registration{
		Descriptor: descriptor,
		New: func() (components.Provider, error) {
			return &lifecycleProvider{descriptor: descriptor, events: &events}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := New(Options{
		Registry:       registry,
		Runtime:        &recordingRuntime{events: &events},
		Probe:          recordingProbe{},
		ProbeInterval:  time.Millisecond,
		StartupTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := engine.Stop(context.Background()); err != nil {
		t.Fatalf("first Stop() error = %v", err)
	}
	if err := engine.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
}

func TestNewFromConfigActivatesFirstPartyComponentsBeforeControlFactory(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Control.Enabled = true
	cfg.Components = map[string]components.Configuration{
		components.BuiltInResilienceComponentName: {"max_attempts": 4},
	}
	events := []string{}
	var descriptors []components.Descriptor
	control := newRecordingControl(&events, nil)
	engine, err := NewFromConfig(cfg, Options{
		Runtime:        &recordingRuntime{events: &events},
		Probe:          recordingProbe{},
		ProbeInterval:  time.Millisecond,
		StartupTimeout: time.Second,
		ControlFactory: func(got []components.Descriptor) (ControlPlane, error) {
			descriptors = got
			return control, nil
		},
		ControlReadyTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewFromConfig() error = %v", err)
	}
	if _, err := engine.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !engine.Registry().Ready() || len(descriptors) != 2 {
		t.Fatalf("registry ready=%v descriptors=%v, want active registry with two first-party descriptors", engine.Registry().Ready(), descriptors)
	}
	if _, err := engine.Registry().Resolve(components.BuiltInResilienceComponentName, components.CapabilityResilience); err != nil {
		t.Fatalf("Resolve(resilience) error = %v", err)
	}
	if err := engine.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestStartRejectsNilControlFactoryResult(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Control.Enabled = true
	engine, err := NewFromConfig(cfg, Options{
		Runtime:        &recordingRuntime{events: &[]string{}},
		Probe:          recordingProbe{},
		ProbeInterval:  time.Millisecond,
		StartupTimeout: time.Second,
		ControlFactory: func([]components.Descriptor) (ControlPlane, error) {
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("NewFromConfig() error = %v", err)
	}
	if _, err := engine.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "control plane") {
		t.Fatalf("Start() error = %v, want nil control plane error", err)
	}
}

type lifecycleProvider struct {
	descriptor   components.Descriptor
	events       *[]string
	configureErr error
	healthErr    error
}

func (p *lifecycleProvider) Descriptor() components.Descriptor { return p.descriptor }
func (p *lifecycleProvider) Configure(context.Context, components.Configuration) error {
	*p.events = append(*p.events, "configure")
	return p.configureErr
}
func (p *lifecycleProvider) Health(context.Context) error {
	*p.events = append(*p.events, "health")
	return p.healthErr
}

type recordingRuntime struct {
	events   *[]string
	readyErr error
}

func (r *recordingRuntime) Start(context.Context) error {
	*r.events = append(*r.events, "runtime-start")
	return nil
}
func (r *recordingRuntime) WaitReady(context.Context, engineRuntime.ReadinessProbe, time.Duration) error {
	*r.events = append(*r.events, "runtime-ready")
	return r.readyErr
}
func (r *recordingRuntime) Stop(context.Context) error {
	*r.events = append(*r.events, "runtime-stop")
	return nil
}

type recordingControl struct {
	events   *[]string
	readyErr error
	stopCh   chan struct{}
	started  chan struct{}
}

func newRecordingControl(events *[]string, readyErr error) *recordingControl {
	return &recordingControl{
		events:   events,
		readyErr: readyErr,
		stopCh:   make(chan struct{}),
		started:  make(chan struct{}),
	}
}

func (c *recordingControl) Start() error {
	*c.events = append(*c.events, "control-start")
	close(c.started)
	<-c.stopCh
	return nil
}
func (c *recordingControl) WaitReady(context.Context) error {
	<-c.started
	*c.events = append(*c.events, "control-ready")
	return c.readyErr
}
func (c *recordingControl) Stop(context.Context) error {
	*c.events = append(*c.events, "control-stop")
	select {
	case <-c.stopCh:
	default:
		close(c.stopCh)
	}
	return nil
}

type recordingProbe struct{}

func (recordingProbe) Check(context.Context) error { return nil }

func lifecycleDescriptor(name string) components.Descriptor {
	return components.Descriptor{
		Name:          name,
		Version:       "1.0.0",
		SchemaVersion: "v1",
		Capabilities:  []components.Capability{components.CapabilityResilience},
		Health:        components.HealthOnStartup,
	}
}

func containsEvent(events []string, wanted string) bool {
	for _, event := range events {
		if event == wanted {
			return true
		}
	}
	return false
}
