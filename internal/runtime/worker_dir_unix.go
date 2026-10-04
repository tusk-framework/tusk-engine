//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package runtime

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

type unixWorkerDirectory struct {
	root        *os.File
	tusk        *os.File
	runtime     *os.File
	rootPath    string
	tuskPath    string
	runtimePath string
}

func openWorkerDirectory(root string) (workerDirectory, error) {
	rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open project root: %w", err)
	}
	d := &unixWorkerDirectory{root: os.NewFile(uintptr(rootFD), root), rootPath: root}
	d.tuskPath = filepath.Join(root, ".tusk")
	d.runtimePath = filepath.Join(d.tuskPath, "runtime")
	if d.tusk, err = openWorkerChild(d.root, ".tusk", d.tuskPath); err != nil {
		_ = d.Close()
		return nil, err
	}
	if d.runtime, err = openWorkerChild(d.tusk, "runtime", d.runtimePath); err != nil {
		_ = d.Close()
		return nil, err
	}
	if !d.StillAtPath() {
		_ = d.Close()
		return nil, fmt.Errorf("runtime directory changed while opening")
	}
	return d, nil
}

func openWorkerChild(parent *os.File, name, path string) (*os.File, error) {
	if err := unix.Mkdirat(int(parent.Fd()), name, 0o700); err != nil && err != unix.EEXIST {
		return nil, fmt.Errorf("create runtime directory %q: %w", path, err)
	}
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open runtime directory %q: %w", path, err)
	}
	return os.NewFile(uintptr(fd), path), nil
}

func (d *unixWorkerDirectory) StillAtPath() bool {
	for _, pair := range []struct {
		path string
		file *os.File
	}{{d.rootPath, d.root}, {d.tuskPath, d.tusk}, {d.runtimePath, d.runtime}} {
		if pair.file == nil {
			return false
		}
		pathInfo, pathErr := os.Lstat(pair.path)
		handleInfo, handleErr := pair.file.Stat()
		if pathErr != nil || handleErr != nil || !pathInfo.IsDir() || !os.SameFile(pathInfo, handleInfo) {
			return false
		}
	}
	return true
}

func (d *unixWorkerDirectory) CreateTemp() (*os.File, string, error) {
	for range 4 {
		name, err := randomWorkerName(".worker-")
		if err != nil {
			return nil, "", err
		}
		fd, err := unix.Openat(int(d.runtime.Fd()), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
		if err == unix.EEXIST {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		return os.NewFile(uintptr(fd), name), name, nil
	}
	return nil, "", fmt.Errorf("could not allocate unique temporary worker")
}

func (d *unixWorkerDirectory) Publish(temp string) error {
	return renameWorkerNoReplace(int(d.runtime.Fd()), temp, "worker.php")
}

func (d *unixWorkerDirectory) MoveToQuarantine() (string, error) {
	name, err := randomWorkerName(".worker-quarantine-")
	if err != nil {
		return "", err
	}
	if err := renameWorkerNoReplace(int(d.runtime.Fd()), "worker.php", name); err != nil {
		return "", err
	}
	return name, nil
}

func (d *unixWorkerDirectory) Restore(name string) error {
	return renameWorkerNoReplace(int(d.runtime.Fd()), name, "worker.php")
}

func (d *unixWorkerDirectory) Remove(name string) error {
	return unix.Unlinkat(int(d.runtime.Fd()), name, 0)
}

func (d *unixWorkerDirectory) Stat(name string) (os.FileInfo, error) {
	fd, err := unix.Openat(int(d.runtime.Fd()), name, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	return file.Stat()
}

func (d *unixWorkerDirectory) Close() error {
	for _, file := range []*os.File{d.runtime, d.tusk, d.root} {
		if file != nil {
			_ = file.Close()
		}
	}
	return nil
}
