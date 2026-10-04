package runtime

import (
	"crypto/rand"
	"encoding/hex"
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
	directory    workerDirectory
}

type workerDirectory interface {
	CreateTemp() (*os.File, string, error)
	Publish(string) error
	MoveToQuarantine() (string, error)
	Restore(string) error
	Remove(string) error
	Stat(string) (os.FileInfo, error)
	StillAtPath() bool
	Close() error
}

type workerHooks struct {
	beforeTemp        func()
	beforePublish     func()
	afterPublish      func()
	beforeCleanupMove func()
	afterCleanupMove  func()
}

// WriteWorker creates the private RoadRunner worker in the project runtime directory.
func WriteWorker(root string) (WorkerFile, error) {
	return writeWorkerWithHooks(root, workerHooks{})
}

func writeWorkerWithHooks(root string, hooks workerHooks) (WorkerFile, error) {
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

	directory, err := openWorkerDirectory(absoluteRoot)
	if err != nil {
		return zero, err
	}
	keepDirectory := false
	defer func() {
		if !keepDirectory {
			_ = directory.Close()
		}
	}()
	if hooks.beforeTemp != nil {
		hooks.beforeTemp()
	}
	if !directory.StillAtPath() {
		return zero, fmt.Errorf("runtime directory moved before worker creation")
	}
	temporary, temporaryName, err := directory.CreateTemp()
	if err != nil {
		return zero, fmt.Errorf("create temporary worker: %w", err)
	}
	identity, err := temporary.Stat()
	if err != nil {
		_ = temporary.Close()
		return zero, fmt.Errorf("inspect temporary worker: %w", err)
	}
	defer func() {
		if current, statErr := directory.Stat(temporaryName); statErr == nil && os.SameFile(identity, current) {
			_ = directory.Remove(temporaryName)
		}
	}()
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
	if err := temporary.Close(); err != nil {
		return zero, fmt.Errorf("close temporary worker: %w", err)
	}
	if hooks.beforePublish != nil {
		hooks.beforePublish()
	}
	if !directory.StillAtPath() {
		return zero, fmt.Errorf("runtime directory moved before worker publication")
	}
	current, err := directory.Stat(temporaryName)
	if err != nil || !os.SameFile(identity, current) {
		return zero, fmt.Errorf("temporary worker was replaced before publication")
	}
	if err := directory.Publish(temporaryName); err != nil {
		return zero, fmt.Errorf("publish worker without replacing an existing file: %w", err)
	}
	published, err := directory.Stat("worker.php")
	if err != nil || !os.SameFile(identity, published) {
		return zero, fmt.Errorf("published worker identity changed")
	}
	if hooks.afterPublish != nil {
		hooks.afterPublish()
	}
	destination := filepath.Join(absoluteRoot, filepath.FromSlash(workerRelativePath))
	worker := WorkerFile{Path: destination, RelativePath: workerRelativePath, ownedPath: destination, identity: identity, directory: directory}
	if !directory.StillAtPath() {
		_ = worker.Cleanup()
		return zero, fmt.Errorf("runtime directory moved during worker publication")
	}
	keepDirectory = true
	return worker, nil
}

// Cleanup removes only the file created by this WorkerFile value.
func (f *WorkerFile) Cleanup() error {
	return f.cleanupWithHooks(workerHooks{})
}

func (f *WorkerFile) cleanupWithHooks(hooks workerHooks) error {
	if f == nil || f.identity == nil {
		return nil
	}
	if f.Path != f.ownedPath || f.RelativePath != workerRelativePath {
		return fmt.Errorf("refuse to remove non-owned worker %q", f.Path)
	}
	current, err := f.directory.Stat("worker.php")
	if os.IsNotExist(err) {
		f.finishCleanup()
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect generated worker: %w", err)
	}
	if !os.SameFile(f.identity, current) {
		f.finishCleanup()
		return fmt.Errorf("refuse to remove replaced worker %q", f.Path)
	}
	if hooks.beforeCleanupMove != nil {
		hooks.beforeCleanupMove()
	}
	quarantine, err := f.directory.MoveToQuarantine()
	if os.IsNotExist(err) {
		f.finishCleanup()
		return nil
	}
	if err != nil {
		return fmt.Errorf("move worker for ownership check: %w", err)
	}
	if hooks.afterCleanupMove != nil {
		hooks.afterCleanupMove()
	}
	info, err := f.directory.Stat(quarantine)
	if err != nil || !os.SameFile(f.identity, info) {
		if restoreErr := f.directory.Restore(quarantine); restoreErr != nil {
			return fmt.Errorf("worker was replaced; preserve moved file %q: %w", quarantine, restoreErr)
		}
		f.finishCleanup()
		return fmt.Errorf("refuse to remove replaced worker %q", f.Path)
	}
	if err := f.directory.Remove(quarantine); err != nil {
		return fmt.Errorf("remove owned worker: %w", err)
	}
	f.finishCleanup()
	return nil
}

func (f *WorkerFile) finishCleanup() {
	_ = f.directory.Close()
	f.directory = nil
	f.identity = nil
}

func randomWorkerName(prefix string) (string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(nonce[:]), nil
}
