package control

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/tusk-framework/tusk-engine/internal/config"
)

type Server struct {
	cfg      config.ControlConfig
	provider SnapshotProvider
	metadata Metadata
	mu       sync.Mutex
	http     *http.Server
	ready    chan struct{}
	startErr chan error
}

func NewServer(cfg config.ControlConfig, provider SnapshotProvider, metadata Metadata) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.Address) == "" {
		cfg.Address = "127.0.0.1"
	}
	if provider == nil {
		return nil, fmt.Errorf("control snapshot provider is required")
	}

	return &Server{
		cfg:      cfg,
		provider: provider,
		metadata: metadata,
		ready:    make(chan struct{}),
		startErr: make(chan error, 1),
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	if !s.cfg.Enabled {
		return mux
	}

	mux.Handle("/v1/healthz", s.protected(http.HandlerFunc(s.handleHealth)))
	mux.Handle("/v1/readyz", s.protected(http.HandlerFunc(s.handleReady)))
	mux.Handle("/v1/metadata", s.protected(http.HandlerFunc(s.handleMetadata)))
	mux.Handle("/v1/metrics", s.protected(promhttp.Handler()))
	return mux
}

func (s *Server) Start() error {
	if !s.cfg.Enabled {
		return nil
	}

	address := net.JoinHostPort(s.cfg.Address, strconv.Itoa(s.cfg.Port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		s.startErr <- err
		return err
	}

	server := &http.Server{Handler: s.Handler()}
	s.mu.Lock()
	s.http = server
	s.mu.Unlock()
	close(s.ready)

	err = server.Serve(listener)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) WaitReady(ctx context.Context) error {
	if !s.cfg.Enabled {
		return nil
	}

	select {
	case <-s.ready:
		return nil
	case err := <-s.startErr:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	server := s.http
	s.mu.Unlock()
	if server == nil {
		return nil
	}
	return server.Shutdown(ctx)
}

func (s *Server) protected(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !authorized(request, s.cfg) {
			writer.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (s *Server) handleHealth(writer http.ResponseWriter, _ *http.Request) {
	snapshot := s.provider.Snapshot()
	statusCode := http.StatusOK
	if !snapshot.Healthy() {
		statusCode = http.StatusServiceUnavailable
	}

	writeJSON(writer, statusCode, healthResponse{
		Version:   "v1",
		Status:    healthStatus(snapshot),
		Engine:    string(snapshot.EngineState),
		Timestamp: time.Now().UTC(),
	})
}

func (s *Server) handleReady(writer http.ResponseWriter, _ *http.Request) {
	snapshot := s.provider.Snapshot()
	ready := snapshot.Ready()
	statusCode := http.StatusServiceUnavailable
	status := "not_ready"
	message := readinessMessage(snapshot.ReadinessReason)
	errorCode := "not_ready"
	if ready {
		statusCode = http.StatusOK
		status = "ready"
		message = "ready"
		errorCode = ""
	}

	writeJSON(writer, statusCode, readinessResponse{
		Version: "v1",
		Status:  status,
		Error:   errorCode,
		Message: message,
		Engine:  string(snapshot.EngineState),
		Workers: workerCounts{
			Desired: snapshot.DesiredWorkers,
			Ready:   snapshot.ReadyWorkers,
			Active:  snapshot.ActiveWorkers,
			Known:   snapshot.WorkerCountsKnown,
		},
		Timestamp: time.Now().UTC(),
	})
}

func (s *Server) handleMetadata(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, s.metadata.safeResponse(!isLoopbackAddress(s.cfg.Address)))
}

func healthStatus(snapshot RuntimeSnapshot) string {
	if snapshot.Healthy() {
		return "ok"
	}
	return "unavailable"
}

func readinessMessage(reason ReadinessReason) string {
	switch reason {
	case ReadinessStarting:
		return "engine is starting"
	case ReadinessNoWorkers:
		return "no worker is ready"
	case ReadinessWorkerCrashed:
		return "no worker is ready after a worker failure"
	case ReadinessReady:
		return "ready"
	case ReadinessStopping:
		return "engine is stopping"
	case ReadinessProcessFailed:
		return "engine process failed"
	case ReadinessTimeout:
		return "engine readiness timed out"
	default:
		return "engine is not ready"
	}
}

type healthResponse struct {
	Version   string    `json:"version"`
	Status    string    `json:"status"`
	Engine    string    `json:"engine"`
	Timestamp time.Time `json:"timestamp"`
}

type readinessResponse struct {
	Version   string       `json:"version"`
	Status    string       `json:"status"`
	Error     string       `json:"error,omitempty"`
	Message   string       `json:"message"`
	Engine    string       `json:"engine"`
	Workers   workerCounts `json:"workers"`
	Timestamp time.Time    `json:"timestamp"`
}

type workerCounts struct {
	Desired int  `json:"desired"`
	Ready   int  `json:"ready"`
	Active  int  `json:"active"`
	Known   bool `json:"known"`
}

func writeJSON(writer http.ResponseWriter, statusCode int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(statusCode)
	_ = json.NewEncoder(writer).Encode(value)
}
