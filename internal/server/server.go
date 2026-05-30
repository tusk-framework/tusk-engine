package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	http *http.Server
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
	s.http = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	fmt.Printf("Tusk Engine listening on %s\n", addr)
	return s.http.ListenAndServe()
}

// Stop stops the HTTP server gracefully
func (s *Server) Stop(ctx context.Context) error {
	if s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	// 1. Static file check
	publicPath := filepath.Join(s.cfg.ProjectRoot, s.cfg.PublicDir, r.URL.Path)
	if stat, err := os.Stat(publicPath); err == nil && !stat.IsDir() && r.URL.Path != "/" {
		http.ServeFile(w, r, publicPath)
		return
	}

	// 2. Extract Headers and Query
	headers := make(map[string][]string)
	for k, v := range r.Header {
		headers[k] = v
	}

	query := make(map[string]string)
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			query[k] = v[0]
		}
	}

	// 3. Handle multipart/form-data and body
	var parsedBody map[string]string
	var uploadedFiles map[string]interface{}
	var rawBody string
	var tempFiles []string

	defer func() {
		for _, tf := range tempFiles {
			os.Remove(tf)
		}
	}()

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Parse multipart form
		if err := r.ParseMultipartForm(10 << 20); err == nil {
			parsedBody = make(map[string]string)
			for k, v := range r.MultipartForm.Value {
				if len(v) > 0 {
					parsedBody[k] = v[0]
				}
			}

			uploadedFiles = make(map[string]interface{})
			for k, files := range r.MultipartForm.File {
				var fileInfoList []map[string]interface{}
				for _, fileHeader := range files {
					file, err := fileHeader.Open()
					if err != nil {
						continue
					}

					// Save to temp file
					tempFile, err := os.CreateTemp("", "tusk-upload-*")
					if err != nil {
						file.Close()
						continue
					}
					io.Copy(tempFile, file)
					tempFile.Close()
					file.Close()

					tempFiles = append(tempFiles, tempFile.Name())

					fileInfoList = append(fileInfoList, map[string]interface{}{
						"name":     fileHeader.Filename,
						"type":     fileHeader.Header.Get("Content-Type"),
						"tmp_name": tempFile.Name(),
						"error":    0,
						"size":     fileHeader.Size,
					})
				}
				if len(fileInfoList) > 0 {
					uploadedFiles[k] = fileInfoList
				}
			}
		}
	} else if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		if err := r.ParseForm(); err == nil {
			parsedBody = make(map[string]string)
			for k, v := range r.PostForm {
				if len(v) > 0 {
					parsedBody[k] = v[0]
				}
			}
		}
	} else {
		// Read raw body
		bodyBytes, _ := io.ReadAll(r.Body)
		rawBody = string(bodyBytes)
	}

	// 4. Extract Cookies
	cookies := make(map[string]string)
	for _, cookie := range r.Cookies() {
		cookies[cookie.Name] = cookie.Value
	}

	// 5. Construct internal request metadata
	req := map[string]interface{}{
		"method":        r.Method,
		"url":           r.RequestURI,
		"query":         query,
		"headers":       headers,
		"cookies":       cookies,
		"body":          rawBody,
		"parsedBody":    parsedBody,
		"uploadedFiles": uploadedFiles,
	}

	// 6. Forward to worker
	start := time.Now()
	metrics.WorkersActive.Inc()
	defer metrics.WorkersActive.Dec()

	resp, err := s.pool.HandleRequest(req)
	defer r.Body.Close()

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
