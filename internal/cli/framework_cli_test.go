package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tusk-framework/tusk-engine/internal/config"
)

func TestFrameworkCommandRunsComposerProxyWithArgumentsAndOriginalProcessContext(t *testing.T) {
	root := t.TempDir()
	entrypoint := filepath.Join(root, "vendor", "bin", "tusk")
	if err := os.MkdirAll(filepath.Dir(entrypoint), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entrypoint, []byte("<?php // Composer proxy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	phpBinary := buildFrameworkPHPTestHelper(t)
	cfg := config.DefaultConfig()
	cfg.ProjectRoot = root
	cfg.PhpBinary = phpBinary
	t.Setenv("TUSK_TEST_PHP_EXIT_CODE", "37")

	absoluteEntrypoint, err := filepath.Abs(entrypoint)
	if err != nil {
		t.Fatal(err)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	for _, forwardedArgs := range [][]string{
		{"list"},
		{"make:controller", "Example Name", "--option=a&b"},
	} {
		t.Run(strings.Join(forwardedArgs, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := runFrameworkCommand(cfg, forwardedArgs, strings.NewReader("stdin payload\n"), &stdout, &stderr)
			var exitError *exec.ExitError
			if !errors.As(err, &exitError) || exitError.ExitCode() != 37 {
				t.Fatalf("runFrameworkCommand() error = %v, want child exit code 37", err)
			}

			var result struct {
				Args  []string `json:"args"`
				Dir   string   `json:"dir"`
				Stdin string   `json:"stdin"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatalf("decode child result %q: %v", stdout.String(), err)
			}
			wantArgs := append([]string{absoluteEntrypoint}, forwardedArgs...)
			if !equalStrings(result.Args, wantArgs) {
				t.Errorf("child args = %#v, want %#v", result.Args, wantArgs)
			}
			if result.Dir != workingDirectory {
				t.Errorf("child working directory = %q, want inherited %q", result.Dir, workingDirectory)
			}
			if result.Stdin != "stdin payload\n" {
				t.Errorf("child stdin = %q, want forwarded input", result.Stdin)
			}
			if !strings.Contains(stderr.String(), "child stderr") {
				t.Errorf("child stderr was not forwarded: %q", stderr.String())
			}
		})
	}
}

func TestResolveFrameworkCLIPrefersComposerPHPProxyToWindowsBatchWrapper(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "vendor", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	phpProxy := filepath.Join(binDir, "tusk")
	batchWrapper := filepath.Join(binDir, "tusk.bat")
	if err := os.WriteFile(phpProxy, []byte("<?php // portable Composer proxy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(batchWrapper, []byte("@echo off\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveFrameworkCLI(root, "windows")
	if err != nil {
		t.Fatalf("resolveFrameworkCLI() error = %v", err)
	}
	absolutePHPProxy, err := filepath.Abs(phpProxy)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != absolutePHPProxy {
		t.Fatalf("resolved entrypoint = %q, want Composer PHP proxy %q", resolved, absolutePHPProxy)
	}
}

func TestResolveFrameworkCLIExplainsMissingComposerProxyInsteadOfRunningBatchWrapper(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "vendor", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "tusk.bat"), []byte("@echo off\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := resolveFrameworkCLI(root, "windows")
	if err == nil || !strings.Contains(err.Error(), "Composer PHP proxy") || !strings.Contains(err.Error(), "tusk install") {
		t.Fatalf("resolveFrameworkCLI() error = %v, want actionable Windows proxy guidance", err)
	}
}

func TestResolveFrameworkCLIRequiresComposerInstalledFrameworkEntrypoint(t *testing.T) {
	_, err := resolveFrameworkCLI(t.TempDir(), runtime.GOOS)
	if err == nil || !strings.Contains(err.Error(), "vendor/bin/tusk") || !strings.Contains(err.Error(), "tusk install") {
		t.Fatalf("resolveFrameworkCLI() error = %v, want actionable missing dependency guidance", err)
	}
}

func TestResolveFrameworkCLIDoesNotExecuteRootScriptsThatOnlyMatchCommandNames(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"tusk", "console"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("unrelated executable"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	_, err := resolveFrameworkCLI(root, runtime.GOOS)
	if err == nil || !strings.Contains(err.Error(), "vendor/bin/tusk") {
		t.Fatalf("resolveFrameworkCLI() error = %v, want only the Composer-installed Framework entrypoint", err)
	}
}

func buildFrameworkPHPTestHelper(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	packageDir := filepath.Dir(sourceFile)
	binary := filepath.Join(t.TempDir(), "php-test-helper")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	command := exec.Command("go", "build", "-o", binary, "./testdata/php-helper")
	command.Dir = packageDir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build PHP test helper: %v\n%s", err, output)
	}
	return binary
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
