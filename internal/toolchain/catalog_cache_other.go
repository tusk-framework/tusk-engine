//go:build !windows

package toolchain

import "os"

func replaceCatalogFile(source, destination string) error {
	return os.Rename(source, destination)
}
