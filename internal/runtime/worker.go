package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const workerRelativePath = ".tusk/runtime/worker.php"

const workerContents = `<?php

require dirname(__DIR__, 2) . '/vendor/autoload.php';

$application = require dirname(__DIR__, 2) . '/bootstrap/app.php';

if (!$application instanceof \Tusk\Foundation\Application) {
    throw new \RuntimeException(
        'bootstrap/app.php must return a Tusk application instance.'
    );
}

$application->runWorker();
`

// WorkerFile owns one Engine-generated application worker.
type WorkerFile struct {
	Path         string
	RelativePath string
	ownedPath    string
	identity     os.FileInfo
}

// WriteWorker creates the private RoadRunner worker in the project runtime directory.
func WriteWorker(root string) (WorkerFile, error) {
	var zero WorkerFile
	if strings.TrimSpace(root) == "" || root == "." || root == ".." || filepath.Clean(root) != root {
		return zero, fmt.Errorf("unsafe project root %q", root)
	}
	for _, part := range strings.Split(filepath.ToSlash(root), "/") {
		if part == ".." {
			return zero, fmt.Errorf("project root contains traversal: %q", root)
		}
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return zero, fmt.Errorf("resolve project root: %w", err)
	}
	if absoluteRoot == filepath.VolumeName(absoluteRoot)+string(filepath.Separator) {
		return zero, fmt.Errorf("project root must not be a volume root: %q", root)
	}
	resolvedRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return zero, fmt.Errorf("resolve project root symlinks: %w", err)
	}
	if filepath.Clean(resolvedRoot) != filepath.Clean(absoluteRoot) {
		return zero, fmt.Errorf("ambiguous project root %q", root)
	}
	rootInfo, err := os.Stat(absoluteRoot)
	if err != nil || !rootInfo.IsDir() {
		return zero, fmt.Errorf("project root must be an existing directory: %q", root)
	}

	tuskDir := filepath.Join(absoluteRoot, ".tusk")
	runtimeDir := filepath.Join(tuskDir, "runtime")
	for _, directory := range []string{tuskDir, runtimeDir} {
		if err := os.Mkdir(directory, 0o700); err != nil && !os.IsExist(err) {
			return zero, fmt.Errorf("create runtime directory %q: %w", directory, err)
		}
		info, err := os.Lstat(directory)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return zero, fmt.Errorf("unsafe runtime directory %q", directory)
		}
	}
	destination := filepath.Join(runtimeDir, "worker.php")
	if _, err := os.Lstat(destination); err == nil {
		return zero, fmt.Errorf("refuse to overwrite existing worker %q", destination)
	} else if !os.IsNotExist(err) {
		return zero, fmt.Errorf("inspect worker destination: %w", err)
	}
	temporary, err := os.CreateTemp(runtimeDir, ".worker-*.php")
	if err != nil {
		return zero, fmt.Errorf("create temporary worker: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return zero, fmt.Errorf("restrict temporary worker: %w", err)
	}
	if _, err := temporary.WriteString(workerContents); err != nil {
		_ = temporary.Close()
		return zero, fmt.Errorf("write temporary worker: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return zero, fmt.Errorf("sync temporary worker: %w", err)
	}
	identity, err := temporary.Stat()
	if err != nil {
		_ = temporary.Close()
		return zero, fmt.Errorf("inspect temporary worker: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return zero, fmt.Errorf("close temporary worker: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return zero, fmt.Errorf("install worker: %w", err)
	}
	return WorkerFile{Path: destination, RelativePath: workerRelativePath, ownedPath: destination, identity: identity}, nil
}

// Cleanup removes only the file created by this WorkerFile value.
func (f *WorkerFile) Cleanup() error {
	if f == nil || f.identity == nil {
		return nil
	}
	if f.Path != f.ownedPath || f.RelativePath != workerRelativePath {
		return fmt.Errorf("refuse to remove non-owned worker %q", f.Path)
	}
	info, err := os.Lstat(f.Path)
	if os.IsNotExist(err) {
		f.identity = nil
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect generated worker: %w", err)
	}
	if !os.SameFile(f.identity, info) {
		return fmt.Errorf("refuse to remove replaced worker %q", f.Path)
	}
	if err := os.Remove(f.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove generated worker: %w", err)
	}
	f.identity = nil
	return nil
}
