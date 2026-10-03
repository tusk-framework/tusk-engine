package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds the Tusk Engine configuration
type Config struct {
	// Server configuration
	Port    int    `json:"port"`
	Address string `json:"address"`

	// Worker configuration
	WorkerCount    int               `json:"worker_count"`
	WorkerCommand  string            `json:"worker_command"`
	PhpBinary      string            `json:"php_binary"`
	PhpIni         string            `json:"php_ini"`
	ProjectRoot    string            `json:"project_root"`
	PublicDir      string            `json:"public_dir"`
	Timeout        int               `json:"timeout"`
	MaxBodyBytes   int64             `json:"max_body_bytes"`
	MaxUploadBytes int64             `json:"max_upload_bytes"`
	MaxUploadFiles int               `json:"max_upload_files"`
	Scripts        map[string]string `json:"scripts"`

	// Package management (from composer.json)
	Name             string                       `json:"name,omitempty"`
	Description      string                       `json:"description,omitempty"`
	Type             string                       `json:"type,omitempty"`
	Version          string                       `json:"version,omitempty"`
	Keywords         []string                     `json:"keywords,omitempty"`
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
	Name             string                       `json:"name"`
	Description      string                       `json:"description"`
	Type             string                       `json:"type"`
	Version          string                       `json:"version"`
	Keywords         []string                     `json:"keywords"`
	Homepage         string                       `json:"homepage"`
	License          interface{}                  `json:"license"`
	Authors          []Author                     `json:"authors"`
	Require          map[string]string            `json:"require"`
	RequireDev       map[string]string            `json:"require-dev"`
	Conflict         map[string]string            `json:"conflict"`
	Replace          map[string]string            `json:"replace"`
	Provide          map[string]string            `json:"provide"`
	Suggest          map[string]string            `json:"suggest"`
	Autoload         map[string]map[string]string `json:"autoload"`
	AutoloadDev      map[string]map[string]string `json:"autoload-dev"`
	MinimumStability string                       `json:"minimum-stability"`
	PreferStable     bool                         `json:"prefer-stable"`
	Bin              interface{}                  `json:"bin"`
	Extra            map[string]interface{}       `json:"extra"`
	Config           map[string]interface{}       `json:"config"`
	Repositories     []interface{}                `json:"repositories"`
	Scripts          map[string]interface{}       `json:"scripts"`
}

const defaultRequestLimit int64 = 10 * 1024 * 1024

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Port:           8080,
		Address:        "0.0.0.0",
		WorkerCount:    4, // Default to a reasonable number
		WorkerCommand:  "worker.php",
		PhpBinary:      "php",
		PhpIni:         "", // Empty means use system default
		ProjectRoot:    "./",
		PublicDir:      "public",
		Timeout:        30,
		MaxBodyBytes:   defaultRequestLimit,
		MaxUploadBytes: defaultRequestLimit,
		MaxUploadFiles: 20,
		Scripts:        make(map[string]string),
	}
}

// LoadConfig reads configuration from the current directory.
func LoadConfig() *Config {
	cfg, err := loadConfigFromDir(".")
	if err != nil {
		fmt.Printf("Warning: Failed to load configuration: %v. Using safe defaults where necessary.\n", err)
	}
	return cfg
}

func loadConfigFromDir(root string) (*Config, error) {
	cfg := DefaultConfig()

	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return cfg, fmt.Errorf("resolve project root: %w", err)
	}

	loadComposerConfig(cfg, filepath.Join(absoluteRoot, "composer.json"))

	tuskPath := filepath.Join(absoluteRoot, "tusk.json")
	data, err := os.ReadFile(tuskPath)
	if err != nil {
		if os.IsNotExist(err) {
			cfg.ProjectRoot = absoluteRoot
			return cfg, validateConfig(cfg)
		}
		return cfg, fmt.Errorf("read tusk.json: %w", err)
	}

	composerScripts := cfg.Scripts
	overlay := DefaultConfig()
	if err := json.Unmarshal(data, overlay); err != nil {
		return cfg, fmt.Errorf("parse tusk.json: %w", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return cfg, fmt.Errorf("parse tusk.json fields: %w", err)
	}
	if _, present := raw["max_body_bytes"]; present && overlay.MaxBodyBytes <= 0 {
		return cfg, fmt.Errorf("max_body_bytes must be positive")
	}
	if _, present := raw["max_upload_bytes"]; present && overlay.MaxUploadBytes <= 0 {
		return cfg, fmt.Errorf("max_upload_bytes must be positive")
	}
	if _, present := raw["max_upload_files"]; present && overlay.MaxUploadFiles <= 0 {
		return cfg, fmt.Errorf("max_upload_files must be positive")
	}

	mergeConfig(cfg, overlay)
	cfg.Scripts = mergeScripts(composerScripts, overlay.Scripts)

	if cfg.ProjectRoot == "" || cfg.ProjectRoot == "." || cfg.ProjectRoot == "./" {
		cfg.ProjectRoot = absoluteRoot
	} else if !filepath.IsAbs(cfg.ProjectRoot) {
		cfg.ProjectRoot = filepath.Join(absoluteRoot, cfg.ProjectRoot)
	} else {
		cfg.ProjectRoot = filepath.Clean(cfg.ProjectRoot)
	}

	return cfg, validateConfig(cfg)
}

func mergeConfig(dst, overlay *Config) {
	if overlay.Port != 0 {
		dst.Port = overlay.Port
	}
	if overlay.Address != "" {
		dst.Address = overlay.Address
	}
	if overlay.WorkerCount != 0 {
		dst.WorkerCount = overlay.WorkerCount
	}
	if overlay.WorkerCommand != "" {
		dst.WorkerCommand = overlay.WorkerCommand
	}
	if overlay.PhpBinary != "" {
		dst.PhpBinary = overlay.PhpBinary
	}
	if overlay.PhpIni != "" {
		dst.PhpIni = overlay.PhpIni
	}
	if overlay.ProjectRoot != "" {
		dst.ProjectRoot = overlay.ProjectRoot
	}
	if overlay.PublicDir != "" {
		dst.PublicDir = overlay.PublicDir
	}
	if overlay.Timeout != 0 {
		dst.Timeout = overlay.Timeout
	}
	if overlay.MaxBodyBytes != 0 {
		dst.MaxBodyBytes = overlay.MaxBodyBytes
	}
	if overlay.MaxUploadBytes != 0 {
		dst.MaxUploadBytes = overlay.MaxUploadBytes
	}
	if overlay.MaxUploadFiles != 0 {
		dst.MaxUploadFiles = overlay.MaxUploadFiles
	}
}

func mergeScripts(base, overlay map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(overlay))
	for name, script := range base {
		merged[name] = script
	}
	for name, script := range overlay {
		merged[name] = script
	}
	return merged
}

func validateConfig(cfg *Config) error {
	if cfg.MaxBodyBytes <= 0 || cfg.MaxUploadBytes <= 0 || cfg.MaxUploadFiles <= 0 {
		return fmt.Errorf("request limits must be positive")
	}
	return nil
}

// loadComposerConfig loads configuration from composer.json if it exists.
func loadComposerConfig(cfg *Config, path string) {
	file, err := os.Open(path)
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
