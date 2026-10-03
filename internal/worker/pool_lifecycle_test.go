package worker

import (
	"testing"
	"time"
)

func TestPoolTimeoutDoesNotReuseKilledWorker(t *testing.T) {
	pool := newTestPool(t, 1)
	if err := pool.Start(); err != nil {
		t.Fatal(err)
	}
	defer pool.Stop()

	pool.cfg.Timeout = 1
	if _, err := pool.HandleRequest(map[string]interface{}{"query": map[string]string{"sleep": "1500"}}); err == nil {
		t.Fatal("long request unexpectedly succeeded")
	}

	started := time.Now()
	response, err := pool.HandleRequest(map[string]interface{}{})
	if err != nil {
		t.Fatalf("replacement worker request failed: %v", err)
	}
	if response["status"] != float64(200) {
		t.Fatalf("status = %#v, want 200", response["status"])
	}
	if time.Since(started) < 900*time.Millisecond {
		t.Fatalf("request used a replacement too quickly: %v", time.Since(started))
	}
}

func TestPoolStopDoesNotRestartExitedWorker(t *testing.T) {
	pool := newTestPool(t, 1)
	if err := pool.Start(); err != nil {
		t.Fatal(err)
	}
	if err := pool.workers[0].cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	pool.Stop()
	time.Sleep(1200 * time.Millisecond)

	pool.mu.Lock()
	workerCount := len(pool.workers)
	pool.mu.Unlock()
	if workerCount != 0 {
		t.Fatalf("workers after Stop = %d, want 0", workerCount)
	}
}

func TestPoolRestartsWorkerThatExitsAfterRequest(t *testing.T) {
	t.Setenv("TUSK_TEST_EXIT_AFTER_REQUEST", "1")
	pool := newTestPool(t, 1)
	if err := pool.Start(); err != nil {
		t.Fatal(err)
	}
	defer pool.Stop()

	if _, err := pool.HandleRequest(map[string]interface{}{}); err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	time.Sleep(1200 * time.Millisecond)
	if _, err := pool.HandleRequest(map[string]interface{}{}); err != nil {
		t.Fatalf("request after worker restart failed: %v", err)
	}
}
