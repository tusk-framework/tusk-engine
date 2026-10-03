// Package toolchain resolves and diagnoses the executables used by a Tusk
// project. It deliberately does not install or replace tools.
package toolchain

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ToolName identifies one of the tools managed by Tusk Engine.
type ToolName string

const (
	PHP        ToolName = "php"
	Composer   ToolName = "composer"
	RoadRunner ToolName = "roadrunner"
)

const (
	SourceProject = "project"
	SourceSystem  = "system"

	StatusOK              = "ok"
	StatusMissing         = "missing"
	StatusVersionMismatch = "version-mismatch"
)

// ToolSpec describes a project-local tool and its expected version.
type ToolSpec struct {
	Version string `json:"version,omitempty"`
	Path    string `json:"path,omitempty"`
}

// Manifest is stored in .tusk/toolchain.json.
type Manifest struct {
	PHP        ToolSpec `json:"php,omitempty"`
	Composer   ToolSpec `json:"composer,omitempty"`
	RoadRunner ToolSpec `json:"roadrunner,omitempty"`
}

// Tool is the diagnostic result for one managed executable.
type Tool struct {
	Name             string `json:"name"`
	Status           string `json:"status"`
	Available        bool   `json:"available"`
	Path             string `json:"path,omitempty"`
	Source           string `json:"source,omitempty"`
	Version          string `json:"version,omitempty"`
	RequestedVersion string `json:"requested_version,omitempty"`
	Error            string `json:"error,omitempty"`
}

// Report is the complete toolchain diagnosis.
type Report struct {
	ProjectRoot  string `json:"project_root"`
	ManifestPath string `json:"manifest_path"`
	Tools        []Tool `json:"tools"`
	Ready        bool   `json:"ready"`
}

// Tool returns one tool from the report. It returns an empty result when the
// name is not part of the report.
func (r Report) Tool(name ToolName) Tool {
	for _, tool := range r.Tools {
		if tool.Name == string(name) {
			return tool
		}
	}
	return Tool{Name: string(name), Status: StatusMissing}
}

// DiagnosticOptions customizes toolchain discovery. Lookup and Version are
// injectable so the resolver is deterministic and easy to test.
type DiagnosticOptions struct {
	Root      string
	Manifest  Manifest
	Overrides map[ToolName]string
	Lookup    func(name string) (string, error)
	Version   func(path string) (string, error)
}

// LoadManifest loads the optional project-local toolchain manifest. An absent
// manifest is valid and results in an empty manifest.
func LoadManifest(root string) (Manifest, error) {
	path := filepath.Join(root, ".tusk", "toolchain.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Manifest{}, nil
	}
	if err != nil {
		return Manifest{}, fmt.Errorf("read toolchain manifest: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse toolchain manifest: %w", err)
	}
	return manifest, nil
}

// Diagnose resolves the supported tools without changing the environment.
func Diagnose(options DiagnosticOptions) (Report, error) {
	root, err := filepath.Abs(options.Root)
	if err != nil {
		return Report{}, fmt.Errorf("resolve project root: %w", err)
	}

	manifest := options.Manifest
	manifestPath := filepath.Join(root, ".tusk", "toolchain.json")
	if manifest == (Manifest{}) {
		manifest, err = LoadManifest(root)
		if err != nil {
			return Report{}, err
		}
	}

	lookup := options.Lookup
	if lookup == nil {
		lookup = SystemLookup
	}
	version := options.Version
	if version == nil {
		version = CommandVersion
	}

	tools := make([]Tool, 0, 3)
	for _, item := range []struct {
		name ToolName
		spec ToolSpec
	}{
		{name: PHP, spec: manifest.PHP},
		{name: Composer, spec: manifest.Composer},
		{name: RoadRunner, spec: manifest.RoadRunner},
	} {
		tools = append(tools, resolve(root, item.name, item.spec, options.Overrides[item.name], lookup, version))
	}

	ready := true
	for _, tool := range tools {
		if tool.Status != StatusOK {
			ready = false
			break
		}
	}

	return Report{ProjectRoot: root, ManifestPath: manifestPath, Tools: tools, Ready: ready}, nil
}

func resolve(root string, name ToolName, spec ToolSpec, override string, lookup func(string) (string, error), version func(string) (string, error)) Tool {
	tool := Tool{
		Name:             string(name),
		Status:           StatusMissing,
		RequestedVersion: spec.Version,
	}

	pathHint := spec.Path
	if pathHint == "" {
		pathHint = override
	}
	if pathHint != "" {
		path := pathHint
		if !filepath.IsAbs(path) {
			candidate := filepath.Join(root, path)
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
			}
		}
		if _, err := os.Stat(path); err == nil {
			tool.Path = filepath.Clean(path)
			if spec.Path != "" || filepath.IsAbs(pathHint) {
				tool.Source = SourceProject
			}
		}
	}

	if tool.Path == "" {
		lookupName := string(name)
		if override != "" && spec.Path == "" {
			lookupName = override
		}
		path, err := lookup(lookupName)
		if err != nil {
			tool.Error = fmt.Sprintf("%s executable not found", name)
			return tool
		}
		tool.Path = path
		tool.Source = SourceSystem
	}

	detected, err := version(tool.Path)
	if err != nil {
		tool.Error = fmt.Sprintf("detect %s version: %v", name, err)
		return tool
	}
	tool.Version = strings.TrimSpace(detected)
	tool.Available = true
	tool.Status = StatusOK
	if spec.Version != "" && !versionMatches(spec.Version, tool.Version) {
		tool.Status = StatusVersionMismatch
		tool.Error = fmt.Sprintf("requested %q, detected %q", spec.Version, tool.Version)
	}
	return tool
}

func versionMatches(requested, detected string) bool {
	requested = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(requested), "v"))
	detected = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(detected), "v"))
	return requested != "" && strings.Contains(detected, requested)
}

// SystemLookup resolves a command using the current process PATH.
func SystemLookup(name string) (string, error) {
	return exec.LookPath(name)
}

// CommandVersion asks a tool for its version and returns its first output
// line. The result is intentionally human-readable because different tools
// use different version formats.
func CommandVersion(path string) (string, error) {
	output, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return "", errors.New("tool returned an empty version")
	}
	return strings.TrimSpace(lines[0]), nil
}
