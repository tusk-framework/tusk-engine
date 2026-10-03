package toolchain

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiagnosePrefersProjectToolsAndFallsBackToSystem(t *testing.T) {
	root := t.TempDir()
	projectPHP := filepath.Join(root, ".tusk", "bin", "php")
	if err := os.MkdirAll(filepath.Dir(projectPHP), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectPHP, []byte("php"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"php":{"version":"8.3","path":".tusk/bin/php"},"composer":{"version":"2"},"roadrunner":{"version":"2025"}}`
	if err := os.WriteFile(filepath.Join(root, ".tusk", "toolchain.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	system := map[ToolName]string{
		Composer:   "/usr/local/bin/composer",
		RoadRunner: "/usr/local/bin/rr",
	}
	versions := map[string]string{
		projectPHP:                "PHP 8.3.11",
		"/usr/local/bin/composer": "Composer version 2.8.11",
		"/usr/local/bin/rr":       "rr version 2025.1.0",
	}

	report, err := Diagnose(DiagnosticOptions{
		Root: root,
		Lookup: func(name string) (string, error) {
			path, ok := system[ToolName(name)]
			if !ok {
				return "", errors.New("not found")
			}
			return path, nil
		},
		Version: func(path string) (string, error) {
			return versions[path], nil
		},
	})
	if err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}
	if !report.Ready {
		t.Fatalf("expected ready report, got %#v", report)
	}

	php := report.Tool(PHP)
	if php.Path != projectPHP || php.Source != SourceProject {
		t.Fatalf("PHP = %#v, want project tool at %q", php, projectPHP)
	}
	if composer := report.Tool(Composer); composer.Source != SourceSystem || composer.Version != versions["/usr/local/bin/composer"] {
		t.Fatalf("Composer = %#v, want system tool", composer)
	}
	if rr := report.Tool(RoadRunner); rr.Source != SourceSystem {
		t.Fatalf("RoadRunner = %#v, want system tool", rr)
	}
}

func TestDiagnoseMarksMissingAndVersionMismatch(t *testing.T) {
	report, err := Diagnose(DiagnosticOptions{
		Root: t.TempDir(),
		Lookup: func(name string) (string, error) {
			if name == string(PHP) {
				return "C:/tools/php.exe", nil
			}
			return "", errors.New("not found")
		},
		Version: func(path string) (string, error) {
			return "PHP 8.2.0", nil
		},
		Manifest: Manifest{
			PHP: ToolSpec{Version: "8.3"},
		},
	})
	if err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}
	if report.Ready {
		t.Fatal("expected report to be not ready")
	}
	if got := report.Tool(PHP).Status; got != StatusVersionMismatch {
		t.Fatalf("PHP status = %q, want %q", got, StatusVersionMismatch)
	}
	if got := report.Tool(Composer).Status; got != StatusMissing {
		t.Fatalf("Composer status = %q, want %q", got, StatusMissing)
	}
}

func TestLoadManifestReportsMalformedJSON(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".tusk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".tusk", "toolchain.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadManifest(root)
	if err == nil || !strings.Contains(err.Error(), "parse toolchain manifest") {
		t.Fatalf("LoadManifest() error = %v, want parse error", err)
	}
}

func TestReportJSONIsStableAndUseful(t *testing.T) {
	report := Report{
		ProjectRoot:  "C:/project",
		ManifestPath: "C:/project/.tusk/toolchain.json",
		Tools: []Tool{
			{Name: string(PHP), Status: StatusOK, Available: true, Path: "C:/php.exe", Source: SourceSystem, Version: "PHP 8.3.11"},
		},
		Ready: true,
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(data)
	for _, fragment := range []string{`"project_root":"C:/project"`, `"ready":true`, `"name":"php"`, `"status":"ok"`} {
		if !strings.Contains(encoded, fragment) {
			t.Fatalf("JSON %q does not contain %q", encoded, fragment)
		}
	}
}
