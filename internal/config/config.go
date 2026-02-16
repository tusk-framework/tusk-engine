package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
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
	Version      string                       `json:"version,omitempty"`
	Keywords     []string                     `json:"keywords,omitempty"`
	Homepage         string                       `json:"homepage,omitempty"`
	License          interface{}                  `json:"license,omitempty"` // Can be string or array
	Authors          []Author                     `json:"authors,omitempty"`
	Require          map[string]string            `json:"require,omitempty"`
	RequireDev       map[string]string            `json:"require-dev,omitempty"`
	Conflict         map[string]string            `json:"conflict,omitempty"`
	Replace          map[string]string            `json:"replace,omitempty"`
	Provide          map[string]string            `json:"provide,omitempty"`
	Suggest          map[string]string            `json:"suggest,omitempty"`
	Autoload         map[string]map[string]string `json:"autoload,omitempty"`
	AutoloadDev      map[string]map[string]string `json:"autoload-dev,omitempty"`
	MinimumStability string                       `json:"minimum-stability,omitempty"`
	PreferStable     bool                         `json:"prefer-stable,omitempty"`
	Bin              interface{}                  `json:"bin,omitempty"` // Can be string or array
	Extra            map[string]interface{}       `json:"extra,omitempty"`
	Config           map[string]interface{}       `json:"config,omitempty"`
	Repositories     []interface{}                `json:"repositories,omitempty"`
}

// Author represents a package author
type Author struct {
	Name     string `json:"name"`
	Email    string `json:"email,omitempty"`
	Homepage string `json:"homepage,omitempty"`
	Role     string `json:"role,omitempty"`
}

// ComposerConfig represents a composer.json file structure
type ComposerConfig struct {
	Name            string                       `json:"name"`
	Description     string                       `json:"description"`
	Type            string                       `json:"type"`
	Version         string                       `json:"version"`
	Keywords        []string                     `json:"keywords"`
	Homepage        string                       `json:"homepage"`
	License         interface{}                  `json:"license"`
	Authors         []Author                     `json:"authors"`
	Require         map[string]string            `json:"require"`
	RequireDev      map[string]string            `json:"require-dev"`
	Conflict        map[string]string            `json:"conflict"`
	Replace         map[string]string            `json:"replace"`
	Provide         map[string]string            `json:"provide"`
	Suggest         map[string]string            `json:"suggest"`
	Autoload        map[string]map[string]string `json:"autoload"`
	AutoloadDev     map[string]map[string]string `json:"autoload-dev"`
	MinimumStability string                      `json:"minimum-stability"`
	PreferStable    bool                         `json:"prefer-stable"`
	Bin             interface{}                  `json:"bin"`
	Extra           map[string]interface{}       `json:"extra"`
	Config          map[string]interface{}       `json:"config"`
	Repositories    []interface{}                `json:"repositories"`
	Scripts         map[string]interface{}       `json:"scripts"`
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
	if composer.Version != "" {
		cfg.Version = composer.Version
	}
	if composer.Homepage != "" {
		cfg.Homepage = composer.Homepage
	}
	if len(composer.Keywords) > 0 {
		cfg.Keywords = composer.Keywords
	}
	if composer.License != nil {
		cfg.License = composer.License
	}
	if len(composer.Authors) > 0 {
		cfg.Authors = composer.Authors
	}

	// Merge dependencies
	if composer.Require != nil {
		cfg.Require = composer.Require
	}
	if composer.RequireDev != nil {
		cfg.RequireDev = composer.RequireDev
	}
	if composer.Conflict != nil {
		cfg.Conflict = composer.Conflict
	}
	if composer.Replace != nil {
		cfg.Replace = composer.Replace
	}
	if composer.Provide != nil {
		cfg.Provide = composer.Provide
	}
	if composer.Suggest != nil {
		cfg.Suggest = composer.Suggest
	}

	// Merge autoload configurations
	if composer.Autoload != nil {
		cfg.Autoload = composer.Autoload
	}
	if composer.AutoloadDev != nil {
		cfg.AutoloadDev = composer.AutoloadDev
	}

	// Merge stability preferences (root-only fields)
	if composer.MinimumStability != "" {
		cfg.MinimumStability = composer.MinimumStability
	}
	cfg.PreferStable = composer.PreferStable

	// Merge other important fields
	if composer.Bin != nil {
		cfg.Bin = composer.Bin
	}
	if composer.Extra != nil {
		cfg.Extra = composer.Extra
	}
	if composer.Config != nil {
		cfg.Config = composer.Config
	}
	if composer.Repositories != nil {
		cfg.Repositories = composer.Repositories
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
				scriptStr = strings.Join(parts, " && ")
			default:
				scriptStr = fmt.Sprintf("%v", script)
			}
			cfg.Scripts[name] = scriptStr
		}
	}
}
