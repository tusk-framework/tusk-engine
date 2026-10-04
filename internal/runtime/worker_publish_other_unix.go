//go:build dragonfly || freebsd || netbsd || openbsd

package runtime

import "golang.org/x/sys/unix"

func renameWorkerNoReplace(dirfd int, from, to string) error {
	if err := unix.Linkat(dirfd, from, dirfd, to, 0); err != nil {
		return err
	}
	return unix.Unlinkat(dirfd, from, 0)
}
