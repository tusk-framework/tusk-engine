package server

import (
	"fmt"
	"io"
	"net/http"

	"github.com/tusk-framework/tusk-engine/internal/config"
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
	headers := make(map[string]string)
	for k, v := range r.Header {
		headers[k] = v[0]
	}

	// 3. Construct internal request
	req := map[string]interface{}{
		"method":  r.Method,
		"url":     r.RequestURI,
		"headers": headers,
		"body":    string(bodyBytes),
	}

	// 4. Forward to worker
	resp, err := s.pool.HandleRequest(req)
	if err != nil {
		fmt.Printf("Engine Relay Error: %v\n", err)
		http.Error(w, fmt.Sprintf("Engine Error: %v", err), http.StatusBadGateway)
		return
	}

	// 5. Parse response
	if respHeaders, ok := resp["headers"].(map[string]interface{}); ok {
		for k, v := range respHeaders {
			if strVal, ok := v.(string); ok {
				w.Header().Set(k, strVal)
			}
		}
	}

	// Write Status
	if status, ok := resp["status"].(float64); ok {
		w.WriteHeader(int(status))
	} else {
		w.WriteHeader(http.StatusOK)
	}

	// Write Body
	if body, ok := resp["body"].(string); ok {
		w.Write([]byte(body))
	} else {
		w.Write([]byte("Invalid response body from worker"))
	}
}
