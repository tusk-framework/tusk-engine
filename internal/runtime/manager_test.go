package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tusk-framework/tusk-engine/internal/metrics"
)

func TestManagerTracksRoadRunnerLifecycle(t *testing.T) {
	process := newFakeProcess()
	manager := NewManager(&fakeFactory{process: process}, ProcessSpec{Binary: "rr"})

	if got := manager.State(); got != StateCreated {
		t.Fatalf("initial state = %s, want %s", got, StateCreated)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	starts := gatheredCounter(t, "tusk_roadrunner_starts_total")
	if starts < 1 {
		t.Fatalf("RoadRunnerStarts = %v, want at least one start", starts)
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
	if got := gatheredCounter(t, "tusk_roadrunner_stops_total"); got < 1 {
		t.Fatalf("RoadRunnerStops = %v, want at least one stop", got)
	}
	process.exit(nil)
	if got := manager.State(); got != StateStopped {
		t.Fatalf("final state = %s, want %s", got, StateStopped)
	}
}

func TestManagerWaitReadyRetriesUntilProbeSucceeds(t *testing.T) {
	process := newFakeProcess()
	manager := NewManager(&fakeFactory{process: process}, ProcessSpec{Binary: "rr", DesiredWorkers: 4})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	probe := &sequenceProbe{errors: []error{ErrNotReady, ErrNotReady, nil}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := manager.WaitReady(ctx, probe, time.Millisecond); err != nil {
		t.Fatalf("WaitReady() error = %v", err)
	}
	snapshot := manager.Snapshot()
	if !snapshot.Ready() || snapshot.DesiredWorkers != 4 || snapshot.WorkerCountsKnown {
		t.Fatalf("snapshot = %+v, want ready with unknown exact counters", snapshot)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestManagerMarksUnexpectedProcessExitAsFailed(t *testing.T) {
	process := newFakeProcess()
	manager := NewManager(&fakeFactory{process: process}, ProcessSpec{Binary: "rr"})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	process.exit(errors.New("child exited"))
	deadline := time.Now().Add(time.Second)
	for manager.State() != StateFailed && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if manager.State() != StateFailed {
		t.Fatalf("state = %s, want failed", manager.State())
	}
	if got := gatheredCounter(t, "tusk_roadrunner_crashes_total"); got < 1 {
		t.Fatalf("RoadRunnerCrashes = %v, want at least one crash", got)
	}
	if got := manager.Snapshot().LastErrorCategory; got != "process_failed" {
		t.Fatalf("error category = %q, want process_failed", got)
	}
}

func TestManagerKillsAfterGracefulStopDeadline(t *testing.T) {
	process := newFakeProcess()
	process.gracefulBlocks = true
	manager := NewManager(&fakeFactory{process: process}, ProcessSpec{Binary: "rr"})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := manager.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if !process.killed {
		t.Fatal("Stop() did not kill after graceful stop deadline")
	}
	if manager.State() != StateStopped {
		t.Fatalf("state = %s, want stopped", manager.State())
	}
}

func TestManagerWaitReadyTimesOutWithStableCategory(t *testing.T) {
	process := newFakeProcess()
	manager := NewManager(&fakeFactory{process: process}, ProcessSpec{Binary: "rr"})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := manager.WaitReady(ctx, &sequenceProbe{errors: []error{ErrNotReady}}, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("WaitReady() error = %v, want timeout", err)
	}
	if manager.State() != StateFailed || manager.Snapshot().LastErrorCategory != "timeout" {
		t.Fatalf("snapshot = %+v, want failed timeout", manager.Snapshot())
	}
	if got := gatheredCounter(t, "tusk_roadrunner_readiness_timeouts_total"); got < 1 {
		t.Fatalf("RoadRunnerReadinessTimeouts = %v, want at least one timeout", got)
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
	spec    ProcessSpec
}

func (f *fakeFactory) Start(spec ProcessSpec) (Process, error) {
	f.spec = spec
	if f.err != nil {
		return nil, f.err
	}
	return f.process, nil
}

type fakeProcess struct {
	reloaded       bool
	killed         bool
	gracefulBlocks bool
	waitCh         chan error
}

func newFakeProcess() *fakeProcess {
	return &fakeProcess{waitCh: make(chan error, 1)}
}

func (p *fakeProcess) GracefulStop(ctx context.Context) error {
	if p.gracefulBlocks {
		<-ctx.Done()
		return ctx.Err()
	}
	p.exit(nil)
	return nil
}
func (p *fakeProcess) Kill() error {
	p.killed = true
	p.exit(nil)
	return nil
}
func (p *fakeProcess) Reload() error {
	p.reloaded = true
	return nil
}
func (p *fakeProcess) Wait() error { return <-p.waitCh }

func (p *fakeProcess) exit(err error) {
	select {
	case p.waitCh <- err:
	default:
	}
}

type sequenceProbe struct {
	errors []error
	index  int
}

func gatheredCounter(t *testing.T, name string) float64 {
	t.Helper()
	families, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	for _, family := range families {
		if family.GetName() == name && len(family.Metric) > 0 {
			return family.Metric[0].GetCounter().GetValue()
		}
	}
	return 0
}

func (p *sequenceProbe) Check(context.Context) error {
	if p.index >= len(p.errors) {
		return p.errors[len(p.errors)-1]
	}
	err := p.errors[p.index]
	p.index++
	return err
}
