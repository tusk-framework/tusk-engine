package cli

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestToolchainSetupWithoutPinsDoesNotRequireCatalogOrNetwork(t *testing.T) {
	cfg := &config.Config{ProjectRoot: t.TempDir()}
	var output bytes.Buffer
	fetchCalled := false
	provisionCalled := false
	dependencies, _, _ := testSetupDependencies(t, func(context.Context, string) ([]byte, error) {
		fetchCalled = true
		return nil, fmt.Errorf("unexpected fetch")
	}, func(context.Context, *config.Config, toolchain.ProvisionOptions, string, toolchain.CatalogVerifier) (toolchain.SetupReport, error) {
		provisionCalled = true
		return toolchain.SetupReport{}, nil
	})
	if err := runToolchainSetupToWith(cfg, []string{"--offline"}, &output, dependencies); err != nil {
		t.Fatalf("runToolchainSetupToWith() error = %v", err)
	}
	if fetchCalled || provisionCalled {
		t.Fatalf("setup called fetch=%v or provision=%v without pinned tools", fetchCalled, provisionCalled)
	}
}

func TestToolchainSetupCodeReturnsNonSuccessOnProvisioningError(t *testing.T) {
	cfg := &config.Config{ProjectRoot: t.TempDir()}
	writeCLIToolchainManifest(t, cfg.ProjectRoot, toolchain.Manifest{Profile: toolchain.ProfileProjectLocal, PHP: toolchain.ToolSpec{Version: "8.3"}})
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
	if _, err := parseToolchainSetupArgs([]string{"--toolchain", "--offline"}); err != nil {
		t.Fatalf("parseToolchainSetupArgs() rejected the documented setup command: %v", err)
	}
}

func TestToolchainSetupRunsVerifiedProvisioningService(t *testing.T) {
	cfg := &config.Config{ProjectRoot: t.TempDir()}
	writeCLIToolchainManifest(t, cfg.ProjectRoot, toolchain.Manifest{Profile: toolchain.ProfileProjectLocal, PHP: toolchain.ToolSpec{Version: "8.3"}})
	var gotOptions toolchain.ProvisionOptions
	dependencies, catalogData, _ := testSetupDependencies(t, nil, func(_ context.Context, gotConfig *config.Config, options toolchain.ProvisionOptions, _ string, _ toolchain.CatalogVerifier) (toolchain.SetupReport, error) {
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
	})
	if err := toolchain.WriteCatalogAtomic(cfg.ProjectRoot, catalogData); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := runToolchainSetupToWith(cfg, []string{"--offline"}, &output, dependencies); err != nil {
		t.Fatalf("runToolchainSetupTo() error = %v", err)
	}
	if !gotOptions.Offline {
		t.Fatal("setup did not pass offline option to provisioning service")
	}
	if !strings.Contains(output.String(), "Provisioned php@8.3.0") {
		t.Fatalf("setup output = %q, want provision result", output.String())
	}
}

func TestToolchainSetupOnlineFetchesVerifiesAndCachesBeforeProvisioning(t *testing.T) {
	cfg := &config.Config{ProjectRoot: t.TempDir()}
	writeCLIToolchainManifest(t, cfg.ProjectRoot, toolchain.Manifest{Profile: toolchain.ProfileProjectLocal, PHP: toolchain.ToolSpec{Version: "8.3"}})
	fetches, provisions := 0, 0
	var catalogData []byte
	var catalogDigest string
	var dependencies toolchainSetupDependencies
	dependencies, catalogData, catalogDigest = testSetupDependencies(t, func(_ context.Context, version string) ([]byte, error) {
		fetches++
		if version != "0.3.2" {
			t.Fatalf("release version = %q, want normalized 0.3.2", version)
		}
		return catalogData, nil
	}, func(_ context.Context, _ *config.Config, _ toolchain.ProvisionOptions, digest string, _ toolchain.CatalogVerifier) (toolchain.SetupReport, error) {
		provisions++
		if digest != catalogDigest {
			t.Fatalf("digest = %q, want embedded catalog digest", digest)
		}
		cached, err := os.ReadFile(filepath.Join(cfg.ProjectRoot, ".tusk", "toolchain.catalog.json"))
		if err != nil || !bytes.Equal(cached, catalogData) {
			t.Fatalf("catalog was not cached before provision: %v", err)
		}
		return toolchain.SetupReport{Provision: toolchain.ProvisionReport{Ready: true, Results: []toolchain.ToolProvisionResult{{Name: toolchain.PHP, Version: "8.3", Path: "php", Status: toolchain.StatusProvisioned}}}}, nil
	})
	dependencies.releaseVersion = func() string { return "0.3.2" }
	var output bytes.Buffer
	if err := runToolchainSetupToWith(cfg, nil, &output, dependencies); err != nil {
		t.Fatalf("runToolchainSetupToWith() error = %v", err)
	}
	if fetches != 1 || provisions != 1 {
		t.Fatalf("fetch/provision calls = %d/%d, want 1/1", fetches, provisions)
	}
}

