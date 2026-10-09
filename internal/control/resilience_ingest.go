package control

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tusk-framework/tusk-engine/internal/metrics"
)

const resilienceIngestPath = "/internal/v1/resilience/snapshot"
const maxResilienceBodyBytes = 64 * 1024

type ResilienceIngestServer struct {
	mu       sync.Mutex
	listener net.Listener
	http     *http.Server
	store    *ResilienceDiagnosticsStore
	url      string
	token    string
	started  bool
	stopped  bool
	ready    chan struct{}
	done     chan struct{}
	startErr error
}

func NewResilienceIngestServer() (*ResilienceIngestServer, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		_ = listener.Close()
		return nil, err
	}
	s := &ResilienceIngestServer{
		listener: listener, store: NewResilienceDiagnosticsStore(),
		url: "http://" + listener.Addr().String(), token: hex.EncodeToString(secret),
		ready: make(chan struct{}), done: make(chan struct{}),
	}
	mux := http.NewServeMux()
	mux.HandleFunc(resilienceIngestPath, s.handleSnapshot)
	s.http = &http.Server{Handler: mux}
	return s, nil
}

func (s *ResilienceIngestServer) URL() string { return s.url }

func (s *ResilienceIngestServer) Token() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}

func (s *ResilienceIngestServer) Store() *ResilienceDiagnosticsStore { return s.store }

func (s *ResilienceIngestServer) Start() error {
	s.mu.Lock()
	if s.started || s.stopped {
		s.mu.Unlock()
		return errors.New("resilience ingest cannot start")
	}
	s.started = true
	close(s.ready)
	s.mu.Unlock()
	err := s.http.Serve(s.listener)
	s.mu.Lock()
	if s.stopped && (err == http.ErrServerClosed || errors.Is(err, net.ErrClosed)) {
		err = nil
	}
	s.startErr = err
	s.stopped = true
	s.token = ""
	s.mu.Unlock()
	close(s.done)
	return err
}

func (s *ResilienceIngestServer) WaitReady(ctx context.Context) error {
	select {
	case <-s.ready:
		return nil
	case <-s.done:
		s.mu.Lock()
		err := s.startErr
		s.mu.Unlock()
		if err == nil {
			return errors.New("resilience ingest stopped before readiness")
		}
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *ResilienceIngestServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return nil
	}
	s.stopped = true
	s.token = ""
	started := s.started
	s.mu.Unlock()
	_ = s.listener.Close()
	err := s.http.Shutdown(ctx)
	if started {
		select {
		case <-s.done:
		case <-ctx.Done():
			if err == nil {
				err = ctx.Err()
			}
		}
	}
	s.store.Clear()
	return err
}

func (s *ResilienceIngestServer) handleSnapshot(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	token := s.token
	stopped := s.stopped
	s.mu.Unlock()
	authorization := request.Header.Get("Authorization")
	provided, hasBearer := strings.CutPrefix(authorization, "Bearer ")
	expectedHash := sha256.Sum256([]byte(token))
	providedHash := sha256.Sum256([]byte(provided))
	if stopped || !hasBearer || subtle.ConstantTimeCompare(expectedHash[:], providedHash[:]) != 1 {
		rejectResilience(writer, "unauthorized", http.StatusUnauthorized)
		return
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		rejectResilience(writer, "unsupported media type", http.StatusUnsupportedMediaType)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, maxResilienceBodyBytes))
	if err != nil {
		writeResilienceDecodeError(writer, err)
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var report WorkerResilienceReport
	if err := decoder.Decode(&report); err != nil {
		writeResilienceDecodeError(writer, err)
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			err = errors.New("trailing JSON")
		}
		writeResilienceDecodeError(writer, err)
		return
	}
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		rejectResilience(writer, "unavailable", http.StatusServiceUnavailable)
		return
	}
	err = s.store.Record(report, time.Now())
	s.mu.Unlock()
	if err != nil {
		rejectResilience(writer, "invalid snapshot", http.StatusBadRequest)
		return
	}
	writer.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(writer, "ok\n")
}

func writeResilienceDecodeError(writer http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		rejectResilience(writer, "snapshot too large", http.StatusRequestEntityTooLarge)
		return
	}
	rejectResilience(writer, "invalid snapshot", http.StatusBadRequest)
}

func rejectResilience(writer http.ResponseWriter, message string, status int) {
	metrics.ResilienceReportsRejected.Inc()
	http.Error(writer, message, status)
}
