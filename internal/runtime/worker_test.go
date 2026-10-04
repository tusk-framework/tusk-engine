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

func TestWriteWorkerDoesNotClobberFileCreatedAtPublication(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, ".tusk", "runtime", "worker.php")
	called := false
	_, err := writeWorkerWithHooks(root, workerHooks{beforePublish: func() {
		called = true
		if writeErr := os.WriteFile(destination, []byte("concurrent owner"), 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
	}})
	if !called || err == nil {
		t.Fatalf("publication did not reject concurrent worker: called=%v, err=%v", called, err)
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "concurrent owner" {
		t.Fatalf("concurrent worker changed: %q, %v", data, err)
	}
	entries, err := os.ReadDir(filepath.Dir(destination))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary worker leaked: %v, %v", entries, err)
	}
}

func TestWriteWorkerDoesNotPublishOrRemoveReplacedTemporaryFile(t *testing.T) {
	root := t.TempDir()
	runtimePath := filepath.Join(root, ".tusk", "runtime")
	var replacementPath string
	_, err := writeWorkerWithHooks(root, workerHooks{beforePublish: func() {
		entries, readErr := os.ReadDir(runtimePath)
		if readErr != nil || len(entries) != 1 {
			t.Fatalf("temporary entries: %v, %v", entries, readErr)
		}
		replacementPath = filepath.Join(runtimePath, entries[0].Name())
		if renameErr := os.Rename(replacementPath, filepath.Join(root, "owned-temp.php")); renameErr != nil {
			t.Fatal(renameErr)
		}
		if writeErr := os.WriteFile(replacementPath, []byte("replacement temp"), 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
	}})
	if err == nil {
		t.Fatal("writer accepted a replaced temporary file")
	}
	data, readErr := os.ReadFile(replacementPath)
	if readErr != nil || string(data) != "replacement temp" {
		t.Fatalf("replacement temporary file changed: %q, %v", data, readErr)
	}
	if _, statErr := os.Stat(filepath.Join(runtimePath, "worker.php")); !os.IsNotExist(statErr) {
		t.Fatalf("worker published from replacement temporary file: %v", statErr)
	}
}

func TestWriteWorkerAtomicallyMovesTemporaryFileIntoPlace(t *testing.T) {
	root := t.TempDir()
	runtimePath := filepath.Join(root, ".tusk", "runtime")
	observed := false
	worker, err := writeWorkerWithHooks(root, workerHooks{afterPublish: func() {
		observed = true
		entries, readErr := os.ReadDir(runtimePath)
		if readErr != nil || len(entries) != 1 || entries[0].Name() != "worker.php" {
			t.Fatalf("publication left temporary entry: %v, %v", entries, readErr)
		}
		data, readErr := os.ReadFile(filepath.Join(runtimePath, "worker.php"))
		if readErr != nil || !strings.Contains(string(data), "runWorker()") {
			t.Fatalf("published worker incomplete: %q, %v", data, readErr)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Cleanup()
	if !observed {
		t.Fatal("publication hook was not reached")
	}
}

func TestWriteWorkerDoesNotFollowRuntimeDirectoryReplacement(t *testing.T) {
	root := t.TempDir()
	runtimePath := filepath.Join(root, ".tusk", "runtime")
	movedPath := filepath.Join(root, ".tusk", "runtime-moved")
	called := false
	moved := false
	worker, err := writeWorkerWithHooks(root, workerHooks{beforeTemp: func() {
		called = true
		if moveErr := os.Rename(runtimePath, movedPath); moveErr != nil {
			// A pinned Windows directory may refuse renaming while the writer owns it.
			return
		}
		moved = true
		if mkdirErr := os.Mkdir(runtimePath, 0o700); mkdirErr != nil {
			t.Fatal(mkdirErr)
		}
	}})
	if !called {
		t.Fatal("directory replacement hook was not reached")
	}
	if err == nil {
		defer worker.Cleanup()
		if moved {
			t.Fatal("writer accepted a moved runtime directory")
		}
	}
	if _, statErr := os.Stat(filepath.Join(runtimePath, "worker.php")); moved && !os.IsNotExist(statErr) {
		t.Fatalf("worker published into replacement directory: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(movedPath, "worker.php")); !os.IsNotExist(statErr) {
		t.Fatalf("worker published into moved directory: %v", statErr)
	}
}

func TestWorkerCleanupDoesNotDeleteReplacementAtRemoval(t *testing.T) {
	root := t.TempDir()
	worker, err := WriteWorker(root)
	if err != nil {
		t.Fatal(err)
	}
	ownedCopy := filepath.Join(root, "owned-copy.php")
	called := false
	err = worker.cleanupWithHooks(workerHooks{beforeCleanupMove: func() {
		called = true
		if moveErr := os.Rename(worker.Path, ownedCopy); moveErr != nil {
			t.Fatal(moveErr)
		}
		if writeErr := os.WriteFile(worker.Path, []byte("replacement"), 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
	}})
	if !called || err == nil {
		t.Fatalf("cleanup did not reject replacement: called=%v, err=%v", called, err)
	}
	data, readErr := os.ReadFile(worker.Path)
	if readErr != nil || string(data) != "replacement" {
		t.Fatalf("replacement was removed or changed: %q, %v", data, readErr)
	}
	if _, statErr := os.Stat(ownedCopy); statErr != nil {
		t.Fatalf("original worker disappeared: %v", statErr)
	}
}

func TestWorkerCleanupPreservesReplacementCreatedAfterMove(t *testing.T) {
	root := t.TempDir()
	worker, err := WriteWorker(root)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	err = worker.cleanupWithHooks(workerHooks{afterCleanupMove: func() {
		called = true
		if writeErr := os.WriteFile(worker.Path, []byte("new owner"), 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
	}})
	if !called || err != nil {
		t.Fatalf("cleanup after replacement: called=%v, err=%v", called, err)
	}
	data, readErr := os.ReadFile(worker.Path)
	if readErr != nil || string(data) != "new owner" {
		t.Fatalf("replacement changed: %q, %v", data, readErr)
	}
}

func TestWorkerCleanupDoesNotMoveExistingReplacement(t *testing.T) {
	root := t.TempDir()
	worker, err := WriteWorker(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(worker.Path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(worker.Path, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	moved := false
	err = worker.cleanupWithHooks(workerHooks{afterCleanupMove: func() { moved = true }})
	if err == nil || moved {
		t.Fatalf("cleanup moved an existing replacement: moved=%v, err=%v", moved, err)
	}
	data, readErr := os.ReadFile(worker.Path)
	if readErr != nil || string(data) != "replacement" {
		t.Fatalf("replacement changed: %q, %v", data, readErr)
	}
}

func TestWorkerCleanupRestoresDirectoryReplacementOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows directory restore behavior")
	}
	root := t.TempDir()
	worker, err := WriteWorker(root)
	if err != nil {
		t.Fatal(err)
	}
	ownedCopy := filepath.Join(root, "owned-copy.php")
	err = worker.cleanupWithHooks(workerHooks{beforeCleanupMove: func() {
		if moveErr := os.Rename(worker.Path, ownedCopy); moveErr != nil {
			t.Fatal(moveErr)
		}
		if mkdirErr := os.Mkdir(worker.Path, 0o700); mkdirErr != nil {
			t.Fatal(mkdirErr)
		}
	}})
	if err == nil {
		t.Fatal("cleanup accepted replacement directory")
	}
	info, statErr := os.Stat(worker.Path)
	if statErr != nil || !info.IsDir() {
		t.Fatalf("replacement directory was not restored: %v, %v", info, statErr)
	}
}
