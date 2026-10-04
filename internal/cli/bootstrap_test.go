package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tusk-framework/tusk-engine/internal/config"
	"github.com/tusk-framework/tusk-engine/internal/toolchain"
)

func writeValidBootstrap(t *testing.T, root string) {
	t.Helper()
	if _, err := exec.LookPath("php"); err != nil {
		t.Skip("PHP is unavailable for bootstrap execution")
	}
	writeBootstrap(t, root, "<?php return new \\Tusk\\Foundation\\Application();")
	autoload := filepath.Join(root, "vendor", "autoload.php")
	if err := os.MkdirAll(filepath.Dir(autoload), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(autoload, []byte("<?php namespace Tusk\\Foundation; class Application {}"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeBootstrap(t *testing.T, root, contents string) {
	t.Helper()
	path := filepath.Join(root, "bootstrap", "app.php")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestStartRejectsMissingBootstrapBeforeProcessCreation(t *testing.T) {
	root := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.ProjectRoot = root
	factory := &observingFactory{}
	err := runServerWithConfigUsing(cfg, factory, nil)
	if err == nil || !strings.Contains(err.Error(), filepath.Join(root, "bootstrap", "app.php")) || !strings.Contains(err.Error(), "tusk migrate") {
		t.Fatalf("missing bootstrap error = %v", err)
	}
	if factory.started {
		t.Fatal("RoadRunner started without bootstrap")
	}
}

func TestStartRejectsInvalidBootstrapBeforeProcessCreation(t *testing.T) {
	if _, err := exec.LookPath("php"); err != nil {
		t.Skip("PHP is unavailable for bootstrap execution")
	}
	root := t.TempDir()
	writeBootstrap(t, root, "<?php return new \\stdClass();")
	autoload := filepath.Join(root, "vendor", "autoload.php")
	if err := os.MkdirAll(filepath.Dir(autoload), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(autoload, []byte("<?php namespace Tusk\\Foundation; class Application {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.ProjectRoot = root
	factory := &observingFactory{}
	err := runServerWithConfigUsing(cfg, factory, nil)
	if err == nil || !strings.Contains(err.Error(), "stdClass") || !strings.Contains(err.Error(), "Tusk\\Foundation\\Application") {
		t.Fatalf("invalid bootstrap error = %v", err)
	}
	if factory.started {
		t.Fatal("RoadRunner started with invalid bootstrap")
	}
}

func TestStartRejectsPositionalWorkerWithMigrationMessage(t *testing.T) {
	for _, command := range []string{"start", "dev"} {
		if err := validateStartArgs([]string{"file.php"}); err == nil || !strings.Contains(err.Error(), "bootstrap/app.php") || !strings.Contains(err.Error(), "migrat") {
			t.Fatalf("%s custom worker error = %v", command, err)
		}
	}
}

func TestStartReportsLegacyRootWorkerWithoutBootstrap(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "worker.php"), []byte("<?php"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.ProjectRoot = root
	factory := &observingFactory{}
	err := runServerWithConfigUsing(cfg, factory, func(string, toolchain.ToolName) (toolchain.Tool, error) {
		t.Fatal("RoadRunner resolved before bootstrap validation")
		return toolchain.Tool{}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "worker.php") || !strings.Contains(err.Error(), "migrat") || factory.started {
		t.Fatalf("legacy project result = %v, started = %t", err, factory.started)
	}
}
