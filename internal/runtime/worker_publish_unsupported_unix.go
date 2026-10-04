//go:build aix || solaris

package runtime

import "fmt"

func renameWorkerNoReplace(_ int, _, _ string) error {
	return fmt.Errorf("atomic no-replace worker rename is unavailable on this platform")
}
