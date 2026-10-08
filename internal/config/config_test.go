package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tusk-framework/tusk-engine/internal/components"
)

func TestDefaultConfigHasSafeRequestLimit(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxBodyBytes != 10*1024*1024 {
		t.Fatalf("MaxBodyBytes = %d, want %d", cfg.MaxBodyBytes, 10*1024*1024)
	}
}

func TestDefaultConfigUsesDisabledLoopbackControl(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Control.Enabled {
		t.Fatal("control API must be disabled by default")
	}
	if cfg.Control.Address != "127.0.0.1" {
		t.Fatalf("expected loopback address, got %q", cfg.Control.Address)
	}
	if cfg.Control.Port != 9091 {
		t.Fatalf("expected control port 9091, got %d", cfg.Control.Port)
	}
}

func TestDefaultConfigUsesLoopbackPrometheusEndpoint(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Runtime.MetricsAddress != "127.0.0.1:2112" {
		t.Fatalf("metrics address = %q, want 127.0.0.1:2112", cfg.Runtime.MetricsAddress)
	}
}

func TestDefaultConfigUsesLoopbackRuntimeDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Runtime.StatusAddress != "127.0.0.1:2114" {
		t.Fatalf("runtime status address = %q, want 127.0.0.1:2114", cfg.Runtime.StatusAddress)
	}
	if cfg.Runtime.RPCAddress != "tcp://127.0.0.1:6001" {
		t.Fatalf("runtime RPC address = %q, want tcp://127.0.0.1:6001", cfg.Runtime.RPCAddress)
	}
	if cfg.Runtime.StartupTimeout <= 0 || cfg.Runtime.ProbeInterval <= 0 {
		t.Fatalf("runtime timings must be positive: %+v", cfg.Runtime)
	}
	if cfg.Runtime.StartupTimeout > 5*time.Minute || cfg.Runtime.ProbeInterval > time.Minute {
		t.Fatalf("runtime timings must be bounded: %+v", cfg.Runtime)
	}
}

func TestRuntimeConfigValidationRejectsUnsafeAddressesAndTimings(t *testing.T) {
	base := DefaultConfig().Runtime
	tests := []struct {
		name   string
		mutate func(*RuntimeConfig)
		want   string
	}{
		{name: "remote status", mutate: func(c *RuntimeConfig) { c.StatusAddress = "0.0.0.0:2114" }, want: "status address"},
		{name: "remote rpc", mutate: func(c *RuntimeConfig) { c.RPCAddress = "tcp://example.test:6001" }, want: "RPC address"},
		{name: "remote metrics", mutate: func(c *RuntimeConfig) { c.MetricsAddress = "0.0.0.0:2112" }, want: "metrics address"},
		{name: "malformed status", mutate: func(c *RuntimeConfig) { c.StatusAddress = "not-an-address" }, want: "status address"},
		{name: "malformed rpc", mutate: func(c *RuntimeConfig) { c.RPCAddress = "http://127.0.0.1:6001" }, want: "RPC address"},
		{name: "zero timeout", mutate: func(c *RuntimeConfig) { c.StartupTimeout = 0 }, want: "startup timeout"},
		{name: "negative interval", mutate: func(c *RuntimeConfig) { c.ProbeInterval = -time.Second }, want: "probe interval"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			tt.mutate(&cfg)
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestControlConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  ControlConfig
		wantErr string
	}{
		{
			name: "loopback does not require token",
			config: ControlConfig{
				Enabled: true,
				Address: "127.0.0.1",
				Port:    9091,
			},
		},
		{
			name: "remote requires token",
			config: ControlConfig{
				Enabled: true,
				Address: "0.0.0.0",
				Port:    9091,
			},
			wantErr: "token",
		},
		{
			name: "remote token is accepted",
			config: ControlConfig{
				Enabled: true,
				Address: "0.0.0.0",
				Port:    9091,
				Token:   "secret",
			},
		},
		{
			name: "port must be positive",
			config: ControlConfig{
				Enabled: true,
				Address: "127.0.0.1",
				Port:    0,
			},
			wantErr: "port",
		},
		{
			name: "port must fit TCP",
			config: ControlConfig{
				Enabled: true,
				Address: "127.0.0.1",
				Port:    65536,
			},
			wantErr: "port",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr == "" && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestControlConfigValidationRejectsUnsafeMetricsPath(t *testing.T) {
	for _, path := range []string{"metrics", "/v1/metrics?token=secret", "/v1/healthz"} {
		cfg := ControlConfig{Enabled: true, Address: "127.0.0.1", Port: 9091, MetricsPath: path}
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "metrics path") {
			t.Fatalf("Validate(%q) error = %v, want metrics path error", path, err)
		}
	}
}

func TestLoadConfigMergesControlAndMetricsSettings(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, "tusk.json"), `{
  "control": {
    "enabled": true,
    "port": 9191
  },
  "runtime": {
    "metrics_address": "127.0.0.1:9211"
  }
}`)

	cfg, err := loadConfigFromDir(root)
	if err != nil {
		t.Fatalf("loadConfigFromDir() error = %v", err)
	}
	if !cfg.Control.Enabled || cfg.Control.Port != 9191 {
		t.Fatalf("control settings were not loaded: %+v", cfg.Control)
	}
	if cfg.Runtime.MetricsAddress != "127.0.0.1:9211" {
		t.Fatalf("metrics settings were not loaded: %+v", cfg.Runtime)
	}
}

func TestDefaultConfigHasNoComponentOverrides(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Components != nil {
		t.Fatalf("Components = %#v, want nil until configured", cfg.Components)
	}
}

func TestLoadConfigMergesComponentOverrides(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, "tusk.json"), `{
  "components": {
    "tusk.resilience": {
      "deadline": "2s",
      "max_attempts": 4,
      "token": "top-secret"
    }
  }
}`)

	cfg, err := loadConfigFromDir(root)
	if err != nil {
		t.Fatalf("loadConfigFromDir() error = %v", err)
	}
	resilience := cfg.Components["tusk.resilience"]
	if resilience["deadline"] != "2s" || resilience["max_attempts"] != float64(4) || resilience["token"] != "top-secret" {
		t.Fatalf("component configuration = %#v, want decoded overrides", resilience)
	}
}

