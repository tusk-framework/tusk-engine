package runtime

import (
	"context"
	"testing"
)

func TestManagerSnapshotMapsLifecycleToControlState(t *testing.T) {
	process := newFakeProcess()
	manager := NewManager(&fakeFactory{process: process}, ProcessSpec{Binary: "rr", DesiredWorkers: 3})
	if snapshot := manager.Snapshot(); snapshot.ReadinessReason == "" || !snapshot.Healthy() {
		t.Fatalf("created snapshot = %+v", snapshot)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if snapshot := manager.Snapshot(); snapshot.EngineState != "starting" {
		t.Fatalf("starting snapshot = %+v", snapshot)
	}
	if err := manager.MarkReady(); err != nil {
		t.Fatal(err)
	}
	ready := manager.Snapshot()
	if ready.EngineState != "running" || !ready.Ready() || ready.DesiredWorkers != 3 || ready.WorkerCountsKnown {
		t.Fatalf("ready snapshot = %+v", ready)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	stopped := manager.Snapshot()
	if stopped.EngineState != "stopped" || stopped.Healthy() || stopped.Ready() {
		t.Fatalf("stopped snapshot = %+v", stopped)
	}
	if stopped.StateChangedAt.Before(stopped.StartedAt) || stopped.StartedAt.IsZero() {
		t.Fatalf("snapshot timestamps = %+v", stopped)
	}
}
