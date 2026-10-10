package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/tusk-framework/tusk-engine/internal/config"
	"github.com/tusk-framework/tusk-engine/internal/php"
)

func resolveFrameworkCLI(projectRoot, goos string) (string, error) {
	if projectRoot == "" {
		return "", fmt.Errorf("project root is required to resolve the Framework CLI")
	}

	displayEntrypoint := filepath.ToSlash(filepath.Join("vendor", "bin", "tusk"))
	entrypoint := filepath.Join(projectRoot, "vendor", "bin", "tusk")
	absoluteEntrypoint, err := filepath.Abs(entrypoint)
	if err != nil {
		return "", fmt.Errorf("resolve Framework CLI path: %w", err)
	}
	info, err := os.Stat(absoluteEntrypoint)
	if err == nil {
		if info.IsDir() {
			return "", fmt.Errorf("Framework CLI at %s is a directory; run `tusk install` to restore Composer dependencies", displayEntrypoint)
		}
		return absoluteEntrypoint, nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect Framework CLI at %s: %w", absoluteEntrypoint, err)
	}

	if goos == "windows" {
		batchWrapper := absoluteEntrypoint + ".bat"
		if _, wrapperErr := os.Stat(batchWrapper); wrapperErr == nil {
			return "", fmt.Errorf("Composer Windows wrapper exists at %s but its Composer PHP proxy is missing; run `tusk install` to regenerate %s", filepath.ToSlash(filepath.Join("vendor", "bin", "tusk.bat")), displayEntrypoint)
		}
	}

	return "", fmt.Errorf("Framework CLI not found at %s; install the project dependencies with `tusk install`", displayEntrypoint)
}

func runFrameworkCommand(cfg *config.Config, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if cfg == nil {
		return fmt.Errorf("project configuration is required to run Framework commands")
	}
	entrypoint, err := resolveFrameworkCLI(cfg.ProjectRoot, runtime.GOOS)
	if err != nil {
		return err
	}
	manager, err := php.NewManager(cfg.PhpBinary)
	if err != nil {
		return fmt.Errorf("resolve PHP runtime for Framework CLI %s: %w", entrypoint, err)
	}

	commandArgs := make([]string, 1, len(args)+1)
	commandArgs[0] = entrypoint
	commandArgs = append(commandArgs, args...)
	command := exec.Command(manager.BinaryPath, commandArgs...)
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}