func TestLoadConfigRejectsNonObjectComponentConfiguration(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, "tusk.json"), `{
  "components": {
    "tusk.resilience": "not-an-object"
  }
}`)

	if _, err := loadConfigFromDir(root); err == nil || !strings.Contains(err.Error(), "parse tusk.json") {
		t.Fatalf("loadConfigFromDir() error = %v, want component JSON parse failure", err)
	}
}

func TestLoadConfigLoadsAndValidatesJobs(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, "tusk.json"), `{"jobs":{"consume":["emails","reports"],"pipelines":{"emails":{"driver":"amqp","config":{"url":"${JOBS_URL}","fallback":"${QUEUE_NAME:-mail}","nested":{"enabled":true}}},"reports":{"driver":"memory","config":{"concurrency":4}}}}}`)

	cfg, err := loadConfigFromDir(root)
	if err != nil {
		t.Fatalf("loadConfigFromDir() error = %v", err)
	}
	if !reflect.DeepEqual(cfg.Jobs.Consume, []string{"emails", "reports"}) {
		t.Fatalf("Jobs.Consume = %#v", cfg.Jobs.Consume)
	}
	if cfg.Jobs.Pipelines["emails"].Config["url"] != "${JOBS_URL}" || cfg.Jobs.Pipelines["emails"].Config["fallback"] != "${QUEUE_NAME:-mail}" {
		t.Fatalf("environment placeholders must remain unexpanded: %#v", cfg.Jobs.Pipelines["emails"].Config)
	}
}

func TestLoadConfigRejectsInvalidJobs(t *testing.T) {
	tests := []struct {
		name, json, want string
	}{
		{name: "missing consumed pipeline", json: `{"jobs":{"consume":["emails"],"pipelines":{}}}`, want: "consumed pipeline"},
		{name: "duplicate consumed pipeline", json: `{"jobs":{"consume":["emails","emails"],"pipelines":{"emails":{"driver":"amqp"}}}}`, want: "duplicate"},
		{name: "invalid identifier", json: `{"jobs":{"consume":["bad/name"],"pipelines":{"bad/name":{"driver":"amqp"}}}}`, want: "identifier"},
		{name: "empty driver", json: `{"jobs":{"pipelines":{"emails":{"driver":"  "}}}}`, want: "driver"},
		{name: "unsupported config value", json: "", want: "JSON-compatible"},
		{name: "malformed interpolation", json: `{"jobs":{"pipelines":{"emails":{"driver":"amqp","config":{"url":"${JOBS_URL"}}}}}`, want: "placeholder"},
		{name: "unsafe interpolation", json: `{"jobs":{"pipelines":{"emails":{"driver":"amqp","config":{"url":"${BAD-NAME}"}}}}}`, want: "placeholder"},
		{name: "unsupported interpolation", json: `{"jobs":{"pipelines":{"emails":{"driver":"amqp","config":{"url":"$JOBS_URL"}}}}}`, want: "placeholder"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.json != "" {
				writeConfigFile(t, filepath.Join(root, "tusk.json"), tt.json)
				_, err := loadConfigFromDir(root)
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("loadConfigFromDir() error = %v, want %q", err, tt.want)
				}
				return
			}
			cfg := DefaultConfig()
			cfg.Jobs = JobsConfig{Pipelines: map[string]JobPipelineConfig{"emails": {Driver: "amqp", Config: map[string]any{"unsupported": make(chan int)}}}}
			if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validateConfig() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadConfigRedactsJobPlaceholderValuesFromErrors(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, "tusk.json"), `{"jobs":{"consume":["missing"],"pipelines":{"private":{"driver":"amqp","config":{"password":"sensitive-value"}}}}}`)
	_, err := loadConfigFromDir(root)
	if err == nil {
		t.Fatal("loadConfigFromDir() accepted a missing consumed pipeline")
	}
	if strings.Contains(err.Error(), "sensitive-value") {
		t.Fatalf("error leaked configured secret: %v", err)
	}
}

