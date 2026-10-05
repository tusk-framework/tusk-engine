package toolchain

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestProvisionerInstallsOnlineThenUsesVerifiedCacheOffline(t *testing.T) {
	data := []byte("rr-binary")
	artifact := provisioningArtifact(RoadRunner, "2025.1.0", data)
	artifact.GOOS = "windows"
	artifact.EntryPoint = "rr.exe"
	cache := ArtifactCache{Root: t.TempDir()}
	downloader := &fixtureDownloader{data: data}

	firstRoot := t.TempDir()
	first := Provisioner{
		Catalog:    CatalogPayload{SchemaVersion: 1, Artifacts: []Artifact{artifact}},
		Downloader: downloader,
		Cache:      cache,
		Installer:  Installer{Root: firstRoot, GOOS: "windows", GOARCH: "amd64"},
		GOOS:       "windows",
		GOARCH:     "amd64",
	}
	report := first.Provision(context.Background(), []ToolRequest{{Name: RoadRunner, Version: "2025.1.0"}}, ProvisionOptions{MaxArtifactBytes: 1024})
	if !report.Ready || report.Results[0].Status != StatusProvisioned {
		t.Fatalf("online report = %#v, want provisioned ready report", report)
	}
	if downloader.calls != 1 {
		t.Fatalf("online downloader calls = %d, want 1", downloader.calls)
	}

	secondRoot := t.TempDir()
	offline := Provisioner{
		Catalog:    CatalogPayload{SchemaVersion: 1, Artifacts: []Artifact{artifact}},
		Downloader: &fixtureDownloader{err: errors.New("network must not be used")},
		Cache:      cache,
		Installer:  Installer{Root: secondRoot, GOOS: "windows", GOARCH: "amd64"},
		GOOS:       "windows",
		GOARCH:     "amd64",
	}
	report = offline.Provision(context.Background(), []ToolRequest{{Name: RoadRunner, Version: "2025.1.0"}}, ProvisionOptions{Offline: true, MaxArtifactBytes: 1024})
	if !report.Ready || report.Results[0].Status != StatusProvisioned {
		t.Fatalf("offline report = %#v, want cache-backed provisioned report", report)
	}
	if _, err := os.Stat(report.Results[0].Path); err != nil {
		t.Fatalf("offline installed path = %q: %v", report.Results[0].Path, err)
	}
}

func TestProvisionerReportsCacheMissAndCatalogMismatch(t *testing.T) {
	provisioner := Provisioner{
		Catalog:   CatalogPayload{SchemaVersion: 1},
		Cache:     ArtifactCache{Root: t.TempDir()},
		Installer: Installer{Root: t.TempDir(), GOOS: "linux", GOARCH: "amd64"},
		GOOS:      "linux",
		GOARCH:    "amd64",
	}
	report := provisioner.Provision(context.Background(), []ToolRequest{{Name: PHP, Version: "8.3.0"}}, ProvisionOptions{Offline: true})
	if report.Ready || report.Results[0].Status != StatusFailed || !strings.Contains(report.Results[0].Error, "catalog") {
		t.Fatalf("catalog mismatch report = %#v, want failed catalog result", report)
	}
}

func TestProvisionerRejectsDigestMismatchAndKeepsOtherToolsIndependent(t *testing.T) {
	rrData := []byte("rr-binary")
	phpData := []byte("php-binary")
	rr := provisioningArtifact(RoadRunner, "2025.1.0", rrData)
	php := provisioningArtifact(PHP, "8.3.0", phpData)
	php.SHA256 = strings.Repeat("f", 64)

	provisioner := Provisioner{
		Catalog:    CatalogPayload{SchemaVersion: 1, Artifacts: []Artifact{rr, php}},
		Downloader: &fixtureDownloader{dataByURL: map[string][]byte{rr.URL: rrData, php.URL: phpData}},
		Cache:      ArtifactCache{Root: t.TempDir()},
		Installer:  Installer{Root: t.TempDir(), GOOS: "linux", GOARCH: "amd64"},
		GOOS:       "linux",
		GOARCH:     "amd64",
	}
	report := provisioner.Provision(context.Background(), []ToolRequest{
		{Name: RoadRunner, Version: "2025.1.0"},
		{Name: PHP, Version: "8.3.0"},
	}, ProvisionOptions{MaxArtifactBytes: 1024})
	if report.Ready || len(report.Results) != 2 {
		t.Fatalf("mixed report = %#v, want two non-ready results", report)
	}
	if report.Result(RoadRunner).Status != StatusProvisioned {
		t.Fatalf("RoadRunner result = %#v, want success despite PHP failure", report.Result(RoadRunner))
	}
	if report.Result(PHP).Status != StatusFailed || !strings.Contains(report.Result(PHP).Error, "digest") {
		t.Fatalf("PHP result = %#v, want digest failure", report.Result(PHP))
	}
}

