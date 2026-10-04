package worker

import (
	"testing"

	"github.com/tusk-framework/tusk-engine/internal/config"
	"github.com/tusk-framework/tusk-engine/internal/control"
)

func TestNewPoolSnapshotStartsBeforeWorkers(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.WorkerCount = 3

	pool, err := NewPool(cfg)
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}

	snapshot := pool.Snapshot()
	if snapshot.EngineState != control.EngineStarting {
		t.Fatalf("EngineState = %q, want %q", snapshot.EngineState, control.EngineStarting)
	}
	if snapshot.ReadinessReason != control.ReadinessStarting {
		t.Fatalf("ReadinessReason = %q, want %q", snapshot.ReadinessReason, control.ReadinessStarting)
	}
	if snapshot.DesiredWorkers != 3 {
		t.Fatalf("DesiredWorkers = %d, want 3", snapshot.DesiredWorkers)
	}
	if snapshot.ReadyWorkers != 0 || snapshot.TotalWorkers != 0 {
		t.Fatalf("initial worker counts = ready %d total %d, want zero", snapshot.ReadyWorkers, snapshot.TotalWorkers)
	}
}

func TestPoolSnapshotTracksAvailabilityAndCrashCounters(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.WorkerCount = 2

	pool, err := NewPool(cfg)
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}

	pool.workers = []*Process{{ID: 0}, {ID: 1}}
	pool.workerQueue <- pool.workers[0]
	pool.workerQueue <- pool.workers[1]

	pool.mu.Lock()
	pool.setEngineStateLocked(control.EngineRunning)
	pool.refreshWorkerSnapshotLocked()
	pool.mu.Unlock()

	snapshot := pool.Snapshot()
	if !snapshot.Ready() {
		t.Fatalf("snapshot should be ready: %+v", snapshot)
	}
	if snapshot.ReadyWorkers != 2 || snapshot.ActiveWorkers != 0 || snapshot.TotalWorkers != 2 {
		t.Fatalf("available snapshot = %+v", snapshot)
	}

	<-pool.workerQueue
	pool.mu.Lock()
	pool.refreshWorkerSnapshotLocked()
	pool.mu.Unlock()

	snapshot = pool.Snapshot()
	if snapshot.ReadyWorkers != 1 || snapshot.ActiveWorkers != 1 {
		t.Fatalf("checked-out snapshot = %+v", snapshot)
	}

	pool.mu.Lock()
	for len(pool.workerQueue) > 0 {
		<-pool.workerQueue
	}
	pool.workers = pool.workers[:0]
	pool.recordWorkerFailureLocked("worker_crashed")
	pool.refreshWorkerSnapshotLocked()
	pool.mu.Unlock()

	snapshot = pool.Snapshot()
	if snapshot.Ready() {
		t.Fatalf("crashed pool should not be ready: %+v", snapshot)
	}
	if snapshot.ReadinessReason != control.ReadinessWorkerCrashed {
		t.Fatalf("ReadinessReason = %q, want %q", snapshot.ReadinessReason, control.ReadinessWorkerCrashed)
	}
	if snapshot.WorkerCrashes != 1 {
		t.Fatalf("WorkerCrashes = %d, want 1", snapshot.WorkerCrashes)
	}
}
