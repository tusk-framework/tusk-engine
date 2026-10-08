package roadrunner

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tusk-framework/tusk-engine/internal/config"
	"go.yaml.in/yaml/v2"
)

type fileConfig struct {
	Version string         `yaml:"version"`
	Server  serverConfig   `yaml:"server"`
	HTTP    httpConfig     `yaml:"http"`
	Status  statusConfig   `yaml:"status"`
	RPC     rpcConfig      `yaml:"rpc"`
	Logs    logsConfig     `yaml:"logs"`
	Metrics *metricsConfig `yaml:"metrics,omitempty"`
	Jobs    *jobsConfig    `yaml:"jobs,omitempty"`
}

type serverConfig struct {
	Command string `yaml:"command"`
	Relay   string `yaml:"relay"`
}

type httpConfig struct {
	Address        string     `yaml:"address"`
	MaxRequestSize int64      `yaml:"max_request_size"`
	Middleware     []string   `yaml:"middleware,omitempty"`
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

type statusConfig struct {
	Address string `yaml:"address"`
}

type rpcConfig struct {
	Listen string `yaml:"listen"`
}

type metricsConfig struct {
	Address string `yaml:"address"`
}

type jobsConfig struct {
	Consume   []string                     `yaml:"consume"`
	Pipelines map[string]jobPipelineConfig `yaml:"pipelines"`
}

type jobPipelineConfig struct {
	Driver string         `yaml:"driver"`
	Config map[string]any `yaml:"config,omitempty"`
}

const generatedWorkerPath = ".tusk/runtime/worker.php"

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
	if strings.TrimSpace(cfg.PhpBinary) == "" {
		return nil, fmt.Errorf("php_binary is required")
	}
	if err := cfg.Runtime.Validate(); err != nil {
		return nil, err
	}

	workerPath := filepath.ToSlash(filepath.Join(cfg.ProjectRoot, generatedWorkerPath))
	command := fmt.Sprintf("%s %s", cfg.PhpBinary, workerPath)
	address := strings.TrimSpace(cfg.Address)
	if address == "" {
		address = "127.0.0.1"
	}

	server := serverConfig{
		Command: command,
		Relay:   "pipes",
	}

	projected := fileConfig{
		Version: "3",
		Server:  server,
		HTTP: httpConfig{
			Address:        fmt.Sprintf("%s:%d", address, cfg.Port),
			MaxRequestSize: (cfg.MaxBodyBytes + 1024*1024 - 1) / (1024 * 1024),
			Middleware:     []string{"http_metrics"},
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
		Status: statusConfig{Address: cfg.Runtime.StatusAddress},
		RPC:    rpcConfig{Listen: cfg.Runtime.RPCAddress},
		Logs: logsConfig{
			Mode:  "production",
			Level: "warn",
		},
	}
	projected.Metrics = &metricsConfig{Address: cfg.Runtime.MetricsAddress}
	if len(cfg.Jobs.Consume) > 0 || len(cfg.Jobs.Pipelines) > 0 {
		pipelines := make(map[string]jobPipelineConfig, len(cfg.Jobs.Pipelines))
		for name, pipeline := range cfg.Jobs.Pipelines {
			pipelines[name] = jobPipelineConfig{Driver: pipeline.Driver, Config: pipeline.Config}
		}
		projected.Jobs = &jobsConfig{Consume: append([]string(nil), cfg.Jobs.Consume...), Pipelines: pipelines}
	}

	return yaml.Marshal(projected)
}