func TestProvisionerSelectsConfiguredPlatformVariant(t *testing.T) {
	data := []byte("ubuntu-php")
	artifact := provisioningArtifact(PHP, "8.3.0", data)
	artifact.GOOS = ""
	artifact.GOARCH = ""
	artifact.Target = Platform{OS: "linux", Arch: "amd64", Distribution: "ubuntu-24.04", Libc: "glibc"}
	p := Provisioner{
		Catalog:    CatalogPayload{SchemaVersion: 1, Artifacts: []Artifact{artifact}},
		Downloader: &fixtureDownloader{data: data},
		Cache:      ArtifactCache{Root: t.TempDir()},
		Installer:  Installer{Root: t.TempDir(), GOOS: "linux", GOARCH: "amd64", Distribution: "ubuntu-24.04", Libc: "glibc"},
		GOOS:       "linux", GOARCH: "amd64", Distribution: "ubuntu-24.04", Libc: "glibc",
	}
	report := p.Provision(context.Background(), []ToolRequest{{Name: PHP, Version: "8.3.0"}}, ProvisionOptions{MaxArtifactBytes: 1024})
	if !report.Ready || report.Results[0].Status != StatusProvisioned {
		t.Fatalf("platform variant report = %#v, want provisioned", report)
	}
}

func TestProvisionerDoesNotUseDistroArtifactForUnspecifiedTarget(t *testing.T) {
	data := []byte("ubuntu-php")
	artifact := provisioningArtifact(PHP, "8.3.0", data)
	artifact.GOOS = ""
	artifact.GOARCH = ""
	artifact.Target = Platform{OS: "linux", Arch: "amd64", Distribution: "ubuntu-24.04", Libc: "glibc"}
	p := Provisioner{Catalog: CatalogPayload{SchemaVersion: 1, Artifacts: []Artifact{artifact}}, Cache: ArtifactCache{Root: t.TempDir()}, GOOS: "linux", GOARCH: "amd64"}
	report := p.Provision(context.Background(), []ToolRequest{{Name: PHP, Version: "8.3.0"}}, ProvisionOptions{Offline: true})
	if report.Ready || !strings.Contains(report.Results[0].Error, "no artifact") {
		t.Fatalf("unspecified target report = %#v, want missing artifact", report)
	}
}

func provisioningArtifact(tool ToolName, version string, data []byte) Artifact {
	return Artifact{
		Tool:       tool,
		Version:    version,
		GOOS:       "linux",
		GOARCH:     "amd64",
		URL:        "https://catalog.example/" + string(tool) + "/" + version,
		SHA256:     digestFor(data),
		Signature:  "catalog-attested",
		Format:     "raw",
		EntryPoint: string(tool),
	}
}

type fixtureDownloader struct {
	data      []byte
	dataByURL map[string][]byte
	err       error
	calls     int
}

func (f *fixtureDownloader) Download(_ context.Context, rawURL string, _ int64) ([]byte, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if f.dataByURL != nil {
		data, ok := f.dataByURL[rawURL]
		if !ok {
			return nil, fmt.Errorf("fixture not found: %s", rawURL)
		}
		return data, nil
	}
	return f.data, nil
}

func (r ProvisionReport) Result(name ToolName) ToolProvisionResult {
	for _, result := range r.Results {
		if result.Name == name {
			return result
		}
	}
	return ToolProvisionResult{Name: name, Status: StatusFailed}
}
