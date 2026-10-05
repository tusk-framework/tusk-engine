package toolchain

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSetupServiceProvisionsPinnedToolsAndPersistsManagedPaths(t *testing.T) {
	root := t.TempDir()
	manifest := Manifest{
		Profile:    ProfileProjectLocal,
		PHP:        ToolSpec{Version: "8.3.0"},
		Composer:   ToolSpec{Version: "2.8.11"},
		RoadRunner: ToolSpec{Version: "2025.1.0"},
	}
	writeSetupManifest(t, root, manifest)

	publicKey, privateKey := setupKeyPair(t)
	artifacts, dataByURL := setupArtifacts(runtime.GOOS, runtime.GOARCH)
	catalogPath := writeSignedSetupCatalog(t, root, artifacts, privateKey)
	downloader := &fixtureDownloader{dataByURL: dataByURL}

	service := SetupService{
		Root:        root,
		CatalogPath: catalogPath,
		Verifier:    CatalogVerifier{PublicKeys: map[string]ed25519.PublicKey{"test-key": publicKey}},
		Downloader:  downloader,
		GOOS:        runtime.GOOS,
		GOARCH:      runtime.GOARCH,
	}

	report, err := service.Run(context.Background(), ProvisionOptions{MaxArtifactBytes: 1024})
	if err != nil {
		t.Fatalf("SetupService.Run() error = %v", err)
	}
	if !report.Provision.Ready || len(report.Provision.Results) != 3 {
		t.Fatalf("provision report = %#v, want three successful results", report.Provision)
	}
	if downloader.calls != 3 {
		t.Fatalf("downloader calls = %d, want one per pinned tool", downloader.calls)
	}

	updated, err := LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		name ToolName
		spec ToolSpec
	}{
		{PHP, updated.PHP},
		{Composer, updated.Composer},
		{RoadRunner, updated.RoadRunner},
	} {
		if item.spec.Path == "" || filepath.IsAbs(item.spec.Path) {
			t.Fatalf("%s manifest path = %q, want relative managed path", item.name, item.spec.Path)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(item.spec.Path))); err != nil {
			t.Fatalf("%s managed path %q is not installed: %v", item.name, item.spec.Path, err)
		}
	}
}

func TestSetupServiceOfflineUsesVerifiedCacheWithoutDownloading(t *testing.T) {
	root := t.TempDir()
	manifest := Manifest{Profile: ProfileProjectLocal, PHP: ToolSpec{Version: "8.3.0"}}
	writeSetupManifest(t, root, manifest)

	publicKey, privateKey := setupKeyPair(t)
	artifacts, dataByURL := setupArtifacts(runtime.GOOS, runtime.GOARCH)
	artifacts = []Artifact{artifacts[0]}
	data := dataByURL[artifacts[0].URL]
	cache := ArtifactCache{Root: filepath.Join(root, ".tusk", "cache")}
	if _, err := cache.Put(artifacts[0].SHA256, data); err != nil {
		t.Fatal(err)
	}
	catalogPath := writeSignedSetupCatalog(t, root, artifacts, privateKey)
	downloader := &fixtureDownloader{err: errors.New("network must not be used")}

	service := SetupService{
		Root:        root,
		CatalogPath: catalogPath,
		Verifier:    CatalogVerifier{PublicKeys: map[string]ed25519.PublicKey{"test-key": publicKey}},
		Downloader:  downloader,
		Cache:       cache,
		GOOS:        runtime.GOOS,
		GOARCH:      runtime.GOARCH,
	}
	report, err := service.Run(context.Background(), ProvisionOptions{Offline: true, MaxArtifactBytes: 1024})
	if err != nil {
		t.Fatalf("offline SetupService.Run() error = %v", err)
	}
	if !report.Provision.Ready || downloader.calls != 0 {
		t.Fatalf("offline report/calls = %#v/%d, want ready with no network", report.Provision, downloader.calls)
	}
}

