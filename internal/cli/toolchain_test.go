package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tusk-framework/tusk-engine/internal/config"
	"github.com/tusk-framework/tusk-engine/internal/toolchain"
)

func TestDoctorJSONOutputIncludesMachineReadableToolStatus(t *testing.T) {
	cfg := &config.Config{ProjectRoot: t.TempDir()}
	report := toolchain.Report{
		ProjectRoot:  cfg.ProjectRoot,
		ManifestPath: filepath.Join(cfg.ProjectRoot, ".tusk", "toolchain.json"),
		Ready:        true,
		Tools: []toolchain.Tool{{
			Name:             string(toolchain.PHP),
			Status:           toolchain.StatusOK,
			Available:        true,
			Path:             filepath.Join(cfg.ProjectRoot, ".tusk", "toolchain", "php"),
			Source:           toolchain.SourceProject,
			Version:          "PHP 8.3.11",
			RequestedVersion: "8.3",
		}},
	}
	var output bytes.Buffer
	err := runDoctorToWith(cfg, []string{"--json"}, &output, func(*config.Config) (toolchain.Report, error) {
		return report, nil
	})
	if err != nil {
		t.Fatalf("runDoctorToWith() error = %v", err)
	}
	var decoded toolchain.Report
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("doctor output is not JSON: %v\n%s", err, output.String())
	}
	if !decoded.Ready || decoded.Tool(toolchain.PHP).RequestedVersion != "8.3" {
		t.Fatalf("doctor report = %#v, want ready PHP report", decoded)
	}
}

