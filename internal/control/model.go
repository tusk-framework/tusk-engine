package control

import "time"

// EngineState represents the lifecycle of the Tusk Engine process.
type EngineState string

const (
	EngineStarting EngineState = "starting"
	EngineRunning  EngineState = "running"
	EngineStopping EngineState = "stopping"
	EngineStopped  EngineState = "stopped"
)

// ReadinessReason explains why the application is or is not ready.
type ReadinessReason string

const (
	ReadinessStarting      ReadinessReason = "starting"
	ReadinessNoWorkers     ReadinessReason = "no_workers"
	ReadinessWorkerCrashed ReadinessReason = "worker_crashed"
	ReadinessReady         ReadinessReason = "ready"
	ReadinessStopping      ReadinessReason = "stopping"
)

// RuntimeSnapshot is an immutable copy of Engine and worker-pool state.
type RuntimeSnapshot struct {
	EngineState       EngineState
	ReadinessReason   ReadinessReason
	DesiredWorkers    int
	ReadyWorkers      int
	ActiveWorkers     int
	TotalWorkers      int
	WorkerCrashes     uint64
	WorkerRestarts    uint64
	StartedAt         time.Time
	StateChangedAt    time.Time
	LastErrorCategory string
}

// SnapshotProvider exposes a point-in-time runtime snapshot.
type SnapshotProvider interface {
	Snapshot() RuntimeSnapshot
}

// Healthy reports whether the Engine process is alive enough for liveness.
func (s RuntimeSnapshot) Healthy() bool {
	return s.EngineState != EngineStopped
}

// Ready reports whether the Engine can accept application traffic.
func (s RuntimeSnapshot) Ready() bool {
	return s.EngineState == EngineRunning &&
		s.ReadinessReason == ReadinessReady &&
		s.ReadyWorkers > 0
}