func TestSetupServicePropagatesPlatformVariantToProvisioning(t *testing.T) {
	root := t.TempDir()
	platform := Platform{OS: "linux", Arch: "amd64", Distribution: "ubuntu-24.04", Libc: "glibc"}
	writeSetupManifest(t, root, Manifest{Profile: ProfileProjectLocal, Platform: platform, PHP: ToolSpec{Version: "8.3.0"}})

	publicKey, privateKey := setupKeyPair(t)
	artifacts, dataByURL := setupArtifacts("linux", "amd64")
	artifacts = artifacts[:1]
	artifacts[0].GOOS = ""
	artifacts[0].GOARCH = ""
	artifacts[0].Target = platform
	catalogPath := writeSignedSetupCatalog(t, root, artifacts, privateKey)

	service := SetupService{
		Root: root, CatalogPath: catalogPath,
		Verifier:   CatalogVerifier{PublicKeys: map[string]ed25519.PublicKey{"test-key": publicKey}},
		Downloader: &fixtureDownloader{dataByURL: dataByURL}, GOOS: "linux", GOARCH: "amd64",
	}
	report, err := service.Run(context.Background(), ProvisionOptions{MaxArtifactBytes: 1024})
	if err != nil {
		t.Fatalf("SetupService.Run() error = %v", err)
	}
	if !strings.Contains(report.Manifest.PHP.Path, filepath.ToSlash(filepath.Join("linux-amd64-ubuntu-24.04-glibc", "php"))) {
		t.Fatalf("managed PHP path = %q, want platform variant path", report.Manifest.PHP.Path)
	}
}