func TestToolchainListAndPinCommands(t *testing.T) {
	cfg := &config.Config{ProjectRoot: t.TempDir()}
	report := toolchain.Report{ProjectRoot: cfg.ProjectRoot, Ready: false}
	var output bytes.Buffer
	err := runToolchainCommandTo(cfg, []string{"list", "--json"}, &output, func(*config.Config) (toolchain.Report, error) {
		return report, nil
	})
	if err != nil {
		t.Fatalf("list error = %v", err)
	}
	if !strings.Contains(output.String(), `"ready": false`) {
		t.Fatalf("list output = %q, want JSON report", output.String())
	}

	output.Reset()
	err = runToolchainCommandTo(cfg, []string{"pin", "php@8.3"}, &output, nil)
	if err != nil {
		t.Fatalf("pin error = %v", err)
	}
	manifest, err := toolchain.LoadManifest(cfg.ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.PHP.Version != "8.3" || !strings.Contains(output.String(), "Pinned php@8.3") {
		t.Fatalf("pin output/manifest = %q / %#v", output.String(), manifest)
	}
}

func TestToolchainSetupOfflineRequiresTrustedCatalog(t *testing.T) {
	cfg := &config.Config{ProjectRoot: t.TempDir()}
	var output bytes.Buffer
	err := runToolchainSetupTo(cfg, []string{"--offline"}, &output)
	if err == nil || !strings.Contains(err.Error(), "trusted catalog") {
		t.Fatalf("setup error = %v, want trusted catalog error", err)
	}
	if strings.Contains(output.String(), "Downloading") {
		t.Fatalf("setup attempted network provisioning: %q", output.String())
	}
}

func TestToolchainSetupCodeReturnsNonSuccessOnProvisioningError(t *testing.T) {
	cfg := &config.Config{ProjectRoot: t.TempDir()}
	var output bytes.Buffer
	var errorsOutput bytes.Buffer
	if code := runToolchainSetupCode(cfg, []string{"--offline"}, &output, &errorsOutput); code == 0 {
		t.Fatalf("runToolchainSetupCode() = %d, want non-zero; stderr = %q", code, errorsOutput.String())
	}
}

func TestToolchainCommandsRejectUnknownFlags(t *testing.T) {
	if _, err := parseDoctorArgs([]string{"--wat"}); err == nil {
		t.Fatal("parseDoctorArgs() accepted unknown flag")
	}
	if _, err := parseToolchainSetupArgs([]string{"--offline", "--wat"}); err == nil {
		t.Fatal("parseToolchainSetupArgs() accepted unknown flag")
	}
	if _, err := parseToolchainSetupArgs([]string{"--offline"}); err != nil {
		t.Fatalf("parseToolchainSetupArgs() error = %v", err)
	}
}

func TestToolchainSetupRunsVerifiedProvisioningService(t *testing.T) {
	cfg := &config.Config{ProjectRoot: t.TempDir()}
	catalogPath := filepath.Join(cfg.ProjectRoot, ".tusk", "toolchain.catalog.json")
	if err := os.MkdirAll(filepath.Dir(catalogPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(catalogPath, []byte("test catalog"), 0o600); err != nil {
		t.Fatal(err)
	}
	var gotOptions toolchain.ProvisionOptions
	original := provisionToolchain
	t.Cleanup(func() { provisionToolchain = original })
	provisionToolchain = func(_ context.Context, gotConfig *config.Config, options toolchain.ProvisionOptions) (toolchain.SetupReport, error) {
		if gotConfig != cfg {
			t.Fatalf("setup config = %p, want %p", gotConfig, cfg)
		}
		gotOptions = options
		return toolchain.SetupReport{Provision: toolchain.ProvisionReport{
			Ready: true,
			Results: []toolchain.ToolProvisionResult{{
				Name:    toolchain.PHP,
				Version: "8.3.0",
				Path:    filepath.Join(cfg.ProjectRoot, ".tusk", "toolchain", "php", "php"),
				Status:  toolchain.StatusProvisioned,
			}},
		}}, nil
	}

	var output bytes.Buffer
	if err := runToolchainSetupTo(cfg, []string{"--offline"}, &output); err != nil {
		t.Fatalf("runToolchainSetupTo() error = %v", err)
	}
	if !gotOptions.Offline {
		t.Fatal("setup did not pass offline option to provisioning service")
	}
	if !strings.Contains(output.String(), "Provisioned php@8.3.0") {
		t.Fatalf("setup output = %q, want provision result", output.String())
	}
}

func TestBuildToolchainCommandUsesResolvedComposerAndIsolatedEnvironment(t *testing.T) {
	cfg := &config.Config{ProjectRoot: t.TempDir()}
	phpPath := filepath.Join(cfg.ProjectRoot, ".tusk", "toolchain", "php", "php.exe")
	composerPath := filepath.Join(cfg.ProjectRoot, ".tusk", "toolchain", "composer", "composer.phar")
	report := toolchain.Report{
		ProjectRoot: cfg.ProjectRoot,
		Platform:    toolchain.Platform{OS: "windows", Arch: "amd64"},
		Tools: []toolchain.Tool{
			{Name: string(toolchain.PHP), Available: true, Path: phpPath, Source: toolchain.SourceProject},
			{Name: string(toolchain.Composer), Available: true, Path: composerPath, Source: toolchain.SourceProject},
		},
	}
	originalPath := `C:\Windows\System32`
	t.Setenv("PATH", originalPath)

	cmd, err := buildToolchainCommand(cfg, report, toolchain.Composer, []string{"install"})
	if err != nil {
		t.Fatalf("buildToolchainCommand() error = %v", err)
	}
	if cmd.Path != phpPath || len(cmd.Args) < 3 || cmd.Args[1] != composerPath || cmd.Args[2] != "install" {
		t.Fatalf("Composer command = path %q args %#v, want PHP + PHAR + install", cmd.Path, cmd.Args)
	}
	if got := commandEnvironmentValue(cmd.Env, "PATH"); !strings.HasPrefix(got, filepath.Dir(phpPath)+";") {
		t.Fatalf("command PATH = %q, want project-local prefix", got)
	}
	if os.Getenv("PATH") != originalPath {
		t.Fatalf("parent PATH changed to %q", os.Getenv("PATH"))
	}
}

func commandEnvironmentValue(env []string, key string) string {
	for _, item := range env {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], key) {
			return parts[1]
		}
	}
	return ""
}
