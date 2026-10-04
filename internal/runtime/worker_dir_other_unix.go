//go:build aix || darwin || dragonfly || freebsd || netbsd || openbsd || solaris

package runtime

import "fmt"

func openWorkerDirectory(_ string) (workerDirectory, error) {
	return nil, fmt.Errorf("generated worker publication is unavailable on this Unix platform")
}
