package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigHasSafeRequestLimits(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxBodyBytes != 10*1024*1024 {
		t.Fatalf("MaxBodyBytes = %d, want %d", cfg.MaxBodyBytes, 10*1024*1024)
	}
	if cfg.MaxUploadBytes != 10*1024*1024 {
		t.Fatalf("MaxUploadBytes = %d, want %d", cfg.MaxUploadBytes, 10*1024*1024)
	}
	if cfg.MaxUploadFiles != 20 {
		t.Fatalf("MaxUploadFiles = %d, want 20", cfg.MaxUploadFiles)
	}
}

func TestLoadConfigMergesScriptsWithTuskPrecedence(t *testing.T) {
	root := t.TempDir()

	writeConfigFile(t, filepath.Join(root, "composer.json"), `{
  "scripts": {
    "build": "composer-build",
    "shared": "composer-shared"
  }
}`)
	writeConfigFile(t, filepath.Join(root, "tusk.json"), `{
  "port": 9000,
  "scripts": {
    "dev": "tusk-dev",
    "shared": "tusk-shared"
  }
}`)

	cfg, err := loadConfigFromDir(root)
	if err != nil {
		t.Fatalf("loadConfigFromDir() error = %v", err)
	}

	if cfg.Port != 9000 {
		t.Fatalf("Port = %d, want 9000", cfg.Port)
	}
	if cfg.Scripts["build"] != "composer-build" {
		t.Fatalf("composer script was lost: %#v", cfg.Scripts)
	}
	if cfg.Scripts["dev"] != "tusk-dev" {
		t.Fatalf("tusk script missing: %#v", cfg.Scripts)
	}
	if cfg.Scripts["shared"] != "tusk-shared" {
		t.Fatalf("tusk script did not win conflict: %#v", cfg.Scripts)
	}
}

func TestLoadConfigResolvesProjectRootAndRejectsInvalidLimits(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, "tusk.json"), `{
  "project_root": ".",
  "max_body_bytes": 0
}`)

	if _, err := loadConfigFromDir(root); err == nil {
		t.Fatal("loadConfigFromDir() accepted zero max_body_bytes")
	}
}

func writeConfigFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}