func TestJobsConfigValidatePreservesSupportedPlaceholders(t *testing.T) {
	cfg := JobsConfig{Pipelines: map[string]JobPipelineConfig{
		"emails": {Driver: "amqp", Config: map[string]any{
			"url":   "amqp://${JOBS_USER}:${JOBS_PASSWORD}@broker",
			"queue": "${QUEUE_NAME:-mail}",
		}},
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestJobsConfigValidateAllowsLiteralDollarSigns(t *testing.T) {
	cfg := JobsConfig{Pipelines: map[string]JobPipelineConfig{
		"billing": {Driver: "memory", Config: map[string]any{"price": "$5.00", "expression": "cost $ + tax"}},
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() rejected literal dollar signs: %v", err)
	}
}

func TestLoadConfigPreservesJobsWithoutConsumption(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, "tusk.json"), `{"jobs":{"pipelines":{"emails":{"driver":"amqp","config":{"queue":"emails"}}}}}`)
	cfg, err := loadConfigFromDir(root)
	if err != nil {
		t.Fatalf("loadConfigFromDir() error = %v", err)
	}
	if len(cfg.Jobs.Consume) != 0 || cfg.Jobs.Pipelines["emails"].Driver != "amqp" {
		t.Fatalf("Jobs = %#v, want configured pipeline without consumption", cfg.Jobs)
	}
}

func TestJobsConfigValidateRedactsInvalidConfiguredValues(t *testing.T) {
	const secret = "private-secret-value"
	cfg := JobsConfig{Pipelines: map[string]JobPipelineConfig{
		"emails": {Driver: "amqp", Config: map[string]any{"credential": secret, "endpoint": "${INVALID-NAME}"}},
	}}
	err := cfg.Validate()
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("Validate() error = %v, expected redacted validation error", err)
	}
}

func TestLoadConfigLoadsAndCopiesComponentConfiguration(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, "tusk.json"), `{
  "components": {
    "default-resilience": {
      "deadline": "5s",
      "max_attempts": 3
    }
  }
}`)

	cfg, err := loadConfigFromDir(root)
	if err != nil {
		t.Fatalf("loadConfigFromDir() error = %v", err)
	}
	want := components.Configuration{"deadline": "5s", "max_attempts": float64(3)}
	if !reflect.DeepEqual(cfg.Components["default-resilience"], want) {
		t.Fatalf("component config = %#v, want %#v", cfg.Components["default-resilience"], want)
	}

	overlay := &Config{Components: map[string]components.Configuration{
		"default-resilience": {"nested": map[string]any{"value": "original"}},
	}}
	merged := DefaultConfig()
	mergeConfig(merged, overlay)
	mutated := overlay.Components["default-resilience"]["nested"].(map[string]any)
	mutated["value"] = "changed"
	if got := merged.Components["default-resilience"]["nested"].(map[string]any)["value"]; got != "original" {
		t.Fatalf("merged component config shares nested map, got %v", got)
	}
}

func TestLoadConfigMergesScriptsWithTuskPrecedence(t *testing.T) {
	root := t.TempDir()

	writeConfigFile(t, filepath.Join(root, "composer.json"), `{
  "scripts": {
    "build": "composer-build",
    "shared": "composer-shared"
  }
}`)
	writeConfigFile(t, filepath.Join(root, "tusk.json"), `{
  "port": 9000,
  "scripts": {
    "dev": "tusk-dev",
    "shared": "tusk-shared"
  }
}`)

	cfg, err := loadConfigFromDir(root)
	if err != nil {
		t.Fatalf("loadConfigFromDir() error = %v", err)
	}

	if cfg.Port != 9000 {
		t.Fatalf("Port = %d, want 9000", cfg.Port)
	}
	if cfg.Scripts["build"] != "composer-build" {
		t.Fatalf("composer script was lost: %#v", cfg.Scripts)
	}
	if cfg.Scripts["dev"] != "tusk-dev" {
		t.Fatalf("tusk script missing: %#v", cfg.Scripts)
	}
	if cfg.Scripts["shared"] != "tusk-shared" {
		t.Fatalf("tusk script did not win conflict: %#v", cfg.Scripts)
	}
}

func TestLoadConfigResolvesProjectRootAndRejectsInvalidLimits(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, "tusk.json"), `{
  "project_root": ".",
  "max_body_bytes": 0
}`)

	if _, err := loadConfigFromDir(root); err == nil {
		t.Fatal("loadConfigFromDir() accepted zero max_body_bytes")
	}
}

func writeConfigFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}
