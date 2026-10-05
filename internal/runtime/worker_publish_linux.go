//go:build linux

package runtime

import "golang.org/x/sys/unix"

func renameWorkerNoReplace(dirfd int, from, to string) error {
	return unix.Renameat2(dirfd, from, dirfd, to, unix.RENAME_NOREPLACE)
}
