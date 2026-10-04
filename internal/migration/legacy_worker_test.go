package migration

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func put(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDetectLegacyWorkerWithoutExecutingIt(t *testing.T) {
	root := t.TempDir()
	put(t, root, "worker.php", "<?php syntax error and dangerous side effects")
	result, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != Legacy || !strings.Contains(result.Message, "RoadRunner") || !strings.Contains(result.Action, "tusk migrate") {
		t.Fatalf("detection = %#v", result)
	}
}

func TestDetectModernProject(t *testing.T) {
	root := t.TempDir()
	put(t, root, "bootstrap/app.php", "<?php return null;")
	result, err := Detect(root)
	if err != nil || result.State != Modern {
		t.Fatalf("detection = %#v, %v", result, err)
	}
}

func TestDetectProjectWithNoWorker(t *testing.T) {
	result, err := Detect(t.TempDir())
	if err != nil || result.State != Missing || !strings.Contains(result.Action, "tusk init") {
		t.Fatalf("detection = %#v, %v", result, err)
	}
}

func TestMigrateLegacyWorkerReportsCreatedPathsAndPreservesWorker(t *testing.T) {
	root := t.TempDir()
	put(t, root, "worker.php", "legacy worker")
	created, err := Migrate(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"bootstrap/app.php", "bootstrap/providers.php", "config/app.php", "routes/web.php", "public/index.php"}
	if !reflect.DeepEqual(created, want) {
		t.Fatalf("created = %#v, want %#v", created, want)
	}
	for _, name := range want {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil || len(data) == 0 {
			t.Fatalf("%s: %v", name, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "worker.php"))
	if err != nil || string(data) != "legacy worker" {
		t.Fatalf("worker changed: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".tusk", "runtime", "worker.php")); !os.IsNotExist(err) {
		t.Fatalf("migration created runtime worker: %v", err)
	}
}

func TestMigrateReportsAllConflictsWithoutOverwritingOrPartialWrites(t *testing.T) {
	root := t.TempDir()
	put(t, root, "worker.php", "legacy")
	put(t, root, "bootstrap/app.php", "custom bootstrap")
	put(t, root, "config/app.php", "custom config")
	created, err := Migrate(root)
	if err == nil || !strings.Contains(err.Error(), "bootstrap/app.php") || !strings.Contains(err.Error(), "config/app.php") || !strings.Contains(err.Error(), "review") {
		t.Fatalf("created = %#v, error = %v", created, err)
	}
	if len(created) != 0 {
		t.Fatalf("created on conflict: %#v", created)
	}
	for name, want := range map[string]string{"bootstrap/app.php": "custom bootstrap", "config/app.php": "custom config"} {
		data, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if readErr != nil || string(data) != want {
			t.Fatalf("%s changed: %q, %v", name, data, readErr)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "routes", "web.php")); !os.IsNotExist(err) {
		t.Fatalf("partial migration: %v", err)
	}
}

func TestMigratePartiallyGeneratedProjectUsesEmptyDirectories(t *testing.T) {
	root := t.TempDir()
	put(t, root, "worker.php", "legacy")
	for _, name := range []string{"bootstrap", "config", "routes", "public"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	created, err := Migrate(root)
	if err != nil || len(created) != 5 {
		t.Fatalf("created = %#v, error = %v", created, err)
	}
}

func TestMigrateNoWorkerAndNoBootstrapIsDiagnosticOnly(t *testing.T) {
	root := t.TempDir()
	created, err := Migrate(root)
	if err == nil || !strings.Contains(err.Error(), "tusk init") || len(created) != 0 {
		t.Fatalf("created = %#v, error = %v", created, err)
	}
	if _, err := os.Stat(filepath.Join(root, "bootstrap")); !os.IsNotExist(err) {
		t.Fatalf("unexpected bootstrap: %v", err)
	}
}
