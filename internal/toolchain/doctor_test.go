package toolchain

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
			if name == "rr" {
				name = string(RoadRunner)
			}
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

func TestResolveExecutablePrefersProjectToolAndRejectsMissingOrMismatched(t *testing.T) {
	root := t.TempDir()
	projectRR := filepath.Join(root, ".tusk", "bin", "rr")
	if err := os.MkdirAll(filepath.Dir(projectRR), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectRR, []byte("rr"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{RoadRunner: ToolSpec{Path: ".tusk/bin/rr", Version: "2025"}}

	tool, err := ResolveExecutableWithOptions(root, RoadRunner, DiagnosticOptions{
		Manifest: manifest,
		Lookup:   func(string) (string, error) { return "/system/rr", nil },
		Version: func(path string) (string, error) {
			if path == projectRR {
				return "rr version 2025.1", nil
			}
			return "rr version 2024.1", nil
		},
	})
	if err != nil {
		t.Fatalf("ResolveExecutableWithOptions() error = %v", err)
	}
	if tool.Path != projectRR || tool.Source != SourceProject {
		t.Fatalf("resolved RoadRunner = %#v, want project executable", tool)
	}

	_, err = ResolveExecutableWithOptions(root, RoadRunner, DiagnosticOptions{
		Manifest: Manifest{RoadRunner: ToolSpec{Version: "2025"}},
		Lookup:   func(string) (string, error) { return "", errors.New("missing") },
		Version:  func(string) (string, error) { return "", nil },
	})
	if err == nil || !strings.Contains(err.Error(), "RoadRunner") {
		t.Fatalf("missing RoadRunner error = %v", err)
	}

	_, err = ResolveExecutableWithOptions(root, RoadRunner, DiagnosticOptions{
		Manifest: Manifest{RoadRunner: ToolSpec{Path: projectRR, Version: "2026"}},
		Version:  func(string) (string, error) { return "rr version 2025.1", nil },
	})
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("version mismatch error = %v", err)
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

func TestPinWritesOnlyTheRequestedVersionAndPreservesPath(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".tusk"), 0o755); err != nil {
		t.Fatal(err)
	}
	initial := `{"php":{"version":"8.2","path":".tusk/bin/php"}}`
	if err := os.WriteFile(filepath.Join(root, ".tusk", "toolchain.json"), []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	manifest, err := Pin(root, PHP, "8.3")
	if err != nil {
		t.Fatalf("Pin() error = %v", err)
	}
	if manifest.PHP.Version != "8.3" || manifest.PHP.Path != ".tusk/bin/php" {
		t.Fatalf("PHP manifest = %#v, want version and path preserved", manifest.PHP)
	}

	loaded, err := LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PHP != manifest.PHP {
		t.Fatalf("loaded manifest = %#v, want %#v", loaded.PHP, manifest.PHP)
	}
}

func TestParsePinRejectsUnknownToolsAndMalformedValues(t *testing.T) {
	for _, input := range []string{"php", "unknown@1", "php@", "@8.3"} {
		if _, _, err := ParsePin(input); err == nil {
			t.Fatalf("ParsePin(%q) expected error", input)
		}
	}

	name, version, err := ParsePin("roadrunner@2025.1")
	if err != nil {
		t.Fatal(err)
	}
	if name != RoadRunner || version != "2025.1" {
		t.Fatalf("ParsePin() = %q, %q", name, version)
	}
}

func TestManifestDefaultsToSystemProfileAndRuntimeTarget(t *testing.T) {
	manifest := Manifest{}

	if got := manifest.EffectiveProfile(); got != ProfileSystem {
		t.Fatalf("EffectiveProfile() = %q, want %q", got, ProfileSystem)
	}
	target := manifest.Target(runtime.GOOS, runtime.GOARCH)
	if target.OS != runtime.GOOS || target.Arch != runtime.GOARCH {
		t.Fatalf("Target() = %#v, want runtime target", target)
	}
}

func TestManifestValidatesProfilesAndTargets(t *testing.T) {
	if err := (Manifest{Profile: Profile("workstation")}).Validate(); err == nil || !strings.Contains(err.Error(), "profile") {
		t.Fatal("Validate() accepted unknown profile")
	}
	if err := (Manifest{Platform: Platform{OS: "linux/windows", Arch: "amd64"}}).Validate(); err == nil || !strings.Contains(err.Error(), "platform") {
		t.Fatal("Validate() accepted unsafe platform")
	}
	if err := (Manifest{Profile: ProfileCI, PHP: ToolSpec{Version: "8.3"}}).Validate(); err != nil {
		t.Fatalf("Validate() rejected valid CI manifest: %v", err)
	}
}

func TestDiagnoseReportsProfileAndTarget(t *testing.T) {
	report, err := Diagnose(DiagnosticOptions{
		Root:     t.TempDir(),
		Manifest: Manifest{Profile: ProfileCI, Platform: Platform{OS: "windows", Arch: "amd64"}},
		Lookup: func(name string) (string, error) {
			return "C:/tools/" + name + ".exe", nil
		},
		Version: func(string) (string, error) { return "tool 1.0", nil },
	})
	if err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}
	if report.Profile != string(ProfileCI) || report.Platform.OS != "windows" || report.Platform.Arch != "amd64" {
		t.Fatalf("report profile/target = %q/%#v, want ci/windows-amd64", report.Profile, report.Platform)
	}
}

func TestResolveRejectsProjectPathOutsideRoot(t *testing.T) {
	_, err := ResolveExecutableWithOptions(t.TempDir(), RoadRunner, DiagnosticOptions{
		Manifest: Manifest{RoadRunner: ToolSpec{Path: "../outside/rr"}},
		Lookup:   func(string) (string, error) { return "", errors.New("not found") },
	})
	if err == nil || !strings.Contains(err.Error(), "project path") {
		t.Fatalf("ResolveExecutableWithOptions() error = %v, want project path diagnostic", err)
	}
}
