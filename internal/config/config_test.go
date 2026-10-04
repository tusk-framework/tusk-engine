package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultConfigHasSafeRequestLimits(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxBodyBytes != 10*1024*1024 {
		t.Fatalf("MaxBodyBytes = %d, want %d", cfg.MaxBodyBytes, 10*1024*1024)
	}
	if cfg.MaxUploadBytes != 10*1024*1024 {
		t.Fatalf("MaxUploadBytes = %d, want %d", cfg.MaxUploadBytes, 10*1024*1024)
	}
	if cfg.MaxUploadFiles != 20 {
		t.Fatalf("MaxUploadFiles = %d, want 20", cfg.MaxUploadFiles)
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

func TestDefaultConfigDisablesPrometheusEndpoint(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Metrics.Enabled {
		t.Fatal("metrics endpoint must be disabled by default")
	}
	if cfg.Metrics.Address != "127.0.0.1:2112" {
		t.Fatalf("metrics address = %q, want 127.0.0.1:2112", cfg.Metrics.Address)
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

func TestMetricsConfigValidationRejectsRemoteEndpoint(t *testing.T) {
	cfg := MetricsConfig{Enabled: true, Address: "0.0.0.0:2112"}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "metrics address") {
		t.Fatalf("Validate() error = %v, want metrics address error", err)
	}
}

func TestLoadConfigMergesControlAndMetricsSettings(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, "tusk.json"), `{
  "control": {
    "enabled": true,
    "port": 9191
  },
  "metrics": {
    "enabled": true,
    "address": "127.0.0.1:9211"
  }
}`)

	cfg, err := loadConfigFromDir(root)
	if err != nil {
		t.Fatalf("loadConfigFromDir() error = %v", err)
	}
	if !cfg.Control.Enabled || cfg.Control.Port != 9191 {
		t.Fatalf("control settings were not loaded: %+v", cfg.Control)
	}
	if !cfg.Metrics.Enabled || cfg.Metrics.Address != "127.0.0.1:9211" {
		t.Fatalf("metrics settings were not loaded: %+v", cfg.Metrics)
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
