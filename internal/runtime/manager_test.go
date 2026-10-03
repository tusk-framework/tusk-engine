package runtime

import (
	"context"
	"errors"
	"testing"
)

func TestManagerTracksRoadRunnerLifecycle(t *testing.T) {
	process := &fakeProcess{}
	manager := NewManager(&fakeFactory{process: process}, ProcessSpec{Binary: "rr"})

	if got := manager.State(); got != StateCreated {
		t.Fatalf("initial state = %s, want %s", got, StateCreated)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got := manager.State(); got != StateStarting {
		t.Fatalf("state after Start = %s, want %s", got, StateStarting)
	}
	if err := manager.MarkReady(); err != nil {
		t.Fatalf("MarkReady() error = %v", err)
	}
	if err := manager.Reload(context.Background()); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if !process.reloaded {
		t.Fatal("Reload() did not reach the child process")
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if got := manager.State(); got != StateStopped {
		t.Fatalf("final state = %s, want %s", got, StateStopped)
	}
}

func TestManagerMarksFailedWhenProcessCannotStart(t *testing.T) {
	manager := NewManager(&fakeFactory{err: errors.New("rr unavailable")}, ProcessSpec{Binary: "rr"})

	if err := manager.Start(context.Background()); err == nil {
		t.Fatal("Start() succeeded unexpectedly")
	}
	if got := manager.State(); got != StateFailed {
		t.Fatalf("state after failed Start = %s, want %s", got, StateFailed)
	}
}

type fakeFactory struct {
	process Process
	err     error
}

func (f *fakeFactory) Start(ProcessSpec) (Process, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.process, nil
}

type fakeProcess struct {
	reloaded bool
}

func (p *fakeProcess) GracefulStop(context.Context) error { return nil }
func (p *fakeProcess) Kill() error                        { return nil }
func (p *fakeProcess) Reload() error {
	p.reloaded = true
	return nil
}
func (p *fakeProcess) Wait() error { return nil }
