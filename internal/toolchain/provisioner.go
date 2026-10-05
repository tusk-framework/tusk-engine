package toolchain

import (
	"context"
	"fmt"
	"runtime"
)

const defaultMaxArtifactBytes int64 = 512 * 1024 * 1024

const (
	StatusProvisioned = "provisioned"
	StatusFailed      = "failed"
)

// ToolRequest asks the provisioner for one exact tool version.
type ToolRequest struct {
	Name    ToolName
	Version string
}

// ProvisionOptions controls network and artifact limits for one run.
type ProvisionOptions struct {
	Offline          bool
	MaxArtifactBytes int64
}

// ToolProvisionResult is the independent outcome for one requested tool.
type ToolProvisionResult struct {
	Name    ToolName `json:"name"`
	Version string   `json:"version"`
	Path    string   `json:"path,omitempty"`
	Status  string   `json:"status"`
	Error   string   `json:"error,omitempty"`
}

// ProvisionReport aggregates independent tool outcomes.
type ProvisionReport struct {
	Results []ToolProvisionResult `json:"results"`
	Ready   bool                  `json:"ready"`
}

// Provisioner coordinates catalog selection, cache access, downloads, and
// installation. It has no global PATH side effects.
type Provisioner struct {
	Catalog      CatalogPayload
	Downloader   Downloader
	Cache        ArtifactCache
	Installer    Installer
	GOOS         string
	GOARCH       string
	Distribution string
	Libc         string
}

// Provision resolves and installs each request independently. A failure for
// one tool is recorded without rolling back another successful tool.
func (p Provisioner) Provision(ctx context.Context, requests []ToolRequest, options ProvisionOptions) ProvisionReport {
	goos := p.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := p.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	maxBytes := options.MaxArtifactBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxArtifactBytes
	}

	report := ProvisionReport{Results: make([]ToolProvisionResult, 0, len(requests)), Ready: true}
	for _, request := range requests {
		result := p.provisionOne(ctx, request, options.Offline, maxBytes, goos, goarch)
		report.Results = append(report.Results, result)
		if result.Status != StatusProvisioned {
			report.Ready = false
		}
	}
	return report
}

func (p Provisioner) provisionOne(ctx context.Context, request ToolRequest, offline bool, maxBytes int64, goos, goarch string) ToolProvisionResult {
	result := ToolProvisionResult{Name: request.Name, Version: request.Version, Status: StatusFailed}
	if request.Name != PHP && request.Name != Composer && request.Name != RoadRunner {
		result.Error = fmt.Sprintf("unsupported tool %q", request.Name)
		return result
	}
	if request.Version == "" {
		result.Error = "tool version is required"
		return result
	}
	artifact, ok := p.findArtifact(request, Platform{OS: goos, Arch: goarch, Distribution: p.Distribution, Libc: p.Libc})
	if !ok {
		result.Error = fmt.Sprintf("catalog has no artifact for %s@%s on %s", request.Name, request.Version, platformLabel(Platform{OS: goos, Arch: goarch, Distribution: p.Distribution, Libc: p.Libc}))
		return result
	}
	if err := validateArtifact(artifact); err != nil {
		result.Error = fmt.Sprintf("catalog artifact invalid: %v", err)
		return result
	}

	data, err := p.Cache.Get(artifact.SHA256)
	if err != nil {
		if offline {
			result.Error = fmt.Sprintf("offline cache unavailable: %v", err)
			return result
		}
		if p.Downloader == nil {
			result.Error = "artifact downloader is not configured"
			return result
		}
		data, err = p.Downloader.Download(ctx, artifact.URL, maxBytes)
		if err != nil {
			result.Error = fmt.Sprintf("download artifact: %v", err)
			return result
		}
		if _, err := p.Cache.Put(artifact.SHA256, data); err != nil {
			result.Error = fmt.Sprintf("cache artifact: %v", err)
			return result
		}
	}

	path, err := p.Installer.Install(artifact, data)
	if err != nil {
		result.Error = fmt.Sprintf("install artifact: %v", err)
		return result
	}
	result.Path = path
	result.Status = StatusProvisioned
	return result
}

func (p Provisioner) findArtifact(request ToolRequest, target Platform) (Artifact, bool) {
	for _, artifact := range p.Catalog.Artifacts {
		artifactTarget, err := artifactPlatform(artifact)
		if err != nil {
			continue
		}
		if artifact.Tool == request.Name && artifact.Version == request.Version && artifactTarget == target {
			return artifact, true
		}
	}
	return Artifact{}, false
}

func platformLabel(target Platform) string {
	label := target.OS + "/" + target.Arch
	if target.Distribution != "" {
		label += "/" + target.Distribution
	}
	if target.Libc != "" {
		label += "/" + target.Libc
	}
	return label
}
