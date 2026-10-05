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
	cfg.WorkerCommand = "worker.php"
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
		"command: php .tusk/runtime/worker.php",
		"address: 127.0.0.1:9090",
		"num_workers: 6",
		"max_request_size: 12",
		"address: 127.0.0.1:2114",
		"listen: tcp://127.0.0.1:6001",
	} {
		if !strings.Contains(string(first), expected) {
			t.Fatalf("rendered config missing %q:\n%s", expected, first)
		}
	}
}

func TestProjectIgnoresLegacyWorkerCommand(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.WorkerCommand = "worker.php"
	cfg.PhpBinary = "custom-php"
	cfg.ProjectRoot = "/srv/tusk-app"
	projected, err := Project(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(projected), "command: custom-php /srv/tusk-app/.tusk/runtime/worker.php") || strings.Contains(string(projected), "command: worker.php") {
		t.Fatalf("configured PHP binary or generated worker contract is wrong: %s", projected)
	}
}

func TestProjectEnablesLoopbackPrometheusMetrics(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Runtime.MetricsAddress = "127.0.0.1:9211"

	projected, err := Project(cfg)
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	contents := string(projected)
	for _, expected := range []string{
		"middleware:",
		"- http_metrics",
		"metrics:",
		"address: 127.0.0.1:9211",
	} {
		if !strings.Contains(contents, expected) {
			t.Fatalf("rendered config missing %q:\n%s", expected, contents)
		}
	}
}

func TestProjectUsesSafeMetricsDefaults(t *testing.T) {
	projected, err := Project(config.DefaultConfig())
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	contents := string(projected)
	for _, expected := range []string{"http_metrics", "address: 127.0.0.1:2112"} {
		if !strings.Contains(contents, expected) {
			t.Fatalf("safe metrics default missing %q:\n%s", expected, contents)
		}
	}
}

func TestProjectRejectsRemoteRuntimeControlEndpoints(t *testing.T) {
	for _, mutate := range []func(*config.Config){
		func(cfg *config.Config) { cfg.Runtime.StatusAddress = "0.0.0.0:2114" },
		func(cfg *config.Config) { cfg.Runtime.RPCAddress = "tcp://example.test:6001" },
	} {
		cfg := config.DefaultConfig()
		mutate(cfg)
		if _, err := Project(cfg); err == nil {
			t.Fatal("Project() accepted a remote runtime control endpoint")
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
