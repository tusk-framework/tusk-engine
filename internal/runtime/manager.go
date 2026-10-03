package runtime

import (
	"context"
	"fmt"
	"sync"
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
	Binary string
	Args   []string
	Dir    string
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

type Manager struct {
	mu      sync.RWMutex
	factory ProcessFactory
	spec    ProcessSpec
	process Process
	state   State
}

func NewManager(factory ProcessFactory, spec ProcessSpec) *Manager {
	return &Manager{
		factory: factory,
		spec:    spec,
		state:   StateCreated,
	}
}

func (m *Manager) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.state
}

func (m *Manager) Start(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state != StateCreated && m.state != StateStopped {
		return fmt.Errorf("cannot start runtime from state %q", m.state)
	}

	m.state = StateStarting
	process, err := m.factory.Start(m.spec)
	if err != nil {
		m.state = StateFailed
		return fmt.Errorf("start runtime: %w", err)
	}

	m.process = process
	return nil
}

func (m *Manager) MarkReady() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state != StateStarting {
		return fmt.Errorf("cannot mark runtime ready from state %q", m.state)
	}

	m.state = StateReady
	return nil
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
		m.mu.Unlock()
		return nil
	}
	if m.state != StateStarting && m.state != StateReady && m.state != StateFailed {
		state := m.state
		m.mu.Unlock()
		return fmt.Errorf("cannot stop runtime from state %q", state)
	}

	process := m.process
	m.state = StateStopping
	m.mu.Unlock()

	if process == nil {
		m.mu.Lock()
		m.state = StateStopped
		m.mu.Unlock()
		return nil
	}

	if err := process.GracefulStop(ctx); err != nil {
		if killErr := process.Kill(); killErr != nil {
			m.mu.Lock()
			m.state = StateFailed
			m.mu.Unlock()
			return fmt.Errorf("stop runtime: %w; kill runtime: %v", err, killErr)
		}
	}

	m.mu.Lock()
	m.state = StateStopped
	m.mu.Unlock()
	return nil
}

func (m *Manager) Fail(err error) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err == nil {
		return fmt.Errorf("runtime failure requires an error")
	}
	if m.state == StateStopped {
		return fmt.Errorf("cannot fail a stopped runtime")
	}

	m.state = StateFailed
	return err
}
