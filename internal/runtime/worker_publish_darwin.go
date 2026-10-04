//go:build darwin

package runtime

import "golang.org/x/sys/unix"

func renameWorkerNoReplace(dirfd int, from, to string) error {
	return unix.RenameatxNp(dirfd, from, dirfd, to, unix.RENAME_EXCL)
}
