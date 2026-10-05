package toolchain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadCatalog reads one catalog and authenticates it before returning payload
// data to a provisioner.
func LoadCatalog(path string, verifier CatalogVerifier) (CatalogPayload, error) {
	if strings.TrimSpace(path) == "" {
		return CatalogPayload{}, errors.New("catalog path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return CatalogPayload{}, fmt.Errorf("read toolchain catalog: %w", err)
	}
	payload, err := verifier.Verify(data)
	if err != nil {
		return CatalogPayload{}, fmt.Errorf("verify toolchain catalog: %w", err)
	}
	return payload, nil
}

// SetupReport contains the independent provisioning outcomes and the manifest
// used for the run. The manifest is only updated after all requested tools
// have been installed successfully.
type SetupReport struct {
	Manifest  Manifest        `json:"manifest"`
	Provision ProvisionReport `json:"provision"`
}

// SetupService performs explicit project-local toolchain setup.
type SetupService struct {
	Root        string
	CatalogPath string
	Verifier    CatalogVerifier
	Downloader  Downloader
	Cache       ArtifactCache
	Installer   Installer
	GOOS        string
	GOARCH      string
}

// Run loads a verified catalog, provisions only pinned tools without explicit
// paths, and atomically persists the resulting managed paths. It never writes
// the manifest after a partial provisioning failure.
func (s SetupService) Run(ctx context.Context, options ProvisionOptions) (SetupReport, error) {
	root, err := filepath.Abs(s.Root)
	if err != nil {
		return SetupReport{}, fmt.Errorf("resolve project root: %w", err)
	}
	manifest, err := LoadManifest(root)
	if err != nil {
		return SetupReport{}, err
	}
	if err := manifest.Validate(); err != nil {
		return SetupReport{}, err
	}
	requests, err := setupRequests(manifest)
	if err != nil {
		return SetupReport{}, err
	}

	goos := s.GOOS
	goarch := s.GOARCH
	target := manifest.Target(goos, goarch)
	if err := (Manifest{Profile: manifest.EffectiveProfile(), Platform: target}).Validate(); err != nil {
		return SetupReport{}, err
	}

	catalogPath := s.CatalogPath
	if catalogPath == "" {
		catalogPath = filepath.Join(root, ".tusk", "toolchain.catalog.json")
	}
	catalog, err := LoadCatalog(catalogPath, s.Verifier)
	if err != nil {
		return SetupReport{}, err
	}

	cache := s.Cache
	if cache.Root == "" {
		cache.Root = filepath.Join(root, ".tusk", "cache")
	}
	installer := s.Installer
	if installer.Root == "" {
		installer.Root = root
	}
	if installer.GOOS == "" {
		installer.GOOS = target.OS
	}
	if installer.GOARCH == "" {
		installer.GOARCH = target.Arch
	}
	if installer.Distribution == "" {
		installer.Distribution = target.Distribution
	}
	if installer.Libc == "" {
		installer.Libc = target.Libc
	}
	downloader := s.Downloader
	if downloader == nil && !options.Offline {
		downloader = HTTPDownloader{}
	}

	provisioner := Provisioner{
		Catalog:      catalog,
		Downloader:   downloader,
		Cache:        cache,
		Installer:    installer,
		GOOS:         target.OS,
		GOARCH:       target.Arch,
		Distribution: target.Distribution,
		Libc:         target.Libc,
	}
	provision := provisioner.Provision(ctx, requests, options)
	if !provision.Ready {
		return SetupReport{Manifest: manifest, Provision: provision}, provisioningError(provision)
	}

	updated := manifest
	changed := false
	for _, result := range provision.Results {
		relative, err := managedRelativePath(root, result.Path)
		if err != nil {
			return SetupReport{Manifest: manifest, Provision: provision}, fmt.Errorf("persist %s path: %w", result.Name, err)
		}
		if setManagedPath(&updated, result.Name, result.Version, relative) {
			changed = true
		}
	}
	if changed {
		if err := writeManifestAtomic(root, updated); err != nil {
			return SetupReport{Manifest: manifest, Provision: provision}, err
		}
	}
	return SetupReport{Manifest: updated, Provision: provision}, nil
}

func setupRequests(manifest Manifest) ([]ToolRequest, error) {
	if manifest.EffectiveProfile() == ProfileDocker {
		return nil, errors.New("docker profile does not install host toolchains; configure container tools instead")
	}
	requests := make([]ToolRequest, 0, 3)
	for _, item := range []struct {
		name ToolName
		spec ToolSpec
	}{
		{PHP, manifest.PHP},
		{Composer, manifest.Composer},
		{RoadRunner, manifest.RoadRunner},
	} {
		if manifest.EffectiveProfile() == ProfileCI && item.spec.Path == "" && item.spec.Version == "" {
			return nil, fmt.Errorf("%s is unpinned in ci profile; run tusk toolchain pin %s@<version>", item.name, item.name)
		}
		if item.spec.Path == "" && strings.TrimSpace(item.spec.Version) != "" {
			requests = append(requests, ToolRequest{Name: item.name, Version: item.spec.Version})
		}
	}
	return requests, nil
}

func managedRelativePath(root, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("installed path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve installed path: %w", err)
	}
	relative, err := filepath.Rel(root, abs)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("installed path %q escapes project root", path)
	}
	return filepath.ToSlash(relative), nil
}

func setManagedPath(manifest *Manifest, name ToolName, version, path string) bool {
	var spec *ToolSpec
	switch name {
	case PHP:
		spec = &manifest.PHP
	case Composer:
		spec = &manifest.Composer
	case RoadRunner:
		spec = &manifest.RoadRunner
	default:
		return false
	}
	if spec.Path != "" || spec.Version != version {
		return false
	}
	spec.Path = path
	return true
}

func provisioningError(report ProvisionReport) error {
	failures := make([]string, 0)
	for _, result := range report.Results {
		if result.Status == StatusProvisioned {
			continue
		}
		message := result.Error
		if message == "" {
			message = "unknown provisioning failure"
		}
		failures = append(failures, fmt.Sprintf("%s@%s: %s", result.Name, result.Version, message))
	}
	if len(failures) == 0 {
		return errors.New("toolchain provisioning failed")
	}
	return fmt.Errorf("toolchain provisioning failed: %s", strings.Join(failures, "; "))
}

func writeManifestAtomic(root string, manifest Manifest) error {
	directory := filepath.Join(root, ".tusk")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create toolchain directory: %w", err)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode toolchain manifest: %w", err)
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(directory, ".toolchain-*.json")
	if err != nil {
		return fmt.Errorf("create temporary toolchain manifest: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure temporary toolchain manifest: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary toolchain manifest: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary toolchain manifest: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary toolchain manifest: %w", err)
	}
	if err := os.Rename(temporaryPath, filepath.Join(directory, "toolchain.json")); err != nil {
		return fmt.Errorf("publish toolchain manifest: %w", err)
	}
	return nil
}
