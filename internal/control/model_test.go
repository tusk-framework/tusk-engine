package control

import (
	"testing"
	"time"
)

func TestRuntimeSnapshotHealthAndReadiness(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		state   EngineState
		reason  ReadinessReason
		ready   int
		healthy bool
		isReady bool
	}{
		{
			name:    "starting is healthy but not ready",
			state:   EngineStarting,
			reason:  ReadinessStarting,
			healthy: true,
		},
		{
			name:    "running with worker is ready",
			state:   EngineRunning,
			reason:  ReadinessReady,
			ready:   1,
			healthy: true,
			isReady: true,
		},
		{
			name:    "running without worker is not ready",
			state:   EngineRunning,
			reason:  ReadinessNoWorkers,
			healthy: true,
		},
		{
			name:    "stopped is not healthy or ready",
			state:   EngineStopped,
			reason:  ReadinessStopping,
			healthy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := RuntimeSnapshot{
				EngineState:     tt.state,
				ReadinessReason: tt.reason,
				ReadyWorkers:    tt.ready,
				StartedAt:       now,
				StateChangedAt:  now,
			}

			if got := snapshot.Healthy(); got != tt.healthy {
				t.Fatalf("Healthy() = %v, want %v", got, tt.healthy)
			}
			if got := snapshot.Ready(); got != tt.isReady {
				t.Fatalf("Ready() = %v, want %v", got, tt.isReady)
			}
		})
	}
}

func TestRuntimeSnapshotIsValueCopy(t *testing.T) {
	original := RuntimeSnapshot{
		EngineState:     EngineRunning,
		ReadinessReason: ReadinessReady,
		ReadyWorkers:    2,
	}
	copy := original
	copy.ReadyWorkers = 1

	if original.ReadyWorkers != 2 {
		t.Fatalf("changing snapshot copy changed original: %+v", original)
	}
}
