//go:build aix || darwin || dragonfly || freebsd || netbsd || openbsd || solaris

package runtime

import (
	"fmt"
	"os"
)

func keepWorkerIdentity(*os.File) *os.File {
	return nil
}

func openWorkerDirectory(_ string) (workerDirectory, error) {
	return nil, fmt.Errorf("generated worker publication is unavailable on this Unix platform")
}
