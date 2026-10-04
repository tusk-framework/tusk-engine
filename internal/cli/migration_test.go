package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tusk-framework/tusk-engine/internal/config"
	"github.com/tusk-framework/tusk-engine/internal/toolchain"
)

func TestDoctorReportsLegacyWorkerAndMigrationAction(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "worker.php"), []byte("not executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{ProjectRoot: root}
	diagnose := func(*config.Config) (toolchain.Report, error) { return toolchain.Report{ProjectRoot: root}, nil }
	var output bytes.Buffer
	if err := runDoctorToWith(cfg, nil, &output, diagnose); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"worker.php", "RoadRunner", "tusk migrate"} {
		if !strings.Contains(output.String(), value) {
			t.Fatalf("doctor missing %q: %s", value, output.String())
		}
	}
	output.Reset()
	if err := runDoctorToWith(cfg, []string{"--json"}, &output, diagnose); err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	project, ok := report["project"].(map[string]any)
	if !ok || project["state"] != "legacy" {
		t.Fatalf("JSON doctor project = %#v", report["project"])
	}
}

func TestDoctorRecognizesModernBootstrap(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "bootstrap"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bootstrap", "app.php"), []byte("<?php"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err := runDoctorToWith(&config.Config{ProjectRoot: root}, nil, &output, func(*config.Config) (toolchain.Report, error) { return toolchain.Report{}, nil })
	if err != nil || strings.Contains(strings.ToLower(output.String()), "legacy") {
		t.Fatalf("doctor = %q, %v", output.String(), err)
	}
}

func TestDoctorReportsProjectDiagnosisWhenToolchainDiagnosisFails(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "worker.php"), []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	diagnoseErr := fmt.Errorf("parse toolchain manifest: malformed JSON")
	err := runDoctorToWith(&config.Config{ProjectRoot: root}, nil, &output, func(*config.Config) (toolchain.Report, error) {
		return toolchain.Report{}, diagnoseErr
	})
	if err == nil || !strings.Contains(err.Error(), "malformed JSON") {
		t.Fatalf("doctor error = %v", err)
	}
	for _, value := range []string{"toolchain", "worker.php", "tusk migrate"} {
		if !strings.Contains(output.String(), value) {
			t.Fatalf("doctor output missing %q: %s", value, output.String())
		}
	}
}

func TestMigrateCommandReportsEachCreatedPath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "worker.php"), []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := runMigrateTo(&config.Config{ProjectRoot: root}, nil, &output); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bootstrap/app.php", "bootstrap/providers.php", "config/app.php", "routes/web.php", "public/index.php"} {
		if !strings.Contains(output.String(), name) {
			t.Fatalf("missing created path %s: %q", name, output.String())
		}
	}
}

func TestStartLegacyDiagnosticPointsToRealCommands(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "worker.php"), []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := rejectLegacyWorker(root)
	if err == nil || !strings.Contains(err.Error(), "tusk doctor") || !strings.Contains(err.Error(), "tusk migrate") {
		t.Fatalf("start legacy error = %v", err)
	}
}
