package config

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tusk-framework/tusk-engine/internal/components"
)

// Config holds the Tusk Engine configuration
type Config struct {
	// Server configuration
	Port       int                                 `json:"port"`
	Address    string                              `json:"address"`
	Control    ControlConfig                       `json:"control"`
	Runtime    RuntimeConfig                       `json:"runtime"`
	Jobs       JobsConfig                          `json:"jobs,omitempty"`
	Components map[string]components.Configuration `json:"components,omitempty"`

	// RoadRunner and project configuration
	WorkerCount  int               `json:"worker_count"`
	PhpBinary    string            `json:"php_binary"`
	ProjectRoot  string            `json:"project_root"`
	Timeout      int               `json:"timeout"`
	MaxBodyBytes int64             `json:"max_body_bytes"`
	Scripts      map[string]string `json:"scripts"`

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

// ControlConfig configures the local control-plane HTTP server.
type ControlConfig struct {
	Enabled     bool   `json:"enabled"`
	Address     string `json:"address"`
	Port        int    `json:"port"`
	Token       string `json:"token"`
	MetricsPath string `json:"metrics_path"`
}

// RuntimeConfig configures the Engine-managed RoadRunner lifecycle.
type RuntimeConfig struct {
	StatusAddress  string        `json:"status_address"`
	RPCAddress     string        `json:"rpc_address"`
	MetricsAddress string        `json:"metrics_address"`
	StartupTimeout time.Duration `json:"startup_timeout"`
	ProbeInterval  time.Duration `json:"probe_interval"`
}

// JobsConfig configures RoadRunner job consumption and named pipelines.
type JobsConfig struct {
	Consume   []string                     `json:"consume,omitempty"`
	Pipelines map[string]JobPipelineConfig `json:"pipelines,omitempty"`
}

// JobPipelineConfig describes a RoadRunner Jobs driver and its opaque settings.
type JobPipelineConfig struct {
	Driver string         `json:"driver"`
	Config map[string]any `json:"config,omitempty"`
}

// Validate checks pipeline names, consumption references, drivers, and safe
// JSON configuration values without including configured values in errors.
func (c JobsConfig) Validate() error {
	seen := make(map[string]struct{}, len(c.Consume))
	for _, name := range c.Consume {
		if !validJobIdentifier(name) {
			return fmt.Errorf("jobs.consume contains an invalid pipeline identifier")
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("jobs.consume contains a duplicate pipeline")
		}
		seen[name] = struct{}{}
		if _, exists := c.Pipelines[name]; !exists {
			return fmt.Errorf("jobs.consume references a missing consumed pipeline")
		}
	}
	names := make([]string, 0, len(c.Pipelines))
	for name := range c.Pipelines {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		pipeline := c.Pipelines[name]
		if !validJobIdentifier(name) {
			return fmt.Errorf("jobs.pipelines contains an invalid pipeline identifier")
		}
		if strings.TrimSpace(pipeline.Driver) == "" {
			return fmt.Errorf("jobs pipeline driver is required")
		}
		if err := validateJobValue(pipeline.Config); err != nil {
			return err
		}
	}
	return nil
}

func validJobIdentifier(value string) bool {
	if value == "" || !isASCIIAlpha(value[0]) && value[0] != '_' {
		return false
	}
	for index := 1; index < len(value); index++ {
		char := value[index]
		if !isASCIIAlpha(char) && (char < '0' || char > '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func isASCIIAlpha(char byte) bool {
	return (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z')
}

func validateJobValue(value any) error {
	switch typed := value.(type) {
	case nil, bool, float64, float32, int, int32, int64, uint, uint32, uint64:
		return nil
	case string:
		return validateJobPlaceholders(typed)
	case []any:
		for _, item := range typed {
			if err := validateJobValue(item); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		for _, item := range typed {
			if err := validateJobValue(item); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("jobs pipeline config must contain only JSON-compatible values")
	}
}

func validateJobPlaceholders(value string) error {
	for index := 0; index < len(value); {
		if value[index] != '$' {
			index++
			continue
		}
		if index+1 >= len(value) || value[index+1] != '{' {
			if index+1 < len(value) && (isASCIIAlpha(value[index+1]) || value[index+1] == '_') {
				return fmt.Errorf("jobs pipeline config contains an unsupported environment placeholder")
			}
			index++
			continue
		}
		end := strings.IndexByte(value[index+2:], '}')
		if end < 0 {
			return fmt.Errorf("jobs pipeline config contains a malformed environment placeholder")
		}
		placeholder := value[index+2 : index+2+end]
		name, fallback, hasFallback := strings.Cut(placeholder, ":-")
		if !validEnvironmentName(name) || strings.Contains(placeholder, "${") || strings.Contains(placeholder, "}") || strings.Contains(name, ":") {
			return fmt.Errorf("jobs pipeline config contains an unsafe environment placeholder")
		}
		if hasFallback && (strings.Contains(fallback, "$") || strings.Contains(fallback, "}")) {
			return fmt.Errorf("jobs pipeline config contains an unsafe environment placeholder")
		}
		index += end + 3
	}
	return nil
}

func validEnvironmentName(value string) bool {
	if value == "" || !isASCIIAlpha(value[0]) && value[0] != '_' {
		return false
	}
	for index := 1; index < len(value); index++ {
		char := value[index]
		if !isASCIIAlpha(char) && (char < '0' || char > '9') && char != '_' {
			return false
		}
	}
	return true
}

const (
	defaultRuntimeStatusAddress = "127.0.0.1:2114"
	defaultRuntimeRPCAddress    = "tcp://127.0.0.1:6001"
	defaultRuntimeMetrics       = "127.0.0.1:2112"
	defaultRuntimeStartup       = 30 * time.Second
	defaultRuntimeProbeInterval = 250 * time.Millisecond
	maxRuntimeStartup           = 5 * time.Minute
	maxRuntimeProbeInterval     = time.Minute
)

// Validate checks that runtime control endpoints are local and timings are bounded.
func (c RuntimeConfig) Validate() error {
	if err := validateLoopbackTCPAddress("status address", c.StatusAddress, false); err != nil {
		return err
	}
	if err := validateLoopbackTCPAddress("RPC address", c.RPCAddress, true); err != nil {
		return err
	}
	if err := validateLoopbackTCPAddress("metrics address", c.MetricsAddress, false); err != nil {
		return err
	}
	if c.StartupTimeout <= 0 || c.StartupTimeout > maxRuntimeStartup {
		return fmt.Errorf("startup timeout must be between 1ns and %s", maxRuntimeStartup)
	}
	if c.ProbeInterval <= 0 || c.ProbeInterval > maxRuntimeProbeInterval {
		return fmt.Errorf("probe interval must be between 1ns and %s", maxRuntimeProbeInterval)
	}
	return nil
}

func validateLoopbackTCPAddress(label, value string, urlForm bool) error {
	address := strings.TrimSpace(value)
	if urlForm {
		parsed, err := url.Parse(address)
		if err != nil || parsed.Scheme != "tcp" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" {
			return fmt.Errorf("invalid %s %q", label, value)
		}
		address = parsed.Host
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || port == "" {
		return fmt.Errorf("invalid %s %q", label, value)
	}
	if !isLocalControlAddress(host) {
		return fmt.Errorf("%s must use a loopback address", label)
	}
	portNumber, err := net.LookupPort("tcp", port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("invalid %s %q", label, value)
	}
	return nil
}

// Validate checks whether the control server can be safely exposed.
func (c ControlConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("control port must be between 1 and 65535")
	}
	if err := validateMetricsPath(c.MetricsPath); err != nil {
		return err
	}

	address := strings.TrimSpace(c.Address)
	if address == "" {
		address = "127.0.0.1"
	}

	if !isLocalControlAddress(address) && strings.TrimSpace(c.Token) == "" {
		return fmt.Errorf("control token is required for non-loopback address %q", address)
	}

	return nil
}

func validateMetricsPath(path string) error {
	if path == "" {
		path = "/v1/metrics"
	}
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return fmt.Errorf("metrics path must be an absolute HTTP path")
	}
	parsed, err := url.ParseRequestURI(path)
	if err != nil || parsed.Path != path || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("metrics path must not contain a query or fragment")
	}
	for _, reserved := range []string{"/v1/healthz", "/v1/readyz", "/v1/metadata"} {
		if path == reserved {
			return fmt.Errorf("metrics path %q conflicts with a control endpoint", path)
		}
	}
	return nil
}

func isLocalControlAddress(address string) bool {
	if strings.EqualFold(address, "localhost") {
		return true
	}

	ip := net.ParseIP(address)

	return ip != nil && ip.IsLoopback()
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
		Port:    8080,
		Address: "0.0.0.0",
		Control: ControlConfig{
			Address: "127.0.0.1",
			Port:    9091,
		},
		Runtime: RuntimeConfig{
			StatusAddress:  defaultRuntimeStatusAddress,
			RPCAddress:     defaultRuntimeRPCAddress,
			MetricsAddress: defaultRuntimeMetrics,
			StartupTimeout: defaultRuntimeStartup,
			ProbeInterval:  defaultRuntimeProbeInterval,
		},
		WorkerCount:  4,
		PhpBinary:    "php",
		ProjectRoot:  "./",
		Timeout:      30,
		MaxBodyBytes: defaultRequestLimit,
		Scripts:      make(map[string]string),
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
	if runtimeRaw, present := raw["runtime"]; present {
		if err := validateExplicitRuntimeTiming(runtimeRaw); err != nil {
			return cfg, err
		}
	}

	mergeConfig(cfg, overlay)
	if _, present := raw["control"]; present {
		cfg.Control = overlay.Control
	}
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
	if overlay.PhpBinary != "" {
		dst.PhpBinary = overlay.PhpBinary
	}
	if overlay.ProjectRoot != "" {
		dst.ProjectRoot = overlay.ProjectRoot
	}
	if overlay.Timeout != 0 {
		dst.Timeout = overlay.Timeout
	}
	if overlay.MaxBodyBytes != 0 {
		dst.MaxBodyBytes = overlay.MaxBodyBytes
	}
	if overlay.Runtime.StatusAddress != "" {
		dst.Runtime.StatusAddress = overlay.Runtime.StatusAddress
	}
	if overlay.Runtime.RPCAddress != "" {
		dst.Runtime.RPCAddress = overlay.Runtime.RPCAddress
	}
	if overlay.Runtime.MetricsAddress != "" {
		dst.Runtime.MetricsAddress = overlay.Runtime.MetricsAddress
	}
	if overlay.Runtime.StartupTimeout != 0 {
		dst.Runtime.StartupTimeout = overlay.Runtime.StartupTimeout
	}
	if overlay.Runtime.ProbeInterval != 0 {
		dst.Runtime.ProbeInterval = overlay.Runtime.ProbeInterval
	}
	if overlay.Components != nil {
		dst.Components = cloneComponentConfigurations(overlay.Components)
	}
	if len(overlay.Jobs.Consume) > 0 || overlay.Jobs.Pipelines != nil {
		dst.Jobs = cloneJobsConfig(overlay.Jobs)
	}
}

func cloneJobsConfig(source JobsConfig) JobsConfig {
	clone := JobsConfig{Consume: append([]string(nil), source.Consume...)}
	if source.Pipelines != nil {
		clone.Pipelines = make(map[string]JobPipelineConfig, len(source.Pipelines))
		for name, pipeline := range source.Pipelines {
			copyOf := JobPipelineConfig{Driver: pipeline.Driver}
			if pipeline.Config != nil {
				copyOf.Config = make(map[string]any, len(pipeline.Config))
				for key, value := range pipeline.Config {
					copyOf.Config[key] = cloneComponentValue(value)
				}
			}
			clone.Pipelines[name] = copyOf
		}
	}
	return clone
}

func cloneComponentConfigurations(source map[string]components.Configuration) map[string]components.Configuration {
	clone := make(map[string]components.Configuration, len(source))
	for name, configuration := range source {
		copyOf := make(components.Configuration, len(configuration))
		for key, value := range configuration {
			copyOf[key] = cloneComponentValue(value)
		}
		clone[name] = copyOf
	}
	return clone
}

func cloneComponentValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		copyOf := make(map[string]any, len(typed))
		for key, nested := range typed {
			copyOf[key] = cloneComponentValue(nested)
		}
		return copyOf
	case []any:
		copyOf := make([]any, len(typed))
		for index, nested := range typed {
			copyOf[index] = cloneComponentValue(nested)
		}
		return copyOf
	default:
		return value
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
	if cfg.MaxBodyBytes <= 0 {
		return fmt.Errorf("max_body_bytes must be positive")
	}
	if err := cfg.Runtime.Validate(); err != nil {
		return err
	}
	if err := cfg.Jobs.Validate(); err != nil {
		return err
	}
	return nil
}

func validateExplicitRuntimeTiming(raw json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("parse runtime configuration: %w", err)
	}
	for _, name := range []string{"startup_timeout", "probe_interval"} {
		value, present := fields[name]
		if !present {
			continue
		}
		var duration time.Duration
		if err := json.Unmarshal(value, &duration); err != nil {
			return fmt.Errorf("parse runtime.%s: %w", name, err)
		}
		if duration <= 0 {
			return fmt.Errorf("runtime.%s must be positive", name)
		}
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
