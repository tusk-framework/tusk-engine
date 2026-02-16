package cli

import (
	"encoding/json"
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

	// 2. Check for built-in commands first (they take priority over scripts)
	switch command {
	case "start", "dev":
		// Both commands start tusk's high-performance server with worker pool
		// "dev" is an alias for "start" to provide familiar npm/bun-style experience
		// Use tusk's server instead of php -S for stateful workers and better performance
		// Check if a custom worker file is specified
		// args[0] = binary name, args[1] = "start"/"dev", args[2] = optional worker file
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
	case "install":
		runInstall(args[2:])
	case "add":
		if len(args) < 3 {
			log.Fatalf("Usage: tusk add <package>")
		}
		runAdd(args[2:])
	case "remove":
		if len(args) < 3 {
			log.Fatalf("Usage: tusk remove <package>")
		}
		runRemove(args[2:])
	case "update":
		runUpdate(args[2:])
	case "init":
		runInit()
	case "run":
		// Explicit command to run scripts from tusk.json or composer.json
		// Usage: tusk run <script>
		if len(args) < 3 {
			log.Fatalf("Usage: tusk run <script>")
		}
		scriptName := args[2]
		if script, ok := cfg.Scripts[scriptName]; ok {
			runScript(script, args[3:])
		} else {
			log.Fatalf("Script '%s' not found in tusk.json or composer.json", scriptName)
		}
	case "help":
		printHelp()
	default:
		// 3. Check for scripts (npm-style) if not a built-in command
		if script, ok := cfg.Scripts[command]; ok {
			runScript(script, args[2:])
			return
		}
		// 4. Proxy everything else to the PHP CLI
		proxyToPHPWithConfig(cfg, args[1:])
	}
}

func printHelp() {
	fmt.Println("Tusk Native Engine (v0.1)")
	fmt.Println("\nUsage:")
	fmt.Println("  tusk start [worker-file]  Start the Application Server")
	fmt.Println("  tusk dev [worker-file]    Start in development mode (alias for start)")
	fmt.Println("  tusk setup                Verify and setup environment")
	fmt.Println("  tusk init                 Initialize a new tusk.json file")
	fmt.Println("\nPackage Management:")
	fmt.Println("  tusk install              Install PHP dependencies")
	fmt.Println("  tusk add <package>        Add a PHP package")
	fmt.Println("  tusk remove <package>     Remove a PHP package")
	fmt.Println("  tusk update [package]     Update dependencies")
	fmt.Println("\nScript Runner:")
	fmt.Println("  tusk run <script>         Run a script from tusk.json or composer.json")
	fmt.Println("  tusk <script>             Run a script directly (shorthand)")
	fmt.Println("\nOther Commands:")
	fmt.Println("  tusk [command]            Run a framework command")
	fmt.Println("\nExamples:")
	fmt.Println("  tusk start                # Start the high-performance tusk server")
	fmt.Println("  tusk dev                  # Same as start - use tusk server, not php -S")
	fmt.Println("  tusk start custom.php     # Uses custom.php as worker")
	fmt.Println("  tusk install              # Install dependencies from composer.json")
	fmt.Println("  tusk add symfony/console  # Add a package")
	fmt.Println("  tusk run test             # Run test script (explicit)")
	fmt.Println("  tusk test                 # Run test script (shorthand)")
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

// runInit creates a new tusk.json file
func runInit() {
	if _, err := os.Stat("tusk.json"); err == nil {
		fmt.Println("tusk.json already exists")
		return
	}

	// Check if composer.json exists
	hasComposer := false
	if _, err := os.Stat("composer.json"); err == nil {
		hasComposer = true
		fmt.Println("Found composer.json - will merge configuration")
	}

	cfg := config.DefaultConfig()
	if hasComposer {
		// Load from composer.json
		cfg = config.LoadConfig()
	}

	// Write tusk.json
	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		log.Fatalf("Failed to create tusk.json: %v", err)
	}

	if err := os.WriteFile("tusk.json", data, 0644); err != nil {
		log.Fatalf("Failed to write tusk.json: %v", err)
	}

	fmt.Println("Created tusk.json successfully!")
}

// runInstall installs PHP dependencies using composer
func runInstall(args []string) {
	fmt.Println("Installing PHP dependencies...")

	// Check if composer is installed
	if _, err := exec.LookPath("composer"); err != nil {
		log.Fatalf("Composer not found. Please install composer: https://getcomposer.org/")
	}

	// Run composer install
	cmdArgs := append([]string{"install"}, args...)
	cmd := exec.Command("composer", cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to install dependencies: %v", err)
	}

	fmt.Println("Dependencies installed successfully!")
}

// runAdd adds a PHP package
func runAdd(packages []string) {
	fmt.Printf("Adding package(s): %s\n", strings.Join(packages, ", "))

// Check if composer is installed
if _, err := exec.LookPath("composer"); err != nil {
log.Fatalf("Composer not found. Please install composer: https://getcomposer.org/")
}

// Run composer require
cmdArgs := append([]string{"require"}, packages...)
cmd := exec.Command("composer", cmdArgs...)
cmd.Stdin = os.Stdin
cmd.Stdout = os.Stdout
cmd.Stderr = os.Stderr

if err := cmd.Run(); err != nil {
log.Fatalf("Failed to add package: %v", err)
}

	fmt.Println("Package(s) added successfully!")
}

// runRemove removes a PHP package
func runRemove(packages []string) {
	fmt.Printf("Removing package(s): %s\n", strings.Join(packages, ", "))

	// Check if composer is installed
	if _, err := exec.LookPath("composer"); err != nil {
		log.Fatalf("Composer not found. Please install composer: https://getcomposer.org/")
	}

	// Run composer remove
	cmdArgs := append([]string{"remove"}, packages...)
	cmd := exec.Command("composer", cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to remove package: %v", err)
	}

	fmt.Println("Package(s) removed successfully!")
}

// runUpdate updates PHP dependencies
func runUpdate(packages []string) {
	if len(packages) == 0 {
		fmt.Println("Updating all PHP dependencies...")
	} else {
		fmt.Printf("Updating package(s): %s\n", strings.Join(packages, ", "))
	}

	// Check if composer is installed
	if _, err := exec.LookPath("composer"); err != nil {
		log.Fatalf("Composer not found. Please install composer: https://getcomposer.org/")
	}

	// Run composer update
	cmdArgs := append([]string{"update"}, packages...)
	cmd := exec.Command("composer", cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to update dependencies: %v", err)
	}

	fmt.Println("Dependencies updated successfully!")
}
