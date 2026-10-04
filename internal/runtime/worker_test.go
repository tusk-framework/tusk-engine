package runtime

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tusk-framework/tusk-engine/internal/config"
)

func TestWriteWorkerCreatesPrivateDeterministicApplicationBridge(t *testing.T) {
	root := t.TempDir()
	worker, err := WriteWorker(root)
	if err != nil {
		t.Fatal(err)
	}
	if worker.RelativePath != ".tusk/runtime/worker.php" {
		t.Fatalf("relative path = %q", worker.RelativePath)
	}
	wantPath := filepath.Join(root, ".tusk", "runtime", "worker.php")
	if worker.Path != wantPath {
		t.Fatalf("path = %q, want %q", worker.Path, wantPath)
	}
	data, err := os.ReadFile(worker.Path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, wanted := range []string{"dirname(__DIR__, 2)", "vendor/autoload.php", "bootstrap/app.php", "instanceof \\Tusk\\Foundation\\Application", "runWorker()"} {
		if !strings.Contains(content, wanted) {
			t.Fatalf("worker missing %q:\n%s", wanted, content)
		}
	}
	for _, forbidden := range []string{"NativeLoopAdapter", "json_decode", "fgets(STDIN)", "echo worker"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("worker contains %q", forbidden)
		}
	}
	info, err := os.Stat(worker.Path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
	if err := worker.Cleanup(); err != nil {
		t.Fatal(err)
	}
	again, err := WriteWorker(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(again.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(second) != content {
		t.Fatal("generated content changed between writes")
	}
	if err := again.Cleanup(); err != nil {
		t.Fatal(err)
	}
}

func TestWriteWorkerPreservesUserFilesAndRejectsExistingDestination(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"worker.php", "bootstrap/app.php", "config/app.php", "routes/web.php", ".tusk/runtime/user.php"} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(rel), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	worker, err := WriteWorker(root)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(root, ".tusk", "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("runtime directory contains unexpected temporary files: %v", entries)
	}
	if _, err := WriteWorker(root); err == nil {
		t.Fatal("second write overwrote the first worker")
	}
	if err := worker.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if err := worker.Cleanup(); err != nil {
		t.Fatalf("repeated cleanup: %v", err)
	}
	for _, rel := range []string{"worker.php", "bootstrap/app.php", "config/app.php", "routes/web.php", ".tusk/runtime/user.php"} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || string(data) != rel {
			t.Fatalf("user file %q changed: %q, %v", rel, data, err)
		}
	}
}

func TestWriteWorkerDoesNotOverwriteExistingRuntimeWorker(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, ".tusk", "runtime", "worker.php")
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("user worker"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteWorker(root); err == nil {
		t.Fatal("WriteWorker overwrote an existing worker")
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "user worker" {
		t.Fatalf("existing worker changed: %q, %v", data, err)
	}
}

func TestWorkerCleanupRefusesMutatedOrReplacedFile(t *testing.T) {
	root := t.TempDir()
	worker, err := WriteWorker(root)
	if err != nil {
		t.Fatal(err)
	}
	ownedPath := worker.Path
	userPath := filepath.Join(root, "user.php")
	if err := os.WriteFile(userPath, []byte("user"), 0o600); err != nil {
		t.Fatal(err)
	}
	worker.Path = userPath
	if err := worker.Cleanup(); err == nil {
		t.Fatal("cleanup accepted a mutated path")
	}
	worker.Path = ownedPath
	if err := os.Remove(ownedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ownedPath, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := worker.Cleanup(); err == nil {
		t.Fatal("cleanup removed a replacement file")
	}
	if data, err := os.ReadFile(ownedPath); err != nil || string(data) != "replacement" {
		t.Fatalf("replacement changed: %q, %v", data, err)
	}
}

func TestWriteWorkerRejectsUnsafeRootsAndSymlinkedRuntime(t *testing.T) {
	root := t.TempDir()
	for _, bad := range []string{"", " ", ".", "..", root + string(filepath.Separator) + ".." + string(filepath.Separator) + filepath.Base(root)} {
		if _, err := WriteWorker(bad); err == nil {
			t.Fatalf("accepted unsafe root %q", bad)
		}
	}
	outside := t.TempDir()
	link := filepath.Join(root, ".tusk")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := WriteWorker(root); err == nil {
		t.Fatal("accepted symlinked .tusk directory")
	}
	if _, err := os.Stat(filepath.Join(outside, "runtime", "worker.php")); !os.IsNotExist(err) {
		t.Fatalf("worker escaped through symlink: %v", err)
	}
}

func TestDefaultWorkerCommandPointsToGeneratedRuntimeWorker(t *testing.T) {
	if got := config.DefaultConfig().WorkerCommand; got != ".tusk/runtime/worker.php" {
		t.Fatalf("default worker command = %q", got)
	}
}
