package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tusk-framework/tusk-engine/internal/metrics"
)

func TestResilienceIngestLoopbackAuthenticationAndRedaction(t *testing.T) {
	ingest, err := NewResilienceIngestServer()
	if err != nil {
		t.Fatal(err)
	}
	address := strings.TrimPrefix(ingest.URL(), "http://")
	host, _, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" {
		t.Fatalf("ingest address = %q, error = %v", address, err)
	}
	if ingest.Token() == "" {
		t.Fatal("missing token")
	}
	done := make(chan error, 1)
	go func() { done <- ingest.Start() }()
	defer func() { _ = ingest.Stop(context.Background()); <-done }()
	payload, _ := json.Marshal(report("opaque-worker", 1, "payments", "open"))
	for _, tt := range []struct {
		name, token string
		status      int
	}{
		{"missing", "", http.StatusUnauthorized}, {"wrong", "wrong", http.StatusUnauthorized}, {"valid", ingest.Token(), http.StatusOK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request, _ := http.NewRequest(http.MethodPost, ingest.URL()+"/internal/v1/resilience/snapshot", bytes.NewReader(payload))
			request.Header.Set("Content-Type", "application/json")
			if tt.token != "" {
				request.Header.Set("Authorization", "Bearer "+tt.token)
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, _ := io.ReadAll(response.Body)
			if response.StatusCode != tt.status {
				t.Fatalf("status = %d, want %d", response.StatusCode, tt.status)
			}
			if strings.Contains(string(body), "payments") || strings.Contains(string(body), "opaque-worker") || strings.Contains(string(body), ingest.Token()) || len(body) > 128 {
				t.Fatalf("response disclosed report or secret: %q", body)
			}
		})
	}
	if ingest.Store().Snapshot(time.Now(), true).Circuits[0].Workers.Open != 1 {
		t.Fatal("valid report not stored")
	}
}

func TestResilienceIngestCountsRejectedReports(t *testing.T) {
	ingest, err := NewResilienceIngestServer()
	if err != nil {
		t.Fatal(err)
	}
	defer ingest.Stop(context.Background())
	count := func() float64 {
		families, err := metrics.Registry.Gather()
		if err != nil {
			t.Fatal(err)
		}
		for _, family := range families {
			if family.GetName() == "tusk_resilience_reports_rejected_total" {
				return family.GetMetric()[0].GetCounter().GetValue()
			}
		}
		return 0
	}
	before := count()
	recorder := httptest.NewRecorder()
	ingest.handleSnapshot(recorder, httptest.NewRequest(http.MethodPost, resilienceIngestPath, strings.NewReader(`{}`)))
	if recorder.Code != http.StatusUnauthorized || count() != before+1 {
		t.Fatal("unauthorized report was not counted")
	}
}

func TestResilienceIngestCannotRecordAfterStop(t *testing.T) {
	ingest, err := NewResilienceIngestServer()
	if err != nil {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	request := httptest.NewRequest(http.MethodPost, resilienceIngestPath, reader)
	request.Header.Set("Authorization", "Bearer "+ingest.Token())
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { ingest.handleSnapshot(recorder, request); close(done) }()
	if _, err := writer.Write([]byte(`{"schema_version":"v1",`)); err != nil {
		t.Fatal(err)
	}
	if err := ingest.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte(`"worker_id":"worker","sequence":1}`)); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	<-done
	if ingest.Store().Snapshot(time.Now(), true).Status != "unavailable" {
		t.Fatal("report stored after shutdown")
	}
}

func TestResilienceIngestWaitReadyAndStopWaitsForServe(t *testing.T) {
	ingest, err := NewResilienceIngestServer()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- ingest.Start() }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := ingest.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if err := ingest.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Stop() returned before private server stopped")
	}
}

