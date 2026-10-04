package cli

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tusk-framework/tusk-engine/internal/components"
	"github.com/tusk-framework/tusk-engine/internal/config"
	engineRuntime "github.com/tusk-framework/tusk-engine/internal/runtime"
	"github.com/tusk-framework/tusk-engine/internal/toolchain"
)

func TestNewComponentRegistryActivatesFirstPartyProviders(t *testing.T) {
	registry, err := newComponentRegistry()
	if err != nil {
		t.Fatalf("newComponentRegistry() error = %v", err)
	}
	if err := registry.Activate(context.Background(), nil); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	for _, check := range []struct {
		name       string
		capability components.Capability
	}{
		{name: components.BuiltInInvocationComponentName, capability: components.CapabilityServiceInvocation},
		{name: components.BuiltInResilienceComponentName, capability: components.CapabilityResilience},
	} {
		if _, err := registry.Resolve(check.name, check.capability); err != nil {
			t.Fatalf("Resolve(%q, %q) error = %v", check.name, check.capability, err)
		}
	}
}

func TestRunServerWithConfigRejectsInvalidRuntimeBeforeResolution(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ProjectRoot = t.TempDir()
	cfg.Runtime.StatusAddress = "0.0.0.0:2114"

	err := runServerWithConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "invalid runtime configuration") {
		t.Fatalf("runServerWithConfig() error = %v, want validation failure", err)
	}
}

type runningFactory struct {
	started chan engineRuntime.ProcessSpec
	exited  chan struct{}
}

func (f *runningFactory) Start(spec engineRuntime.ProcessSpec) (engineRuntime.Process, error) {
	f.started <- spec
	return runningProcess{exited: f.exited}, nil
}

type runningProcess struct{ exited <-chan struct{} }

func (p runningProcess) Wait() error                        { <-p.exited; return errors.New("process exited") }
func (p runningProcess) GracefulStop(context.Context) error { return nil }
func (p runningProcess) Kill() error                        { return nil }
func (p runningProcess) Reload() error                      { return nil }

func TestStartKeepsGeneratedWorkerDuringRunningProcessAndCleansOnExit(t *testing.T) {
	root := t.TempDir()
	writeValidBootstrap(t, root)
	status := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer status.Close()
	cfg := config.DefaultConfig()
	cfg.ProjectRoot = root
	cfg.Runtime.StatusAddress = strings.TrimPrefix(status.URL, "http://")
	factory := &runningFactory{started: make(chan engineRuntime.ProcessSpec), exited: make(chan struct{})}
	result := make(chan error, 1)
	go func() {
		result <- runServerWithConfigUsing(cfg, factory, func(string, toolchain.ToolName) (toolchain.Tool, error) {
			return toolchain.Tool{Path: "rr-test"}, nil
		})
	}()
	select {
	case <-factory.started:
	case err := <-result:
		t.Fatalf("server exited before process start: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start process")
	}
	workerPath := filepath.Join(root, ".tusk", "runtime", "worker.php")
	if _, err := os.Stat(workerPath); err != nil {
		t.Fatalf("generated worker absent while process runs: %v", err)
	}
	close(factory.exited)
	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "RoadRunner") {
			t.Fatalf("server exit error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop after process exit")
	}
	if _, err := os.Stat(workerPath); !os.IsNotExist(err) {
		t.Fatalf("generated worker remains after process exit: %v", err)
	}
}

func TestRunServerWithConfigDoesNotFallbackWhenRoadRunnerIsMissing(t *testing.T) {
	root := t.TempDir()
	writeValidBootstrap(t, root)
	phpPath, err := exec.LookPath("php")
	if err != nil {
		t.Skip("PHP is unavailable for bootstrap execution")
	}
	cfg := config.DefaultConfig()
	cfg.ProjectRoot = root
	cfg.PhpBinary = phpPath
	t.Setenv("PATH", root)

	err = runServerWithConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "resolve RoadRunner") {
		t.Fatalf("runServerWithConfig() error = %v, want RoadRunner resolution failure", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "native") || strings.Contains(strings.ToLower(err.Error()), "worker pool") {
		t.Fatalf("missing RoadRunner must not fall back to native runtime: %v", err)
	}
}

type observingFactory struct {
	started bool
	check   func(engineRuntime.ProcessSpec) error
}

func (f *observingFactory) Start(spec engineRuntime.ProcessSpec) (engineRuntime.Process, error) {
	f.started = true
	if f.check != nil {
		if err := f.check(spec); err != nil {
			return nil, err
		}
	}
	return nil, errors.New("injected process start failure")
}

func TestStartUsesGeneratedWorkerUntilProcessStops(t *testing.T) {
	root := t.TempDir()
	writeValidBootstrap(t, root)
	if err := os.WriteFile(filepath.Join(root, "worker.php"), []byte("legacy user worker"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.ProjectRoot = root
	cfg.WorkerCommand = "worker.php"
	factory := &observingFactory{check: func(spec engineRuntime.ProcessSpec) error {
		if spec.Dir != root || spec.Binary != "rr-test" {
			t.Fatalf("process spec = %+v", spec)
		}
		workerPath := filepath.Join(root, ".tusk", "runtime", "worker.php")
		if _, err := os.Stat(workerPath); err != nil {
			t.Fatalf("generated worker missing while starting RoadRunner: %v", err)
		}
		projected, err := os.ReadFile(spec.Args[2])
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(projected), "command: php .tusk/runtime/worker.php") {
			t.Fatalf("RoadRunner config uses wrong worker: %s", projected)
		}
		return nil
	}}
	err := runServerWithConfigUsing(cfg, factory, func(string, toolchain.ToolName) (toolchain.Tool, error) {
		return toolchain.Tool{Path: "rr-test"}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "injected process start failure") || !factory.started {
		t.Fatalf("start result = %v, factory started = %t", err, factory.started)
	}
	if _, err := os.Stat(filepath.Join(root, ".tusk", "runtime", "worker.php")); !os.IsNotExist(err) {
		t.Fatalf("generated worker remains after RoadRunner stop: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "worker.php"))
	if err != nil || string(data) != "legacy user worker" {
		t.Fatalf("legacy worker changed: %q, %v", data, err)
	}
}
