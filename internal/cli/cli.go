package cli

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
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
		// Check if a custom worker file is specified
		// args[0] = binary name, args[1] = "start", args[2] = optional worker file
		if len(args) >= 3 {
			workerFile := args[2]
			// Validate the worker file exists
			if _, err := os.Stat(workerFile); os.IsNotExist(err) {
				log.Fatalf("Worker file not found: %s", workerFile)
			}
			// Validate it has a .php extension
			if !strings.HasSuffix(strings.ToLower(workerFile), ".php") {
				log.Fatalf("Worker file must be a PHP file (*.php): %s", workerFile)
			}
			cfg.WorkerCommand = workerFile
		}
		runServerWithConfig(cfg)
	case "setup":
		runSetup(cfg)
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
	fmt.Println("  tusk start [worker-file]  Start the Application Server")
	fmt.Println("  tusk setup                Verify and setup environment")
	fmt.Println("  tusk [command]            Run a framework command or script")
	fmt.Println("\nExamples:")
	fmt.Println("  tusk start                # Uses worker.php (default)")
	fmt.Println("  tusk start custom.php     # Uses custom.php as worker")
	fmt.Println("  tusk migrate")
}

func runSetup(cfg *config.Config) {
	fmt.Println("--- Tusk Environment Setup ---")

	// 1. Check PHP
	mgr, err := php.NewManager(cfg.PhpBinary)
	if err != nil {
		fmt.Printf("PHP Error: %v\n", err)
		fmt.Println("Tip: Install PHP or set 'php_binary' in tusk.json")
	} else {
		fmt.Printf("PHP Found: %s\n", mgr.BinaryPath)
	}

	// 2. Check Paths
	cwd, _ := os.Getwd()
	fmt.Printf("Project Root: %s\n", cwd)

	fmt.Println("\nTusk is ready to go!")
}

func runServerWithConfig(cfg *config.Config) {
	// Resolve the worker path for logging
	workerPath := cfg.WorkerCommand
	if !filepath.IsAbs(workerPath) {
		workerPath = filepath.Join(cfg.ProjectRoot, workerPath)
	}
	// Get absolute path for clearer logging
	if absPath, err := filepath.Abs(workerPath); err == nil {
		workerPath = absPath
	}
	log.Printf("Starting server with worker: %s", workerPath)
	
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
