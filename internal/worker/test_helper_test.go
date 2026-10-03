package worker

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/tusk-framework/tusk-engine/internal/config"
)

func TestMain(m *testing.M) {
	if os.Getenv("TUSK_TEST_WORKER") == "1" {
		runTestWorker()
		return
	}
	os.Exit(m.Run())
}

func runTestWorker() {
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			return
		}
		if query, ok := request["query"].(map[string]interface{}); ok {
			if rawSleep, ok := query["sleep"]; ok {
				if sleep, err := strconv.Atoi(fmt.Sprint(rawSleep)); err == nil {
					time.Sleep(time.Duration(sleep) * time.Millisecond)
				}
			}
		}
		responseHeaders := map[string][]string{"X-Test-Worker": {"ok"}}
		if headers, ok := request["headers"].(map[string]interface{}); ok {
			for name, raw := range headers {
				values, ok := raw.([]interface{})
				if !ok {
					continue
				}
				for _, value := range values {
					responseHeaders[name] = append(responseHeaders[name], fmt.Sprint(value))
				}
			}
		}
		_ = encoder.Encode(map[string]interface{}{
			"status":  200,
			"headers": responseHeaders,
			"body":    "ok",
		})
		if os.Getenv("TUSK_TEST_EXIT_AFTER_REQUEST") == "1" {
			return
		}
	}
}

func newTestPool(t *testing.T, workerCount int) *Pool {
	t.Helper()
	root := t.TempDir()
	workerFile := root + string(os.PathSeparator) + "worker.php"
	if err := os.WriteFile(workerFile, []byte("test"), 0600); err != nil {
		t.Fatalf("write test worker: %v", err)
	}
	cfg := config.DefaultConfig()
	cfg.WorkerCount = workerCount
	cfg.PhpBinary = os.Args[0]
	cfg.WorkerCommand = "worker.php"
	cfg.ProjectRoot = root

	commandFactory := func(binary string, args ...string) *exec.Cmd {
		cmd := exec.Command(binary, args...)
		cmd.Env = append(os.Environ(), "TUSK_TEST_WORKER=1")
		return cmd
	}
	pool, err := newPoolWithCommandFactory(cfg, commandFactory)
	if err != nil {
		t.Fatalf("create test pool: %v", err)
	}
	return pool
}