func TestSetupServiceRejectsUntrustedCatalogBeforeProvisioning(t *testing.T) {
	root := t.TempDir()
	manifest := Manifest{Profile: ProfileProjectLocal, PHP: ToolSpec{Version: "8.3.0"}}
	writeSetupManifest(t, root, manifest)
	catalogPath := filepath.Join(root, ".tusk", "toolchain.catalog.json")
	if err := os.WriteFile(catalogPath, []byte(`{"payload":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	originalManifest := readSetupManifestBytes(t, root)
	downloader := &fixtureDownloader{data: []byte("must not download")}

	service := SetupService{
		Root:        root,
		CatalogPath: catalogPath,
		Verifier:    CatalogVerifier{PublicKeys: map[string]ed25519.PublicKey{"test-key": make(ed25519.PublicKey, ed25519.PublicKeySize)}},
		Downloader:  downloader,
		GOOS:        runtime.GOOS,
		GOARCH:      runtime.GOARCH,
	}
	_, err := service.Run(context.Background(), ProvisionOptions{})
	if err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("untrusted catalog error = %v, want signature failure", err)
	}
	if downloader.calls != 0 {
		t.Fatalf("downloader calls = %d, want zero after catalog rejection", downloader.calls)
	}
	if got := readSetupManifestBytes(t, root); string(got) != string(originalManifest) {
		t.Fatalf("manifest changed after catalog rejection: %q -> %q", originalManifest, got)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".tusk", "cache")); !os.IsNotExist(statErr) {
		t.Fatalf("cache directory exists after catalog rejection: %v", statErr)
	}
}

func TestSetupServiceDoesNotPersistPartialProvisioning(t *testing.T) {
	root := t.TempDir()
	manifest := Manifest{
		Profile:    ProfileProjectLocal,
		PHP:        ToolSpec{Version: "8.3.0"},
		RoadRunner: ToolSpec{Version: "2025.1.0"},
	}
	writeSetupManifest(t, root, manifest)
	originalManifest := readSetupManifestBytes(t, root)

	publicKey, privateKey := setupKeyPair(t)
	artifacts, dataByURL := setupArtifacts(runtime.GOOS, runtime.GOARCH)
	artifacts = []Artifact{artifacts[0], artifacts[2]}
	delete(dataByURL, artifacts[1].URL)
	catalogPath := writeSignedSetupCatalog(t, root, artifacts, privateKey)
	downloader := &fixtureDownloader{dataByURL: dataByURL}

	service := SetupService{
		Root:        root,
		CatalogPath: catalogPath,
		Verifier:    CatalogVerifier{PublicKeys: map[string]ed25519.PublicKey{"test-key": publicKey}},
		Downloader:  downloader,
		GOOS:        runtime.GOOS,
		GOARCH:      runtime.GOARCH,
	}
	report, err := service.Run(context.Background(), ProvisionOptions{MaxArtifactBytes: 1024})
	if err == nil || report.Provision.Ready {
		t.Fatalf("partial setup result = %#v, error = %v, want failure", report.Provision, err)
	}
	if got := readSetupManifestBytes(t, root); string(got) != string(originalManifest) {
		t.Fatalf("manifest changed after partial provisioning: %q -> %q", originalManifest, got)
	}
}

func TestSetupServicePreservesExplicitToolPaths(t *testing.T) {
	root := t.TempDir()
	explicitPath := filepath.Join(root, "vendor", "php")
	if err := os.MkdirAll(filepath.Dir(explicitPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(explicitPath, []byte("user managed"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{Profile: ProfileProjectLocal, PHP: ToolSpec{Version: "8.3.0", Path: explicitPath}}
	writeSetupManifest(t, root, manifest)
	publicKey, privateKey := setupKeyPair(t)
	artifacts, _ := setupArtifacts(runtime.GOOS, runtime.GOARCH)
	catalogPath := writeSignedSetupCatalog(t, root, artifacts[:1], privateKey)

	service := SetupService{
		Root:        root,
		CatalogPath: catalogPath,
		Verifier:    CatalogVerifier{PublicKeys: map[string]ed25519.PublicKey{"test-key": publicKey}},
		Downloader:  &fixtureDownloader{err: errors.New("must not download")},
		GOOS:        runtime.GOOS,
		GOARCH:      runtime.GOARCH,
	}
	if _, err := service.Run(context.Background(), ProvisionOptions{Offline: true}); err != nil {
		t.Fatalf("SetupService.Run() error = %v", err)
	}
	updated, err := LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if updated.PHP.Path != explicitPath {
		t.Fatalf("explicit PHP path = %q, want %q", updated.PHP.Path, explicitPath)
	}
	data, err := os.ReadFile(explicitPath)
	if err != nil || string(data) != "user managed" {
		t.Fatalf("explicit PHP binary changed: %q / %v", data, err)
	}
}

func TestSafeRelativePathRejectsParentSegmentsAfterNormalization(t *testing.T) {
	for _, value := range []string{"bin/../rr", "bin//../rr", "./bin/../rr"} {
		if _, err := safeRelativePath(value); err == nil || !strings.Contains(err.Error(), "traversal") {
			t.Fatalf("safeRelativePath(%q) error = %v, want traversal rejection", value, err)
		}
	}
}

func setupKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return publicKey, privateKey
}

func setupArtifacts(goos, goarch string) ([]Artifact, map[string][]byte) {
	entries := []struct {
		name    ToolName
		version string
		data    []byte
		entry   string
	}{
		{PHP, "8.3.0", []byte("php-binary"), executableEntry(goos, "php")},
		{Composer, "2.8.11", []byte("composer-phar"), "composer.phar"},
		{RoadRunner, "2025.1.0", []byte("rr-binary"), executableEntry(goos, "rr")},
	}
	artifacts := make([]Artifact, 0, len(entries))
	dataByURL := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		artifact := provisioningArtifact(entry.name, entry.version, entry.data)
		artifact.GOOS = goos
		artifact.GOARCH = goarch
		artifact.EntryPoint = entry.entry
		artifacts = append(artifacts, artifact)
		dataByURL[artifact.URL] = entry.data
	}
	return artifacts, dataByURL
}

func executableEntry(goos, name string) string {
	if strings.EqualFold(goos, "windows") {
		return name + ".exe"
	}
	return name
}

func writeSignedSetupCatalog(t *testing.T, root string, artifacts []Artifact, privateKey ed25519.PrivateKey) string {
	t.Helper()
	payload := CatalogPayload{
		SchemaVersion:  1,
		CatalogVersion: "test",
		IssuedAt:       time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
		ExpiresAt:      time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		AllowedHosts:   []string{"catalog.example"},
		Artifacts:      artifacts,
	}
	data := marshalSignedCatalog(t, payload, "test-key", privateKey)
	path := filepath.Join(root, ".tusk", "toolchain.catalog.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeSetupManifest(t *testing.T, root string, manifest Manifest) {
	t.Helper()
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".tusk", "toolchain.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readSetupManifestBytes(t *testing.T, root string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".tusk", "toolchain.json"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
