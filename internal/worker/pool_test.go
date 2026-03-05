package worker

import (
	"testing"
	"time"

	"github.com/tusk-framework/tusk-engine/internal/config"
)

func TestPoolConcurrency(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.WorkerCount = 2
	cfg.WorkerCommand = "test_worker.php"
	cfg.ProjectRoot = "./"

	pool, err := NewPool(cfg)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	if err := pool.Start(); err != nil {
		t.Fatalf("Failed to start pool: %v", err)
	}
	defer pool.Stop()

	// Wait for workers to start
	time.Sleep(100 * time.Millisecond)

	// Send 3 requests.
	// Request 1 & 2 should take 500ms each.
	// Since we have 2 workers, they should run in parallel.
	// Request 3 should wait for one of the workers to become free.

	start := time.Now()
	results := make(chan error, 3)

	sendReq := func(sleepMs int) {
		_, err := pool.HandleRequest(map[string]interface{}{"sleep": sleepMs})
		results <- err
	}

	go sendReq(500)
	go sendReq(500)
	go sendReq(1) // This should wait for one of the 500ms ones to finish

	for i := 0; i < 3; i++ {
		if err := <-results; err != nil {
			t.Errorf("Request %d failed: %v", i, err)
		}
	}

	duration := time.Since(start)

	// If concurrent, it should take ~500ms + some overhead.
	// If serial, it would take ~1000ms.
	if duration > 800*time.Millisecond {
		t.Errorf("Execution took too long (%v), requests might be serialized", duration)
	}

	if duration < 500*time.Millisecond {
		t.Errorf("Execution took too short (%v), synchronization might be broken", duration)
	}
}

func TestHeaderRelay(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.WorkerCount = 1
	cfg.WorkerCommand = "test_worker.php"
	cfg.ProjectRoot = "./"

	pool, err := NewPool(cfg)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	if err := pool.Start(); err != nil {
		t.Fatalf("Failed to start pool: %v", err)
	}
	defer pool.Stop()

	time.Sleep(100 * time.Millisecond)

	inputHeaders := map[string][]string{
		"X-Single": {"val1"},
		"X-Multi":  {"val1", "val2"},
	}

	resp, err := pool.HandleRequest(map[string]interface{}{
		"headers": inputHeaders,
	})

	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	respHeaders, ok := resp["headers"].(map[string]interface{})
	if !ok {
		t.Fatalf("Invalid response headers format: %v", resp["headers"])
	}

	// Check X-Multi
	multi, ok := respHeaders["X-Multi"].([]interface{})
	if !ok || len(multi) != 2 {
		t.Errorf("X-Multi header lost values: %v", respHeaders["X-Multi"])
	}
}