func TestToolchainSetupOfflineNeverFetchesAndRejectsDigestMismatch(t *testing.T) {
	cfg := &config.Config{ProjectRoot: t.TempDir()}
	writeCLIToolchainManifest(t, cfg.ProjectRoot, toolchain.Manifest{Profile: toolchain.ProfileProjectLocal, PHP: toolchain.ToolSpec{Version: "8.3"}})
	if err := toolchain.WriteCatalogAtomic(cfg.ProjectRoot, []byte("stale catalog")); err != nil {
		t.Fatal(err)
	}
	fetches, provisions := 0, 0
	dependencies, _, _ := testSetupDependencies(t, func(context.Context, string) ([]byte, error) {
		fetches++
		return nil, nil
	}, func(context.Context, *config.Config, toolchain.ProvisionOptions, string, toolchain.CatalogVerifier) (toolchain.SetupReport, error) {
		provisions++
		return toolchain.SetupReport{}, nil
	})
	var output bytes.Buffer
	err := runToolchainSetupToWith(cfg, []string{"--offline"}, &output, dependencies)
	if err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("offline setup error = %v, want digest mismatch", err)
	}
	if fetches != 0 || provisions != 0 {
		t.Fatalf("offline setup called fetch=%d provision=%d, want neither", fetches, provisions)
	}
}

func TestToolchainSetupOnlineFailurePreservesOldCatalogAndSkipsProvision(t *testing.T) {
	cfg := &config.Config{ProjectRoot: t.TempDir()}
	writeCLIToolchainManifest(t, cfg.ProjectRoot, toolchain.Manifest{Profile: toolchain.ProfileProjectLocal, PHP: toolchain.ToolSpec{Version: "8.3"}})
	original := []byte("old catalog")
	if err := toolchain.WriteCatalogAtomic(cfg.ProjectRoot, original); err != nil {
		t.Fatal(err)
	}
	provisions := 0
	dependencies, _, _ := testSetupDependencies(t, func(context.Context, string) ([]byte, error) { return nil, fmt.Errorf("offline network") }, func(context.Context, *config.Config, toolchain.ProvisionOptions, string, toolchain.CatalogVerifier) (toolchain.SetupReport, error) {
		provisions++
		return toolchain.SetupReport{}, nil
	})
	var output bytes.Buffer
	if err := runToolchainSetupToWith(cfg, nil, &output, dependencies); err == nil {
		t.Fatal("online setup unexpectedly succeeded")
	}
	if provisions != 0 {
		t.Fatalf("provision calls = %d, want 0", provisions)
	}
	got, err := os.ReadFile(filepath.Join(cfg.ProjectRoot, ".tusk", "toolchain.catalog.json"))
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("old catalog changed after failed fetch: %q, %v", got, err)
	}
}

func testSetupDependencies(t *testing.T, fetch func(context.Context, string) ([]byte, error), provision func(context.Context, *config.Config, toolchain.ProvisionOptions, string, toolchain.CatalogVerifier) (toolchain.SetupReport, error)) (toolchainSetupDependencies, []byte, string) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	artifact := toolchain.Artifact{Tool: toolchain.PHP, Version: "8.3", GOOS: "windows", GOARCH: "amd64", URL: "https://catalog.example/php.zip", SHA256: strings.Repeat("a", 64), Format: "zip", EntryPoint: "php.exe"}
	payload := toolchain.CatalogPayload{SchemaVersion: 1, CatalogVersion: "2026.10.0", IssuedAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339), ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339), AllowedHosts: []string{"catalog.example"}, Artifacts: []toolchain.Artifact{artifact}}
	data, err := toolchain.SignCatalog(payload, "test", privateKey, map[string]ed25519.PublicKey{"test": publicKey})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	digestHex := hex.EncodeToString(digest[:])
	return toolchainSetupDependencies{
		releaseVersion: func() string { return "0.3.2" },
		digest:         func() (string, error) { return digestHex, nil },
		verifier: func() (toolchain.CatalogVerifier, error) {
			return toolchain.CatalogVerifier{PublicKeys: map[string]ed25519.PublicKey{"test": publicKey}, RequireValidity: true}, nil
		},
		fetch:     fetch,
		provision: provision,
	}, data, digestHex
}

func writeCLIToolchainManifest(t *testing.T, root string, manifest toolchain.Manifest) {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".tusk", "toolchain.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
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
