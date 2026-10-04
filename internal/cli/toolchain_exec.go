package cli

import (
	"fmt"
	"os/exec"

	"github.com/tusk-framework/tusk-engine/internal/config"
	"github.com/tusk-framework/tusk-engine/internal/toolchain"
)

// buildToolchainCommand resolves one tool independently and gives it a
// process-local environment. The parent Engine environment is never mutated.
func buildToolchainCommand(cfg *config.Config, report toolchain.Report, name toolchain.ToolName, args []string) (*exec.Cmd, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	command, err := toolchain.ResolveCommand(report, name)
	if err != nil {
		return nil, err
	}
	commandArgs := append(append([]string(nil), command.Args...), args...)
	process := exec.Command(command.Executable, commandArgs...)
	process.Dir = cfg.ProjectRoot
	process.Env = toolchain.BuildEnvironment(cfg.ProjectRoot, report, nil)
	return process, nil
}
