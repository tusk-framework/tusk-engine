package server

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tusk-framework/tusk-engine/internal/config"
	"github.com/tusk-framework/tusk-engine/internal/metrics"
	"github.com/tusk-framework/tusk-engine/internal/worker"
)

// Server is the HTTP server for Tusk
type Server struct {
	cfg  *config.Config
	pool *worker.Pool
}

// NewServer creates a new HTTP server
func NewServer(cfg *config.Config, pool *worker.Pool) *Server {
	return &Server{
		cfg:  cfg,
		pool: pool,
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/", s.handleRequest)

	addr := fmt.Sprintf("%s:%d", s.cfg.Address, s.cfg.Port)
	fmt.Printf("Tusk Engine listening on %s\n", addr)

	return http.ListenAndServe(addr, mux)
}

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	// 1. Read Body
	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body.Close()

	// 2. Extract Headers
	headers := make(map[string][]string)
	for k, v := range r.Header {
		headers[k] = v
	}

	// 3. Construct internal request
	req := map[string]interface{}{
		"method":  r.Method,
		"url":     r.RequestURI,
		"headers": headers,
		"body":    string(bodyBytes),
	}

	// 4. Forward to worker
	start := time.Now()
	metrics.WorkersActive.Inc()
	defer metrics.WorkersActive.Dec()

	resp, err := s.pool.HandleRequest(req)

	duration := time.Since(start).Seconds()
	metrics.RequestDuration.WithLabelValues(r.Method).Observe(duration)
	if err != nil {
		fmt.Printf("Engine Relay Error: %v\n", err)
		http.Error(w, fmt.Sprintf("Engine Error: %v", err), http.StatusBadGateway)
		return
	}

	// 5. Parse response
	if respHeaders, ok := resp["headers"].(map[string]interface{}); ok {
		for k, v := range respHeaders {
			switch val := v.(type) {
			case string:
				w.Header().Set(k, val)
			case []interface{}:
				for _, h := range val {
					if str, ok := h.(string); ok {
						w.Header().Add(k, str)
					}
				}
			}
		}
	}

	// Write Status
	status := http.StatusOK
	if statusVal, ok := resp["status"].(float64); ok {
		status = int(statusVal)
	}
	w.WriteHeader(status)

	metrics.RequestsTotal.WithLabelValues(r.Method, strconv.Itoa(status)).Inc()

	// 5. Log Request
	fmt.Printf("[%s] %s %s - %d (%.3fs)\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, r.URL.Path, status, duration)

	// Write Body
	if body, ok := resp["body"].(string); ok {
		w.Write([]byte(body))
	} else {
		w.Write([]byte("Invalid response body from worker"))
	}
}
