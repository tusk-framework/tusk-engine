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
	stopOnce sync.Once
	stopDone chan struct{}
	stopErr  error
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
	doneOnce sync.Once
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
		stopDone: make(chan struct{}),
	}
	mux := http.NewServeMux()
	mux.HandleFunc(resilienceIngestPath, s.handleSnapshot)
	s.http = &http.Server{Handler: mux, ReadHeaderTimeout: time.Second, ReadTimeout: 5 * time.Second}
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
	stopped := s.stopped
	if stopped && (err == http.ErrServerClosed || errors.Is(err, net.ErrClosed)) {
		err = nil
	}
	s.startErr = err
	s.stopped = true
	s.token = ""
	s.mu.Unlock()
	if err != nil && !stopped {
		_ = s.http.Close()
	}
	s.doneOnce.Do(func() { close(s.done) })
	return err
}

func (s *ResilienceIngestServer) WaitReady(ctx context.Context) error {
	terminalError := func() error {
		s.mu.Lock()
		err := s.startErr
		s.mu.Unlock()
		if err == nil {
			return errors.New("resilience ingest stopped before readiness")
		}
		return err
	}
	select {
	case <-s.done:
		return terminalError()
	default:
	}
	select {
	case <-s.ready:
		select {
		case <-s.done:
			return terminalError()
		default:
		}
		s.mu.Lock()
		stopped, err := s.stopped, s.startErr
		s.mu.Unlock()
		if stopped {
			if err != nil {
				return err
			}
			return errors.New("resilience ingest stopped before readiness")
		}
		return nil
	case <-s.done:
		return terminalError()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *ResilienceIngestServer) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.stopOnce.Do(func() {
		go func() {
			s.stopErr = s.stop(ctx)
			close(s.stopDone)
		}()
	})
	select {
	case <-s.stopDone:
		return s.stopErr
	case <-ctx.Done():
		waitContext, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		select {
		case <-s.stopDone:
			return s.stopErr
		case <-waitContext.Done():
			return waitContext.Err()
		}
	}
}

func (s *ResilienceIngestServer) stop(ctx context.Context) error {
	s.mu.Lock()
	started := s.started
	if !s.stopped {
		s.stopped = true
		s.token = ""
	}
	if !started {
		s.startErr = errors.New("resilience ingest stopped before start")
	}
	s.mu.Unlock()
	var err error
	if started {
		shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancelShutdown()
		err = s.http.Shutdown(shutdownContext)
		if err != nil {
			_ = s.http.Close()
			err = nil
		}
	}
	_ = s.listener.Close()
	if !started {
		s.doneOnce.Do(func() { close(s.done) })
	} else {
		waitContext, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		select {
		case <-s.done:
			err = nil
		case <-waitContext.Done():
			err = waitContext.Err()
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
