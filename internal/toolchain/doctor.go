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
	"runtime"
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

// Profile controls which tool sources are acceptable for a project.
type Profile string

const (
	ProfileSystem       Profile = "system"
	ProfileProjectLocal Profile = "project-local"
	ProfileDocker       Profile = "docker"
	ProfileCI           Profile = "ci"
)

// Platform identifies the target for a toolchain manifest and diagnostic.
type Platform struct {
	OS           string `json:"os,omitempty"`
	Arch         string `json:"arch,omitempty"`
	Distribution string `json:"distribution,omitempty"`
	Libc         string `json:"libc,omitempty"`
}

// ProfilePolicy describes source and pin requirements for a profile.
type ProfilePolicy struct {
	AllowSystemFallback bool
	RequirePins         bool
	PreferProjectLocal  bool
}

// Policy returns the source policy for a supported profile.
func (p Profile) Policy() ProfilePolicy {
	switch p {
	case ProfileProjectLocal:
		return ProfilePolicy{AllowSystemFallback: true, PreferProjectLocal: true}
	case ProfileDocker:
		return ProfilePolicy{AllowSystemFallback: true}
	case ProfileCI:
		return ProfilePolicy{RequirePins: true, PreferProjectLocal: true}
	case ProfileSystem, "":
		return ProfilePolicy{AllowSystemFallback: true}
	default:
		return ProfilePolicy{}
	}
}

// ToolSpec describes a project-local tool and its expected version.
type ToolSpec struct {
	Version string `json:"version,omitempty"`
	Path    string `json:"path,omitempty"`
}

// Manifest is stored in .tusk/toolchain.json.
type Manifest struct {
	Profile    Profile  `json:"profile,omitempty"`
	Platform   Platform `json:"platform,omitempty"`
	PHP        ToolSpec `json:"php,omitempty"`
	Composer   ToolSpec `json:"composer,omitempty"`
	RoadRunner ToolSpec `json:"roadrunner,omitempty"`
}

// EffectiveProfile returns the backwards-compatible default for manifests
// created before profiles existed.
func (m Manifest) EffectiveProfile() Profile {
	if m.Profile == "" {
		return ProfileSystem
	}
	return m.Profile
}

// Target resolves an incomplete manifest target against the current runtime.
func (m Manifest) Target(defaultOS, defaultArch string) Platform {
	if defaultOS == "" {
		defaultOS = runtime.GOOS
	}
	if defaultArch == "" {
		defaultArch = runtime.GOARCH
	}
	target := m.Platform
	if target.OS == "" {
		target.OS = defaultOS
	}
	if target.Arch == "" {
		target.Arch = defaultArch
	}
	return target
}

// Validate checks the declarative part of the manifest without touching the
// filesystem or accepting a binary.
func (m Manifest) Validate() error {
	profile := m.EffectiveProfile()
	switch profile {
	case ProfileSystem, ProfileProjectLocal, ProfileDocker, ProfileCI:
	default:
		return fmt.Errorf("toolchain profile %q is unsupported; use system, project-local, docker, or ci", profile)
	}
	for label, value := range map[string]string{
		"platform OS":           m.Platform.OS,
		"platform arch":         m.Platform.Arch,
		"platform distribution": m.Platform.Distribution,
		"platform libc":         m.Platform.Libc,
	} {
		if value != "" && (strings.ContainsAny(value, `/\\:`) || strings.ContainsAny(value, "\x00\r\n")) {
			return fmt.Errorf("toolchain %s must be a safe path segment", label)
		}
	}
	return nil
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
	Verification     string `json:"verification,omitempty"`
	Error            string `json:"error,omitempty"`
}

