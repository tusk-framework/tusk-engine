package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config holds the Tusk Engine configuration
type Config struct {
	// Server configuration
	Port    int    `json:"port"`
	Address string `json:"address"`

	// Worker configuration
	WorkerCount   int               `json:"worker_count"`
	WorkerCommand string            `json:"worker_command"`
	PhpBinary     string            `json:"php_binary"`
	PhpIni        string            `json:"php_ini"`
	ProjectRoot   string            `json:"project_root"`
	Scripts       map[string]string `json:"scripts"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Port:          8080,
		Address:       "0.0.0.0",
		WorkerCount:   4, // Default to a reasonable number
		WorkerCommand: "worker.php",
		PhpBinary:     "php",
		PhpIni:        "", // Empty means use system default
		ProjectRoot:   "./",
		Scripts:       make(map[string]string),
	}
}

// LoadConfig reads configuration from tusk.json if it exists, otherwise returns default
func LoadConfig() *Config {
	cfg := DefaultConfig()

	// Check for tusk.json in current directory
	file, err := os.Open("tusk.json")
	if err != nil {
		if os.IsNotExist(err) {
			return cfg // No config file, use defaults
		}
		// If exists but error opening, log it (but we don't have logger setup in config package usually)
		// For now just return defaults or maybe panic? Better to just return default with valid error
		// Refactor later to return (*Config, error)
		return cfg
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(cfg); err != nil {
		// Found file but failed to parse.
		// Ideally we should warn the user.
		fmt.Printf("Warning: Failed to parse tusk.json: %v. Using defaults.\n", err)
		return cfg
	}

	return cfg
}
