package php

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Manager handles the PHP runtime
type Manager struct {
	BinaryPath string
}

// NewManager creates a new PHP manager
func NewManager(configuredPath string) (*Manager, error) {
	path, err := resolvePhpPath(configuredPath)
	if err != nil {
		return nil, err
	}
	return &Manager{BinaryPath: path}, nil
}

// resolvePhpPath attempts to find a usable PHP binary
func resolvePhpPath(configPath string) (string, error) {
	// 1. If explicitly configured, trust it (but verify existence)
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}
		// If configured but not found, maybe it's just a command name available in PATH
		if path, err := exec.LookPath(configPath); err == nil {
			return path, nil
		}
		return "", fmt.Errorf("configured PHP not found: %s", configPath)
	}

	// 2. Check for embedded/sidecar PHP in .tusk/bin
	// Get executable directory to find relative .tusk folder
	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		sidecarPath := filepath.Join(exeDir, ".tusk", "bin", "php")
		if runtime.GOOS == "windows" {
			sidecarPath += ".exe"
		}
		if _, err := os.Stat(sidecarPath); err == nil {
			return sidecarPath, nil
		}
	}

	// 3. Fallback to system PATH
	path, err := exec.LookPath("php")
	if err == nil {
		return path, nil
	}

	return "", fmt.Errorf("PHP executable not found. Please install PHP or place a portable version in .tusk/bin")
}