// Report is the complete toolchain diagnosis.
type Report struct {
	ProjectRoot  string   `json:"project_root"`
	ManifestPath string   `json:"manifest_path"`
	Profile      string   `json:"profile"`
	Platform     Platform `json:"platform"`
	Tools        []Tool   `json:"tools"`
	Ready        bool     `json:"ready"`
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
	Profile   Profile
	Platform  Platform
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

// ParsePin parses the CLI form name@version.
func ParsePin(value string) (ToolName, string, error) {
	parts := strings.SplitN(strings.TrimSpace(value), "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(parts[1], "\r\n") {
		return "", "", fmt.Errorf("toolchain pin must use name@version")
	}

	name := ToolName(strings.ToLower(parts[0]))
	switch name {
	case PHP, Composer, RoadRunner:
	default:
		return "", "", fmt.Errorf("unknown tool %q", parts[0])
	}
	return name, parts[1], nil
}

// Pin updates one requested version in the project manifest. It preserves
// any existing executable path and never downloads or replaces a binary.
func Pin(root string, name ToolName, version string) (Manifest, error) {
	if strings.TrimSpace(version) == "" {
		return Manifest{}, errors.New("toolchain version cannot be empty")
	}
	if name != PHP && name != Composer && name != RoadRunner {
		return Manifest{}, fmt.Errorf("unknown tool %q", name)
	}

	root, err := filepath.Abs(root)
	if err != nil {
		return Manifest{}, fmt.Errorf("resolve project root: %w", err)
	}
	manifest, err := LoadManifest(root)
	if err != nil {
		return Manifest{}, err
	}
	spec := ToolSpec{Version: version}
	switch name {
	case PHP:
		spec.Path = manifest.PHP.Path
		manifest.PHP = spec
	case Composer:
		spec.Path = manifest.Composer.Path
		manifest.Composer = spec
	case RoadRunner:
		spec.Path = manifest.RoadRunner.Path
		manifest.RoadRunner = spec
	}

	manifestDir := filepath.Join(root, ".tusk")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		return Manifest{}, fmt.Errorf("create toolchain directory: %w", err)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Manifest{}, fmt.Errorf("encode toolchain manifest: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(manifestDir, "toolchain.json"), data, 0o644); err != nil {
		return Manifest{}, fmt.Errorf("write toolchain manifest: %w", err)
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
	if err := manifest.Validate(); err != nil {
		return Report{}, err
	}
	profile := manifest.EffectiveProfile()
	if options.Profile != "" {
		profile = options.Profile
	}
	policy := profile.Policy()
	target := manifest.Target(runtime.GOOS, runtime.GOARCH)
	if options.Platform.OS != "" {
		target.OS = options.Platform.OS
	}
	if options.Platform.Arch != "" {
		target.Arch = options.Platform.Arch
	}
	if err := (Manifest{Profile: profile, Platform: target}).Validate(); err != nil {
		return Report{}, err
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
		tools = append(tools, resolve(root, item.name, item.spec, options.Overrides[item.name], lookup, version, profile, policy, target))
	}

	ready := true
	for _, tool := range tools {
		if tool.Status != StatusOK {
			ready = false
			break
		}
	}

	return Report{ProjectRoot: root, ManifestPath: manifestPath, Profile: string(profile), Platform: target, Tools: tools, Ready: ready}, nil
}

// ResolveExecutable resolves one tool for a project and returns a startup-safe
// diagnostic instead of silently installing or replacing anything.
func ResolveExecutable(root string, name ToolName) (Tool, error) {
	return ResolveExecutableWithOptions(root, name, DiagnosticOptions{})
}

// ResolveExecutableWithOptions is the injectable form used by tests and
// callers that need deterministic discovery.
func ResolveExecutableWithOptions(root string, name ToolName, options DiagnosticOptions) (Tool, error) {
	if name != PHP && name != Composer && name != RoadRunner {
		return Tool{}, fmt.Errorf("unknown tool %q", name)
	}
	options.Root = root
	report, err := Diagnose(options)
	if err != nil {
		return Tool{}, err
	}
	tool := report.Tool(name)
	if tool.Status != StatusOK {
		label := string(name)
		if name == RoadRunner {
			label = "RoadRunner"
		}
		if tool.Error != "" {
			return tool, fmt.Errorf("%s: %s; run tusk doctor", label, tool.Error)
		}
		return tool, fmt.Errorf("%s executable is unavailable; run tusk doctor", label)
	}
	return tool, nil
}

func resolve(root string, name ToolName, spec ToolSpec, override string, lookup func(string) (string, error), version func(string) (string, error), profile Profile, policy ProfilePolicy, target Platform) Tool {
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
			candidate, err := filepath.Abs(filepath.Join(root, path))
			if err != nil {
				tool.Error = fmt.Sprintf("resolve %s project path: %v", name, err)
				return tool
			}
			relative, err := filepath.Rel(root, candidate)
			if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				tool.Error = fmt.Sprintf("%s project path %q escapes project root; use a path inside the project or an absolute managed path", name, pathHint)
				return tool
			}
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
		if policy.RequirePins && spec.Version == "" {
			tool.Error = fmt.Sprintf("%s is unpinned in %s profile; run tusk toolchain pin %s@<version>", name, profile, name)
			return tool
		}
		if !policy.AllowSystemFallback {
			tool.Error = fmt.Sprintf("%s has no project-local executable for %s profile on %s/%s; run tusk setup --toolchain", name, profile, target.OS, target.Arch)
			return tool
		}
		lookupName := executableName(name)
		if override != "" && spec.Path == "" {
			lookupName = override
		}
		path, err := lookup(lookupName)
		if err != nil {
			tool.Error = fmt.Sprintf("%s executable not found for %s profile on %s/%s; install it or run tusk setup --toolchain", name, profile, target.OS, target.Arch)
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

func executableName(name ToolName) string {
	if name == RoadRunner {
		return "rr"
	}
	return string(name)
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
