package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/tusk-framework/tusk-engine/internal/config"
	"github.com/tusk-framework/tusk-engine/internal/control"
	"github.com/tusk-framework/tusk-engine/internal/metrics"
	"github.com/tusk-framework/tusk-engine/internal/php"
	"github.com/tusk-framework/tusk-engine/internal/roadrunner"
	engineRuntime "github.com/tusk-framework/tusk-engine/internal/runtime"
	"github.com/tusk-framework/tusk-engine/internal/toolchain"
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
		// Both commands start the managed RoadRunner runtime.
		// "dev" is an alias for "start" to provide familiar npm/bun-style experience.
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
		if err := runServerWithConfig(cfg); err != nil {
			log.Fatalf("Runtime failed: %v", err)
		}
	case "setup":
		if hasArg(args[2:], "--toolchain") {
			runToolchainSetup(cfg)
		} else {
			runSetup(cfg)
		}
	case "doctor":
		runDoctor(cfg, args[2:])
	case "toolchain":
		runToolchainCommand(cfg, args[2:])
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
			log.Fatalf("Error: Script name required. Usage: tusk run <script>")
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

// RunWithExitCode preserves Run's compatibility while allowing the binary to
// report explicit setup failures to CI and shell scripts.
func RunWithExitCode(args []string) int {
	if len(args) >= 3 && args[1] == "setup" && hasArg(args[2:], "--toolchain") {
		cfg := config.LoadConfig()
		return runToolchainSetupCode(cfg, args[2:], os.Stdout, os.Stderr)
	}
	Run(args)
	return 0
}

func printHelp() {
	fmt.Println("Tusk Engine (v0.1)")
	fmt.Println("\nUsage:")
	fmt.Println("  tusk start [worker-file]  Start RoadRunner under Engine control")
	fmt.Println("  tusk dev [worker-file]    Start RoadRunner (alias for start)")
	fmt.Println("  tusk setup                Verify and setup environment")
	fmt.Println("  tusk setup --toolchain    Provision an explicit verified toolchain")
	fmt.Println("  tusk doctor [--json]      Diagnose PHP, Composer, and RoadRunner")
	fmt.Println("  tusk toolchain list       Show the resolved project toolchain")
	fmt.Println("  tusk toolchain pin X@V    Pin a tool version in .tusk/toolchain.json")
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
	runDoctor(cfg, nil)
}

func runToolchainSetup(cfg *config.Config) {
	if err := runToolchainSetupTo(cfg, nil, os.Stdout); err != nil {
		log.Printf("Toolchain setup failed: %v", err)
	}
}

func runToolchainCommand(cfg *config.Config, args []string) {
	if err := runToolchainCommandTo(cfg, args, os.Stdout, defaultDiagnose); err != nil {
		log.Printf("Toolchain command failed: %v", err)
	}
}

type diagnoseFunc func(*config.Config) (toolchain.Report, error)

func runToolchainCommandTo(cfg *config.Config, args []string, output io.Writer, diagnose diagnoseFunc) error {
	if len(args) == 0 || args[0] == "list" {
		return runDoctorToWith(cfg, args[1:], output, diagnose)
	}
	if args[0] != "pin" || len(args) != 2 {
		return fmt.Errorf("usage: tusk toolchain list [--json] | tusk toolchain pin <tool>@<version>")
	}

	name, version, err := toolchain.ParsePin(args[1])
	if err != nil {
		return err
	}
	if _, err := toolchain.Pin(cfg.ProjectRoot, name, version); err != nil {
		return fmt.Errorf("failed to pin toolchain: %w", err)
	}
	_, err = fmt.Fprintf(output, "Pinned %s@%s in .tusk/toolchain.json\n", name, version)
	return err
}

func hasArg(args []string, wanted string) bool {
	for _, arg := range args {
		if arg == wanted {
			return true
		}
	}
	return false
}

func runDoctor(cfg *config.Config, args []string) {
	if err := runDoctorTo(cfg, args, os.Stdout); err != nil {
		log.Printf("Toolchain diagnosis failed: %v", err)
	}
}

func runDoctorTo(cfg *config.Config, args []string, output io.Writer) error {
	return runDoctorToWith(cfg, args, output, defaultDiagnose)
}

func runDoctorToWith(cfg *config.Config, args []string, output io.Writer, diagnose diagnoseFunc) error {
	jsonOutput, err := parseDoctorArgs(args)
	if err != nil {
		return err
	}
	report, err := diagnose(cfg)
	if err != nil {
		return err
	}

	if jsonOutput {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("encode toolchain report: %w", err)
		}
		_, err = fmt.Fprintln(output, string(data))
		return err
	}

	_, _ = fmt.Fprintf(output, "Project Root: %s\n", report.ProjectRoot)
	_, _ = fmt.Fprintf(output, "Manifest: %s\n\n", report.ManifestPath)
	for _, tool := range report.Tools {
		if tool.Available {
			_, _ = fmt.Fprintf(output, "%-10s %-17s %s (%s)\n", tool.Name, tool.Status, tool.Path, tool.Source)
			continue
		}
		_, _ = fmt.Fprintf(output, "%-10s %-17s %s\n", tool.Name, tool.Status, tool.Error)
	}
	if report.Ready {
		_, err = fmt.Fprintln(output, "\nToolchain is ready.")
	} else {
		_, err = fmt.Fprintln(output, "\nToolchain is incomplete. Install the missing tools or add project-local paths to .tusk/toolchain.json.")
	}
	return err
}

