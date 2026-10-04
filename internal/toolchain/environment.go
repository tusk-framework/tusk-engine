package toolchain

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ResolvedCommand is the executable plus arguments needed to invoke one
// tool. Composer PHARs use PHP as the executable and the PHAR as an argument.
type ResolvedCommand struct {
	Tool       ToolName
	Executable string
	Args       []string
}

// BuildEnvironmentForPlatform returns a copy of base with project-local
// tool directories and vendor/bin prepended to PATH. It never mutates the
// parent process environment or the supplied slice.
func BuildEnvironmentForPlatform(root string, report Report, base []string, goos string) []string {
	if base == nil {
		base = os.Environ()
	}
	if goos == "" {
		goos = report.Platform.OS
	}
	if goos == "" {
		goos = runtime.GOOS
	}
	windows := strings.EqualFold(goos, "windows")
	separator := ":"
	if windows {
		separator = ";"
	}

	entries := make([]string, 0, len(base)+1)
	managed := make([]string, 0, 4)
	seen := make(map[string]struct{})
	for _, tool := range []ToolName{PHP, Composer, RoadRunner} {
		resolved := report.Tool(tool)
		if !resolved.Available || resolved.Path == "" || resolved.Source != SourceProject {
			continue
		}
		directory := filepath.Dir(resolved.Path)
		key := directory
		if windows {
			key = strings.ToLower(directory)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		managed = append(managed, directory)
	}
	if root != "" {
		vendorBin := filepath.Join(root, "vendor", "bin")
		key := vendorBin
		if windows {
			key = strings.ToLower(vendorBin)
		}
		if _, exists := seen[key]; !exists {
			managed = append(managed, vendorBin)
		}
	}

	pathKey := "PATH"
	var existingPath string
	for _, item := range base {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 2 && ((windows && strings.EqualFold(parts[0], pathKey)) || (!windows && parts[0] == pathKey)) {
			existingPath = parts[1]
			continue
		}
		entries = append(entries, item)
	}
	pathValue := strings.Join(managed, separator)
	if existingPath != "" {
		if pathValue != "" {
			pathValue += separator
		}
		pathValue += existingPath
	}
	entries = append(entries, pathKey+"="+pathValue)
	return entries
}

// BuildEnvironment derives an environment using the report's target.
func BuildEnvironment(root string, report Report, base []string) []string {
	return BuildEnvironmentForPlatform(root, report, base, report.Platform.OS)
}

// ResolveCommand turns one diagnosed tool into a safe process invocation.
func ResolveCommand(report Report, name ToolName) (ResolvedCommand, error) {
	tool := report.Tool(name)
	if !tool.Available || strings.TrimSpace(tool.Path) == "" {
		label := string(name)
		if name == RoadRunner {
			label = "RoadRunner"
		} else if name == Composer {
			label = "Composer"
		}
		if tool.Error != "" {
			return ResolvedCommand{}, fmt.Errorf("%s is unavailable: %s", label, tool.Error)
		}
		return ResolvedCommand{}, fmt.Errorf("%s executable is unavailable; run tusk doctor", label)
	}

	command := ResolvedCommand{Tool: name, Executable: tool.Path}
	if name != Composer || !strings.EqualFold(filepath.Ext(tool.Path), ".phar") {
		return command, nil
	}
	php := report.Tool(PHP)
	if !php.Available || strings.TrimSpace(php.Path) == "" {
		return ResolvedCommand{}, fmt.Errorf("Composer PHAR requires a resolved PHP executable; run tusk doctor for PHP")
	}
	command.Executable = php.Path
	command.Args = []string{tool.Path}
	return command, nil
}
