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

func TestDoctorReportsMissingModernBootstrap(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{ProjectRoot: root}
	diagnose := func(*config.Config) (toolchain.Report, error) { return toolchain.Report{ProjectRoot: root}, nil }
	var output bytes.Buffer
	if err := runDoctorToWith(cfg, nil, &output, diagnose); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"bootstrap/app.php", "modern"} {
		if !strings.Contains(output.String(), value) {
			t.Fatalf("doctor missing %q: %s", value, output.String())
		}
	}
	for _, value := range []string{"legacy", "tusk migrate", "worker.php"} {
		if strings.Contains(strings.ToLower(output.String()), strings.ToLower(value)) {
			t.Fatalf("doctor advertised removed legacy contract %q: %s", value, output.String())
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
	if !ok || project["state"] != "missing" {
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
	var output bytes.Buffer
	diagnoseErr := fmt.Errorf("parse toolchain manifest: malformed JSON")
	err := runDoctorToWith(&config.Config{ProjectRoot: root}, nil, &output, func(*config.Config) (toolchain.Report, error) {
		return toolchain.Report{}, diagnoseErr
	})
	if err == nil || !strings.Contains(err.Error(), "malformed JSON") {
		t.Fatalf("doctor error = %v", err)
	}
	for _, value := range []string{"toolchain", "bootstrap/app.php"} {
		if !strings.Contains(output.String(), value) {
			t.Fatalf("doctor output missing %q: %s", value, output.String())
		}
	}
}

func TestStartUsesOnlyModernBootstrapDiagnostic(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "worker.php"), []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.ProjectRoot = root
	err := validateBootstrap(cfg)
	if err == nil || !strings.Contains(err.Error(), "bootstrap/app.php") {
		t.Fatalf("start modern bootstrap error = %v", err)
	}
	for _, value := range []string{"worker.php", "tusk migrate", "legacy"} {
		if strings.Contains(strings.ToLower(err.Error()), strings.ToLower(value)) {
			t.Fatalf("start error advertised removed legacy contract %q: %v", value, err)
		}
	}
}
