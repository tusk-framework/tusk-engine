package roadrunner

import (
	"fmt"
	"strings"

	"github.com/tusk-framework/tusk-engine/internal/config"
	"go.yaml.in/yaml/v2"
)

type fileConfig struct {
	Version string       `yaml:"version"`
	Server  serverConfig `yaml:"server"`
	HTTP    httpConfig   `yaml:"http"`
	Logs    logsConfig   `yaml:"logs"`
}

type serverConfig struct {
	Command string `yaml:"command"`
	Relay   string `yaml:"relay"`
}

type httpConfig struct {
	Address        string     `yaml:"address"`
	MaxRequestSize int64      `yaml:"max_request_size"`
	Pool           poolConfig `yaml:"pool"`
}

type poolConfig struct {
	NumWorkers   int              `yaml:"num_workers"`
	MaxJobs      int              `yaml:"max_jobs"`
	MaxQueueSize int              `yaml:"max_queue_size"`
	Debug        bool             `yaml:"debug"`
	Supervisor   supervisorConfig `yaml:"supervisor"`
}

type supervisorConfig struct {
	MaxWorkerMemory int    `yaml:"max_worker_memory"`
	ExecTTL         string `yaml:"exec_ttl"`
}

type logsConfig struct {
	Mode  string `yaml:"mode"`
	Level string `yaml:"level"`
}

// Project renders the canonical Tusk configuration as a RoadRunner v3 file.
func Project(cfg *config.Config) ([]byte, error) {
	if cfg == nil {
		return nil, fmt.Errorf("project configuration is required")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return nil, fmt.Errorf("port must be between 1 and 65535")
	}
	if cfg.WorkerCount <= 0 {
		return nil, fmt.Errorf("worker_count must be positive")
	}
	if cfg.MaxBodyBytes <= 0 {
		return nil, fmt.Errorf("max_body_bytes must be positive")
	}
	if strings.TrimSpace(cfg.WorkerCommand) == "" {
		return nil, fmt.Errorf("worker_command is required")
	}
	if strings.TrimSpace(cfg.PhpBinary) == "" {
		return nil, fmt.Errorf("php_binary is required")
	}

	command := strings.TrimSpace(cfg.PhpBinary) + " " + strings.TrimSpace(cfg.WorkerCommand)
	address := strings.TrimSpace(cfg.Address)
	if address == "" {
		address = "127.0.0.1"
	}

	projected := fileConfig{
		Version: "3",
		Server: serverConfig{
			Command: command,
			Relay:   "pipes",
		},
		HTTP: httpConfig{
			Address:        fmt.Sprintf("%s:%d", address, cfg.Port),
			MaxRequestSize: (cfg.MaxBodyBytes + 1024*1024 - 1) / (1024 * 1024),
			Pool: poolConfig{
				NumWorkers:   cfg.WorkerCount,
				MaxJobs:      1000,
				MaxQueueSize: 1000,
				Debug:        false,
				Supervisor: supervisorConfig{
					MaxWorkerMemory: 256,
					ExecTTL:         "30s",
				},
			},
		},
		Logs: logsConfig{
			Mode:  "production",
			Level: "warn",
		},
	}

	return yaml.Marshal(projected)
}
