package toolchain

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBuildEnvironmentForPlatformPrependsManagedToolsWithoutMutatingParent(t *testing.T) {
	root := t.TempDir()
	base := []string{"PATH=C:\\Windows\\System32", "HOME=C:\\Users\\tester"}
	report := Report{
		ProjectRoot: root,
		Platform:    Platform{OS: "windows", Arch: "amd64"},
		Tools: []Tool{
			{Name: string(PHP), Available: true, Path: filepath.Join(root, ".tusk", "toolchain", "php", "php.exe"), Source: SourceProject},
			{Name: string(Composer), Available: true, Path: filepath.Join(root, ".tusk", "toolchain", "composer", "composer.phar"), Source: SourceProject},
		},
	}
	original := append([]string(nil), base...)

	env := BuildEnvironmentForPlatform(root, report, base, "windows")
	pathValue := environmentValue(env, "PATH", true)
	wantPrefix := strings.Join([]string{
		filepath.Dir(report.Tools[0].Path),
		filepath.Dir(report.Tools[1].Path),
		filepath.Join(root, "vendor", "bin"),
	}, ";")
	if !strings.HasPrefix(pathValue, wantPrefix+";") {
		t.Fatalf("PATH = %q, want managed prefix %q", pathValue, wantPrefix)
	}
	if !reflect.DeepEqual(base, original) {
		t.Fatalf("base environment mutated: %#v", base)
	}
}

func TestBuildEnvironmentUsesUnixPathSeparator(t *testing.T) {
	root := t.TempDir()
	report := Report{ProjectRoot: root, Platform: Platform{OS: "linux", Arch: "amd64"}}
	env := BuildEnvironmentForPlatform(root, report, []string{"PATH=/usr/bin"}, "linux")
	if got := environmentValue(env, "PATH", false); !strings.Contains(got, filepath.Join(root, "vendor", "bin")+":/usr/bin") {
		t.Fatalf("PATH = %q, want Unix-separated project PATH", got)
	}
}

func TestResolveCommandRunsComposerPharThroughResolvedPHP(t *testing.T) {
	report := Report{Tools: []Tool{
		{Name: string(PHP), Available: true, Path: `C:\\toolchain\\php.exe`},
		{Name: string(Composer), Available: true, Path: `C:\\toolchain\\composer.phar`},
	}}

	command, err := ResolveCommand(report, Composer)
	if err != nil {
		t.Fatalf("ResolveCommand() error = %v", err)
	}
	if command.Executable != report.Tools[0].Path || !reflect.DeepEqual(command.Args, []string{report.Tools[1].Path}) {
		t.Fatalf("Composer command = %#v, want PHP + composer.phar", command)
	}
}

func TestResolveCommandReportsMissingIndependentTool(t *testing.T) {
	_, err := ResolveCommand(Report{Tools: []Tool{{Name: string(PHP), Available: true, Path: "php"}}}, Composer)
	if err == nil || !strings.Contains(err.Error(), "Composer") {
		t.Fatalf("ResolveCommand() error = %v, want actionable Composer error", err)
	}
}

func environmentValue(env []string, key string, windows bool) string {
	for _, item := range env {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 2 && ((windows && strings.EqualFold(parts[0], key)) || (!windows && parts[0] == key)) {
			return parts[1]
		}
	}
	return ""
}
