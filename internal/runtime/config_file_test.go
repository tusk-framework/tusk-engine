package runtime

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNewConfigFileWritesPrivateUniqueRuntimeFile(t *testing.T) {
	root := t.TempDir()
	userConfig := filepath.Join(root, ".rr.yaml")
	if err := os.WriteFile(userConfig, []byte("user config\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	contents := []byte("version: \"3\"\n")

	first, err := NewConfigFile(root, contents)
	if err != nil {
		t.Fatalf("NewConfigFile() error = %v", err)
	}
	second, err := NewConfigFile(root, contents)
	if err != nil {
		t.Fatalf("NewConfigFile() second error = %v", err)
	}
	if first.Path == second.Path {
		t.Fatalf("generated paths must be unique: %q", first.Path)
	}
	for _, generated := range []*ConfigFile{first, second} {
		data, err := os.ReadFile(generated.Path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != string(contents) {
			t.Fatalf("generated contents = %q, want %q", data, contents)
		}
		info, err := os.Stat(generated.Path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("generated mode = %o, want 600", info.Mode().Perm())
		}
	}
	if err := first.Cleanup(); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if _, err := os.Stat(first.Path); !os.IsNotExist(err) {
		t.Fatalf("first generated file still exists: %v", err)
	}
	if _, err := os.Stat(userConfig); err != nil {
		t.Fatalf("user .rr.yaml was removed: %v", err)
	}
	if err := second.Cleanup(); err != nil {
		t.Fatalf("second Cleanup() error = %v", err)
	}
}

func TestConfigFileCleanupRefusesMutatedPath(t *testing.T) {
	root := t.TempDir()
	userConfig := filepath.Join(root, ".rr.yaml")
	if err := os.WriteFile(userConfig, []byte("user config\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	generated, err := NewConfigFile(root, []byte("version: \"3\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	generatedPath := generated.Path
	generated.Path = userConfig
	if err := generated.Cleanup(); err == nil {
		t.Fatal("Cleanup() accepted a path outside the generated runtime file")
	}
	if _, err := os.Stat(userConfig); err != nil {
		t.Fatalf("user .rr.yaml was removed: %v", err)
	}
	generated.Path = generatedPath
	if err := generated.Cleanup(); err != nil {
		t.Fatalf("Cleanup() generated file: %v", err)
	}
}
