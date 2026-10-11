package toolchain

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// WriteCatalogAtomic stores already-verified catalog bytes in the project's
// .tusk directory. Callers must authenticate bytes before invoking it.
func WriteCatalogAtomic(root string, data []byte) error {
	return writeCatalogAtomic(root, data, replaceCatalogFile)
}

func writeCatalogAtomic(root string, data []byte, replace func(string, string) error) error {
	if len(data) == 0 {
		return errors.New("catalog data is empty")
	}
	directory := filepath.Join(root, ".tusk")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create toolchain directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".toolchain-catalog-*.json")
	if err != nil {
		return fmt.Errorf("create temporary catalog: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure temporary catalog: %w", err)
	}
	written, err := temporary.Write(data)
	if err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary catalog: %w", err)
	}
	if written != len(data) {
		_ = temporary.Close()
		return fmt.Errorf("write temporary catalog: %w", io.ErrShortWrite)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary catalog: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary catalog: %w", err)
	}
	destination := filepath.Join(directory, "toolchain.catalog.json")
	if err := replace(temporaryPath, destination); err != nil {
		return fmt.Errorf("publish toolchain catalog: %w", err)
	}
	return nil
}
