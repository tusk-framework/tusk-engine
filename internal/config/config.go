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

	// Package management (from composer.json)
	Name         string                       `json:"name,omitempty"`
	Description  string                       `json:"description,omitempty"`
	Type         string                       `json:"type,omitempty"`
	Require      map[string]string            `json:"require,omitempty"`
	RequireDev   map[string]string            `json:"require-dev,omitempty"`
	Autoload     map[string]map[string]string `json:"autoload,omitempty"`
}

// ComposerConfig represents a composer.json file structure
type ComposerConfig struct {
	Name        string                       `json:"name"`
	Description string                       `json:"description"`
	Type        string                       `json:"type"`
	Require     map[string]string            `json:"require"`
	RequireDev  map[string]string            `json:"require-dev"`
	Autoload    map[string]map[string]string `json:"autoload"`
	Scripts     map[string]interface{}       `json:"scripts"`
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

	// Try to load composer.json first (as fallback)
	loadComposerConfig(cfg)

	// Then try to load tusk.json (takes priority over composer.json)
	file, err := os.Open("tusk.json")
	if err != nil {
		if os.IsNotExist(err) {
			return cfg // No tusk.json, use defaults + composer.json
		}
		return cfg
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(cfg); err != nil {
		fmt.Printf("Warning: Failed to parse tusk.json: %v. Using defaults.\n", err)
		return cfg
	}

	return cfg
}

// loadComposerConfig loads configuration from composer.json if it exists
func loadComposerConfig(cfg *Config) {
	file, err := os.Open("composer.json")
	if err != nil {
		return // No composer.json, skip
	}
	defer file.Close()

	var composer ComposerConfig
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&composer); err != nil {
		fmt.Printf("Warning: Failed to parse composer.json: %v\n", err)
		return
	}

	// Merge composer.json into config
	if composer.Name != "" {
		cfg.Name = composer.Name
	}
	if composer.Description != "" {
		cfg.Description = composer.Description
	}
	if composer.Type != "" {
		cfg.Type = composer.Type
	}

	// Merge dependencies
	if composer.Require != nil {
		cfg.Require = composer.Require
	}
	if composer.RequireDev != nil {
		cfg.RequireDev = composer.RequireDev
	}
	if composer.Autoload != nil {
		cfg.Autoload = composer.Autoload
	}

	// Merge scripts from composer.json
	if composer.Scripts != nil {
		for name, script := range composer.Scripts {
			// Convert script to string (can be string or array in composer.json)
			var scriptStr string
			switch v := script.(type) {
			case string:
				scriptStr = v
			case []interface{}:
				// If it's an array, join with &&
				var parts []string
				for _, part := range v {
					if str, ok := part.(string); ok {
						parts = append(parts, str)
					}
				}
				scriptStr = fmt.Sprintf("%v", parts)
			default:
				scriptStr = fmt.Sprintf("%v", script)
			}
			cfg.Scripts[name] = scriptStr
		}
	}
}
