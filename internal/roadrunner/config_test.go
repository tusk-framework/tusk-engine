package roadrunner

import (
	"strings"
	"testing"

	"github.com/tusk-framework/tusk-engine/internal/config"
)

func TestProjectProducesDeterministicRoadRunnerConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Address = "127.0.0.1"
	cfg.Port = 9090
	cfg.WorkerCount = 6
	cfg.WorkerCommand = "vendor/bin/tusk run app.php --runtime=roadrunner"
	cfg.PhpBinary = "php"
	cfg.MaxBodyBytes = 12 * 1024 * 1024

	first, err := Project(cfg)
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	second, err := Project(cfg)
	if err != nil {
		t.Fatalf("Project() second error = %v", err)
	}

	if string(first) != string(second) {
		t.Fatalf("Project() is not deterministic:\n%s\n---\n%s", first, second)
	}
	for _, expected := range []string{
		"version: \"3\"",
		"command: php vendor/bin/tusk run app.php --runtime=roadrunner",
		"address: 127.0.0.1:9090",
		"num_workers: 6",
		"max_request_size: 12",
	} {
		if !strings.Contains(string(first), expected) {
			t.Fatalf("rendered config missing %q:\n%s", expected, first)
		}
	}
}

func TestProjectRejectsInvalidRuntimeInputs(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Port = 0

	if _, err := Project(cfg); err == nil {
		t.Fatal("Project() accepted an invalid port")
	}
}
