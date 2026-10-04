//go:build windows

package runtime

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

type windowsWorkerDirectory struct {
	root        *os.File
	tusk        *os.File
	runtime     *os.File
	rootPath    string
	tuskPath    string
	runtimePath string
}

func openWorkerDirectory(root string) (workerDirectory, error) {
	d := &windowsWorkerDirectory{rootPath: root}
	d.tuskPath = filepath.Join(root, ".tusk")
	d.runtimePath = filepath.Join(d.tuskPath, "runtime")
	var err error
	if d.root, err = openPinnedWorkerDirectory(root); err != nil {
		return nil, err
	}
	for _, child := range []struct {
		path string
		file **os.File
	}{{d.tuskPath, &d.tusk}, {d.runtimePath, &d.runtime}} {
		if err := os.Mkdir(child.path, 0o700); err != nil && !os.IsExist(err) {
			_ = d.Close()
			return nil, fmt.Errorf("create runtime directory %q: %w", child.path, err)
		}
		if *child.file, err = openPinnedWorkerDirectory(child.path); err != nil {
			_ = d.Close()
			return nil, err
		}
	}
	if !d.StillAtPath() {
		_ = d.Close()
		return nil, fmt.Errorf("runtime directory changed while opening")
	}
	return d, nil
}

func openPinnedWorkerDirectory(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, fmt.Errorf("open runtime directory %q: %w", path, err)
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("unsafe runtime directory %q", path)
	}
	return os.NewFile(uintptr(handle), path), nil
}

func (d *windowsWorkerDirectory) StillAtPath() bool {
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

func (d *windowsWorkerDirectory) CreateTemp() (*os.File, string, error) {
	file, err := os.CreateTemp(d.runtimePath, ".worker-*")
	if err != nil {
		return nil, "", err
	}
	return file, filepath.Base(file.Name()), nil
}

func (d *windowsWorkerDirectory) Publish(temp string) error {
	from, err := windows.UTF16PtrFromString(filepath.Join(d.runtimePath, temp))
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(filepath.Join(d.runtimePath, "worker.php"))
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, 0)
}

func (d *windowsWorkerDirectory) MoveToQuarantine() (string, error) {
	name, err := randomWorkerName(".worker-quarantine-")
	if err != nil {
		return "", err
	}
	from, err := windows.UTF16PtrFromString(filepath.Join(d.runtimePath, "worker.php"))
	if err != nil {
		return "", err
	}
	to, err := windows.UTF16PtrFromString(filepath.Join(d.runtimePath, name))
	if err != nil {
		return "", err
	}
	if err := windows.MoveFileEx(from, to, 0); err != nil {
		return "", err
	}
	return name, nil
}

func (d *windowsWorkerDirectory) Restore(name string) error {
	from, err := windows.UTF16PtrFromString(filepath.Join(d.runtimePath, name))
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(filepath.Join(d.runtimePath, "worker.php"))
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, 0)
}

func (d *windowsWorkerDirectory) Remove(name string) error {
	return os.Remove(filepath.Join(d.runtimePath, name))
}

func (d *windowsWorkerDirectory) Stat(name string) (os.FileInfo, error) {
	return os.Lstat(filepath.Join(d.runtimePath, name))
}

func (d *windowsWorkerDirectory) Close() error {
	for _, file := range []*os.File{d.runtime, d.tusk, d.root} {
		if file != nil {
			_ = file.Close()
		}
	}
	return nil
}
