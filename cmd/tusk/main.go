package main

import (
	"os"

	"github.com/tusk-framework/tusk-engine/internal/cli"
)

func main() {
	os.Exit(cli.RunWithExitCode(os.Args))
}