func TestResilienceIngestWaitReadyReportsFailureAndStopBeforeStart(t *testing.T) {
	ingest, err := NewResilienceIngestServer()
	if err != nil {
		t.Fatal(err)
	}
	if err := ingest.listener.Close(); err != nil {
		t.Fatal(err)
	}
	startDone := make(chan error, 1)
	go func() { startDone <- ingest.Start() }()
	if err := <-startDone; err == nil {
		t.Fatal("Start() hid listener failure")
	}
	if err := ingest.WaitReady(context.Background()); err == nil {
		t.Fatal("WaitReady() succeeded after listener failure")
	}

	stopped, err := NewResilienceIngestServer()
	if err != nil {
		t.Fatal(err)
	}
	if err := stopped.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := stopped.WaitReady(ctx); err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitReady() after stop-before-start = %v, want immediate stopped error", err)
	}
}

func TestResilienceIngestWaitReadyRejectsRecordedFailureBeforeDoneSignal(t *testing.T) {
	ingest, err := NewResilienceIngestServer()
	if err != nil {
		t.Fatal(err)
	}
	close(ingest.ready)
	ingest.mu.Lock()
	ingest.stopped = true
	ingest.startErr = errors.New("listener failed")
	ingest.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := ingest.WaitReady(ctx); err == nil || !strings.Contains(err.Error(), "listener failed") {
		t.Fatalf("WaitReady() = %v, want recorded listener failure", err)
	}
}

func TestResilienceIngestShutdownClosesIncompleteRequest(t *testing.T) {
	ingest, err := NewResilienceIngestServer()
	if err != nil {
		t.Fatal(err)
	}
	active := make(chan struct{}, 1)
	ingest.http.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateActive {
			select {
			case active <- struct{}{}:
			default:
			}
		}
	}
	done := make(chan error, 1)
	go func() { done <- ingest.Start() }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := ingest.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	connection, err := net.Dial("tcp", strings.TrimPrefix(ingest.URL(), "http://"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = fmt.Fprintf(connection, "POST %s HTTP/1.1\r\nHost: localhost\r\nAuthorization: Bearer %s\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{", resilienceIngestPath, ingest.Token())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-active:
	case <-time.After(time.Second):
		t.Fatal("server did not accept incomplete request")
	}
	stopContext, cancelStop := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelStop()
	if err := ingest.Stop(stopContext); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop() error = %v, want deadline exceeded after forced close", err)
	}
	_ = connection.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := connection.Read(make([]byte, 1)); err == nil {
		t.Fatal("incomplete request connection remained open after Stop")
	} else {
		var timeout interface{ Timeout() bool }
		if errors.As(err, &timeout) && timeout.Timeout() {
			t.Fatalf("connection remained open until read deadline: %v", err)
		}
	}
	_ = connection.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("private server did not exit after forced close")
	}
}

func TestResilienceIngestRejectsInvalidAndOversizedBodies(t *testing.T) {
	ingest, err := NewResilienceIngestServer()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- ingest.Start() }()
	defer func() { _ = ingest.Stop(context.Background()); <-done }()
	for _, tt := range []struct {
		name, body string
		status     int
	}{
		{"oversized", strings.Repeat("x", 65537), http.StatusRequestEntityTooLarge},
		{"schema", `{"schema_version":"v2","worker_id":"w","sequence":1}`, http.StatusBadRequest},
		{"name", `{"schema_version":"v1","worker_id":"w","sequence":1,"policies":[{"name":"bad name","features":[]}]}`, http.StatusBadRequest},
		{"state", `{"schema_version":"v1","worker_id":"w","sequence":1,"policies":[{"name":"payments","features":[]}],"circuits":[{"name":"payments","state":"broken"}]}`, http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request, _ := http.NewRequest(http.MethodPost, ingest.URL()+"/internal/v1/resilience/snapshot", strings.NewReader(tt.body))
			request.Header.Set("Authorization", "Bearer "+ingest.Token())
			request.Header.Set("Content-Type", "application/json")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != tt.status {
				t.Fatalf("status = %d, want %d", response.StatusCode, tt.status)
			}
		})
	}
	if got := ingest.Store().Snapshot(time.Now(), true); got.Status != "unavailable" {
		t.Fatalf("invalid report stored: %#v", got)
	}
}
