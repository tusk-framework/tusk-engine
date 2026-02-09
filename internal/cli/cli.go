package cli

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/tusk-framework/tusk-engine/internal/config"
	"github.com/tusk-framework/tusk-engine/internal/php"
	"github.com/tusk-framework/tusk-engine/internal/server"
	"github.com/tusk-framework/tusk-engine/internal/worker"
)

// Run handles the command line arguments
func Run(args []string) {
	if len(args) < 2 {
		printHelp()
		return
	}

	// 1. Load Config
	cfg := config.LoadConfig()

	command := args[1]

	// 2. Check for scripts (npm-style)
	if script, ok := cfg.Scripts[command]; ok {
		runScript(script, args[2:])
		return
	}

	switch command {
	case "start":
		runServerWithConfig(cfg)
	case "help":
		printHelp()
	default:
		// Proxy everything else to the PHP CLI
		proxyToPHPWithConfig(cfg, args[1:])
	}
}

func printHelp() {
	fmt.Println("Tusk Native Engine (v0.1)")
	fmt.Println("\nUsage:")
	fmt.Println("  tusk start       Start the Application Server")
	fmt.Println("  tusk [command]   Run a framework command or script")
	fmt.Println("\nExamples:")
	fmt.Println("  tusk start")
	fmt.Println("  tusk migrate")
}

func runServerWithConfig(cfg *config.Config) {
	// 2. Initialize Worker Pool
	pool, err := worker.NewPool(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize worker pool: %v", err)
	}

	if err := pool.Start(); err != nil {
		log.Fatalf("Failed to start worker pool: %v", err)
	}
	defer pool.Stop()

	// 3. Start HTTP Server
	srv := server.NewServer(cfg, pool)
	if err := srv.Start(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func runScript(script string, extraArgs []string) {
	fullCommand := script
	if len(extraArgs) > 0 {
		fullCommand += " " + strings.Join(extraArgs, " ")
	}

	fmt.Printf("> %s\n", fullCommand)

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", fullCommand)
	} else {
		cmd = exec.Command("sh", "-c", fullCommand)
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			os.Exit(exitError.ExitCode())
		}
		log.Fatalf("Script failed: %v", err)
	}
}

func proxyToPHPWithConfig(cfg *config.Config, args []string) {
	// Initialize PHP Manager to find the binary
	mgr, err := php.NewManager(cfg.PhpBinary)
	if err != nil {
		log.Fatalf("Error resolving PHP: %v", err)
	}

	// Target script: user's "tusk" script or "console"
	script := "tusk"
	if _, err := os.Stat(script); os.IsNotExist(err) {
		if _, err := os.Stat("console"); err == nil {
			script = "console"
		} else {
			log.Fatalf("Could not find 'tusk' or 'console' script to execute.")
		}
	}

	// Construct command: php script [args]
	cmdArgs := append([]string{script}, args...)

	cmd := exec.Command(mgr.BinaryPath, cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			os.Exit(exitError.ExitCode())
		}
		log.Fatalf("Execution failed: %v", err)
	}
}
