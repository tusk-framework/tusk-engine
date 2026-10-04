package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/tusk-framework/tusk-engine/internal/control"
	"github.com/tusk-framework/tusk-engine/internal/metrics"
)

var (
	ErrNotReady         = errors.New("runtime is not ready")
	ErrProbeFailed      = errors.New("runtime readiness probe failed")
	ErrRuntimeFailed    = errors.New("runtime process failed")
	ErrReadinessTimeout = errors.New("runtime readiness timeout")
)

type State string

const (
	StateCreated  State = "created"
	StateStarting State = "starting"
	StateReady    State = "ready"
	StateStopping State = "stopping"
	StateStopped  State = "stopped"
	StateFailed   State = "failed"
)

type ProcessSpec struct {
	Binary         string
	Args           []string
	ReloadArgs     []string
	Dir            string
	Env            []string
	DesiredWorkers int
}

type Process interface {
	GracefulStop(context.Context) error
	Kill() error
	Reload() error
	Wait() error
}

type ProcessFactory interface {
	Start(ProcessSpec) (Process, error)
}

type ReadinessProbe interface {
	Check(context.Context) error
}

type Manager struct {
	mu                sync.RWMutex
	factory           ProcessFactory
	spec              ProcessSpec
	process           Process
	state             State
	startedAt         time.Time
	stateChangedAt    time.Time
	lastError         error
	lastErrorCategory string
}

func NewManager(factory ProcessFactory, spec ProcessSpec) *Manager {
	now := time.Now().UTC()
	return &Manager{factory: factory, spec: spec, state: StateCreated, stateChangedAt: now}
}

func (m *Manager) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

func (m *Manager) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	if m.state != StateCreated && m.state != StateStopped {
		state := m.state
		m.mu.Unlock()
		return fmt.Errorf("cannot start runtime from state %q", state)
	}
	m.state = StateStarting
	m.stateChangedAt = time.Now().UTC()
	m.lastError = nil
	m.lastErrorCategory = ""
	m.mu.Unlock()

	process, err := m.factory.Start(m.spec)
	if err != nil {
		m.fail(err, "process_failed")
		return fmt.Errorf("start runtime: %w", err)
	}
	m.mu.Lock()
	m.process = process
	m.startedAt = time.Now().UTC()
	m.mu.Unlock()
	metrics.RoadRunnerStarts.Inc()
	go m.monitor(process)
	return nil
}

func (m *Manager) monitor(process Process) {
	err := process.Wait()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.process != process || m.state == StateStopping || m.state == StateStopped {
		return
	}
	m.state = StateFailed
	m.stateChangedAt = time.Now().UTC()
	m.lastError = err
	m.lastErrorCategory = "process_failed"
	metrics.RoadRunnerCrashes.Inc()
}

func (m *Manager) MarkReady() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state != StateStarting {
		return fmt.Errorf("cannot mark runtime ready from state %q", m.state)
	}
	m.state = StateReady
	m.stateChangedAt = time.Now().UTC()
	return nil
}

func (m *Manager) WaitReady(ctx context.Context, probe ReadinessProbe, interval time.Duration) error {
	if probe == nil {
		return errors.New("readiness probe is required")
	}
	if interval <= 0 {
		return errors.New("readiness probe interval must be positive")
	}
	for {
		if err := m.failureError(); err != nil {
			return err
		}
		if err := probe.Check(ctx); err == nil {
			return m.MarkReady()
		}
		if err := m.failureError(); err != nil {
			return err
		}
		timer := time.NewTimer(interval)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			m.fail(ctx.Err(), "timeout")
			metrics.RoadRunnerReadinessTimeouts.Inc()
			return fmt.Errorf("%w: %v", ErrReadinessTimeout, ctx.Err())
		}
	}
}

func (m *Manager) failureError() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.state != StateFailed {
		return nil
	}
	if m.lastError == nil {
		return ErrRuntimeFailed
	}
	return fmt.Errorf("%w: %s", ErrRuntimeFailed, m.lastErrorCategory)
}

func (m *Manager) Reload(context.Context) error {
	m.mu.RLock()
	process := m.process
	state := m.state
	m.mu.RUnlock()
	if state != StateReady {
		return fmt.Errorf("cannot reload runtime from state %q", state)
	}
	if process == nil {
		return fmt.Errorf("runtime process is not available")
	}
	if err := process.Reload(); err != nil {
		return fmt.Errorf("reload runtime: %w", err)
	}
	return nil
}

func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	if m.state == StateStopped || m.state == StateCreated {
		m.state = StateStopped
		m.stateChangedAt = time.Now().UTC()
		m.mu.Unlock()
		return nil
	}
	if m.state != StateStarting && m.state != StateReady && m.state != StateFailed {
		state := m.state
		m.mu.Unlock()
		return fmt.Errorf("cannot stop runtime from state %q", state)
	}
	process := m.process
	wasFailed := m.state == StateFailed
	m.state = StateStopping
	m.stateChangedAt = time.Now().UTC()
	m.mu.Unlock()

	if process == nil {
		m.markStopped()
		metrics.RoadRunnerStops.Inc()
		return nil
	}
	if err := process.GracefulStop(ctx); err != nil {
		if killErr := process.Kill(); killErr != nil && !wasFailed {
			m.fail(killErr, "process_failed")
			return fmt.Errorf("stop runtime: %w; kill runtime: %v", err, killErr)
		}
	}
	m.markStopped()
	metrics.RoadRunnerStops.Inc()
	return nil
}

func (m *Manager) markStopped() {
	m.mu.Lock()
	m.state = StateStopped
	m.stateChangedAt = time.Now().UTC()
	m.mu.Unlock()
}

func (m *Manager) Fail(err error) error {
	if err == nil {
		return fmt.Errorf("runtime failure requires an error")
	}
	m.fail(err, "process_failed")
	return err
}

func (m *Manager) fail(err error, category string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == StateStopped {
		return
	}
	m.state = StateFailed
	m.stateChangedAt = time.Now().UTC()
	m.lastError = err
	m.lastErrorCategory = category
}

func (m *Manager) Snapshot() control.RuntimeSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	snapshot := control.RuntimeSnapshot{
		EngineState:       control.EngineStarting,
		ReadinessReason:   control.ReadinessStarting,
		DesiredWorkers:    m.spec.DesiredWorkers,
		WorkerCountsKnown: false,
		StartedAt:         m.startedAt,
		StateChangedAt:    m.stateChangedAt,
		LastErrorCategory: m.lastErrorCategory,
	}
	switch m.state {
	case StateReady:
		snapshot.EngineState = control.EngineRunning
		snapshot.ReadinessReason = control.ReadinessReady
		snapshot.ReadyWorkers = 1
	case StateStopping:
		snapshot.EngineState = control.EngineStopping
		snapshot.ReadinessReason = control.ReadinessStopping
	case StateStopped:
		snapshot.EngineState = control.EngineStopped
		snapshot.ReadinessReason = control.ReadinessStopping
	case StateFailed:
		snapshot.EngineState = control.EngineFailed
		snapshot.ReadinessReason = control.ReadinessProcessFailed
		if m.lastErrorCategory == "timeout" {
			snapshot.ReadinessReason = control.ReadinessTimeout
		}
	}
	return snapshot
}