func defaultDiagnose(cfg *config.Config) (toolchain.Report, error) {
	return toolchain.Diagnose(toolchain.DiagnosticOptions{
		Root: cfg.ProjectRoot,
		Overrides: map[toolchain.ToolName]string{
			toolchain.PHP: cfg.PhpBinary,
		},
	})
}

func parseDoctorArgs(args []string) (bool, error) {
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		default:
			return false, fmt.Errorf("unknown doctor flag %q", arg)
		}
	}
	return jsonOutput, nil
}

func parseToolchainSetupArgs(args []string) (toolchain.ProvisionOptions, error) {
	options := toolchain.ProvisionOptions{}
	for _, arg := range args {
		switch arg {
		case "--offline":
			options.Offline = true
		default:
			return options, fmt.Errorf("unknown toolchain setup flag %q", arg)
		}
	}
	return options, nil
}

func runToolchainSetupTo(cfg *config.Config, args []string, output io.Writer) error {
	options, err := parseToolchainSetupArgs(args)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(output, "--- Tusk Toolchain Setup ---")
	if options.Offline {
		_, _ = fmt.Fprintln(output, "Offline mode enabled; network access is disabled.")
	}
	catalogPath := filepath.Join(cfg.ProjectRoot, ".tusk", "toolchain.catalog.json")
	if _, err := os.Stat(catalogPath); err != nil {
		return fmt.Errorf("trusted catalog is not configured at %s; update the Engine catalog before provisioning", catalogPath)
	}
	return fmt.Errorf("trusted catalog loading is not available in this Engine build")
}

func runToolchainSetupCode(cfg *config.Config, args []string, output, errorsOutput io.Writer) int {
	if err := runToolchainSetupTo(cfg, args, output); err != nil {
		_, _ = fmt.Fprintln(errorsOutput, err)
		return 1
	}
	return 0
}

func runServerWithConfig(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("configuration is required")
	}
	if err := cfg.Control.Validate(); err != nil {
		return fmt.Errorf("invalid control configuration: %w", err)
	}
	if err := cfg.Runtime.Validate(); err != nil {
		return fmt.Errorf("invalid runtime configuration: %w", err)
	}

	resolved, err := toolchain.ResolveExecutable(cfg.ProjectRoot, toolchain.RoadRunner)
	if err != nil {
		return fmt.Errorf("resolve RoadRunner: %w", err)
	}
	projected, err := roadrunner.Project(cfg)
	if err != nil {
		return fmt.Errorf("project RoadRunner configuration: %w", err)
	}
	configFile, err := engineRuntime.NewConfigFile(cfg.ProjectRoot, projected)
	if err != nil {
		return err
	}
	defer func() { _ = configFile.Cleanup() }()

	manager := engineRuntime.NewManager(
		engineRuntime.ExecProcessFactory{Stdout: os.Stdout, Stderr: os.Stderr},
		engineRuntime.ProcessSpec{
			Binary:         resolved.Path,
			Args:           []string{"serve", "-c", configFile.Path},
			ReloadArgs:     []string{"reset", "-c", configFile.Path},
			Dir:            cfg.ProjectRoot,
			DesiredWorkers: cfg.WorkerCount,
		},
	)
	metricsHandler := metrics.NewHandler("http://" + cfg.Runtime.MetricsAddress + "/metrics")
	controlServer, err := control.NewServer(cfg.Control, manager, control.Metadata{
		EngineName:     "tusk-engine",
		Version:        "0.1.0",
		GoVersion:      runtime.Version(),
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
		WorkerCount:    cfg.WorkerCount,
		TimeoutSeconds: cfg.Timeout,
		Capabilities:   []string{"roadrunner", "persistent-workers", "metrics"},
	}, metricsHandler)
	if err != nil {
		return fmt.Errorf("failed to initialize control server: %w", err)
	}

	var controlErr <-chan error
	if cfg.Control.Enabled {
		controlError := make(chan error, 1)
		controlErr = controlError
		go func() { controlError <- controlServer.Start() }()
		readyContext, cancelReady := context.WithTimeout(context.Background(), 5*time.Second)
		err = controlServer.WaitReady(readyContext)
		cancelReady()
		if err != nil {
			shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
			_ = controlServer.Stop(shutdownContext)
			cancelShutdown()
			return fmt.Errorf("failed to start control server: %w", err)
		}
	}

	if err := manager.Start(context.Background()); err != nil {
		shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
		_ = controlServer.Stop(shutdownContext)
		cancelShutdown()
		return err
	}
	defer func() {
		shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
		_ = manager.Stop(shutdownContext)
		_ = controlServer.Stop(shutdownContext)
		cancelShutdown()
	}()
	probe := engineRuntime.NewHTTPReadinessProbe(cfg.Runtime.StatusAddress)
	startupContext, cancelStartup := context.WithTimeout(context.Background(), cfg.Runtime.StartupTimeout)
	err = manager.WaitReady(startupContext, probe, cfg.Runtime.ProbeInterval)
	cancelStartup()
	if err != nil {
		return fmt.Errorf("RoadRunner did not become ready: %w", err)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	for {
		select {
		case <-stop:
			log.Println("Shutting down gracefully...")
			log.Println("Server stopped.")
			return nil
		case err := <-controlErr:
			if err == nil {
				return fmt.Errorf("control server stopped unexpectedly")
			}
			return fmt.Errorf("control server failed: %w", err)
		default:
			if manager.State() == engineRuntime.StateFailed {
				return fmt.Errorf("RoadRunner failed")
			}
			time.Sleep(25 * time.Millisecond)
		}
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
