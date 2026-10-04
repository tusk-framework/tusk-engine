package control

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/tusk-framework/tusk-engine/internal/config"
)

type fakeProvider struct {
	snapshot RuntimeSnapshot
}

func (f fakeProvider) Snapshot() RuntimeSnapshot {
	return f.snapshot
}

func readySnapshot() RuntimeSnapshot {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

	return RuntimeSnapshot{
		EngineState:       EngineRunning,
		ReadinessReason:   ReadinessReady,
		DesiredWorkers:    2,
		ReadyWorkers:      2,
		ActiveWorkers:     0,
		TotalWorkers:      2,
		WorkerCountsKnown: true,
		StartedAt:         now,
		StateChangedAt:    now,
	}
}

func TestHealthAndReadinessUseStableStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		snapshot   RuntimeSnapshot
		path       string
		wantCode   int
		wantStatus string
		wantReason string
	}{
		{
			name:       "starting health is alive",
			snapshot:   RuntimeSnapshot{EngineState: EngineStarting, ReadinessReason: ReadinessStarting},
			path:       "/v1/healthz",
			wantCode:   200,
			wantStatus: "ok",
		},
		{
			name:       "stopping health is alive",
			snapshot:   RuntimeSnapshot{EngineState: EngineStopping, ReadinessReason: ReadinessStopping},
			path:       "/v1/healthz",
			wantCode:   200,
			wantStatus: "ok",
		},
		{
			name:       "ready readiness succeeds",
			snapshot:   readySnapshot(),
			path:       "/v1/readyz",
			wantCode:   200,
			wantStatus: "ready",
		},
		{
			name:       "no workers is not ready",
			snapshot:   RuntimeSnapshot{EngineState: EngineRunning, ReadinessReason: ReadinessNoWorkers},
			path:       "/v1/readyz",
			wantCode:   503,
			wantStatus: "not_ready",
			wantReason: "no_workers",
		},
		{
			name:       "failed process is unavailable",
			snapshot:   RuntimeSnapshot{EngineState: EngineFailed, ReadinessReason: ReadinessProcessFailed},
			path:       "/v1/readyz",
			wantCode:   503,
			wantStatus: "not_ready",
			wantReason: "process_failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(
				config.ControlConfig{Enabled: true, Address: "127.0.0.1", Port: 9091},
				fakeProvider{snapshot: tt.snapshot},
				Metadata{EngineName: "tusk-engine", Version: "0.1.0"},
			)
			if err != nil {
				t.Fatalf("NewServer() error = %v", err)
			}

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest("GET", tt.path, nil)
			server.Handler().ServeHTTP(recorder, request)

			if recorder.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, tt.wantCode, recorder.Body.String())
			}

			var body map[string]interface{}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("response is not JSON: %v", err)
			}
			if body["version"] != "v1" || body["status"] != tt.wantStatus {
				t.Fatalf("response = %v", body)
			}
			if tt.wantReason != "" && body["error"] != tt.wantStatus {
				t.Fatalf("error = %v, want %s", body["error"], tt.wantStatus)
			}
			if tt.wantReason != "" && body["reason"] != tt.wantReason {
				t.Fatalf("reason = %v, want %s", body["reason"], tt.wantReason)
			}
			if tt.path == "/v1/readyz" {
				workers, ok := body["workers"].(map[string]interface{})
				if !ok {
					t.Fatalf("workers response = %v", body["workers"])
				}
				if _, ok := workers["known"]; !ok {
					t.Fatalf("workers response lacks knowledge flag: %v", workers)
				}
			}
		})
	}
}

func TestRemoteControlProtectsAllEndpoints(t *testing.T) {
	server, err := NewServer(
		config.ControlConfig{Enabled: true, Address: "0.0.0.0", Port: 9091, Token: "secret"},
		fakeProvider{snapshot: readySnapshot()},
		Metadata{EngineName: "tusk-engine", Version: "0.1.0"},
	)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	for _, path := range []string{"/v1/healthz", "/v1/readyz", "/v1/metadata", "/v1/metrics"} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest("GET", path, nil)
			server.Handler().ServeHTTP(recorder, request)

			if recorder.Code != 401 {
				t.Fatalf("unauthenticated status = %d, want 401", recorder.Code)
			}
			if recorder.Body.String() == "secret" {
				t.Fatal("response must not echo the token")
			}
		})
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/v1/metadata", nil)
	request.Header.Set("Authorization", "Bearer secret")
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != 200 {
		t.Fatalf("authenticated status = %d, want 200", recorder.Code)
	}
}

func TestDisabledControlDoesNotExposeRoutes(t *testing.T) {
	server, err := NewServer(
		config.ControlConfig{Enabled: false},
		fakeProvider{snapshot: readySnapshot()},
		Metadata{EngineName: "tusk-engine", Version: "0.1.0"},
	)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest("GET", "/v1/healthz", nil))
	if recorder.Code != 404 {
		t.Fatalf("disabled status = %d, want 404", recorder.Code)
	}
}

func TestServerBindsBeforeReportingReadyAndStops(t *testing.T) {
	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve control port: %v", err)
	}
	port := reserved.Addr().(*net.TCPAddr).Port
	if err := reserved.Close(); err != nil {
		t.Fatalf("release control port: %v", err)
	}

	server, err := NewServer(
		config.ControlConfig{Enabled: true, Address: "127.0.0.1", Port: port},
		fakeProvider{snapshot: readySnapshot()},
		Metadata{EngineName: "tusk-engine", Version: "0.1.0"},
	)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Start()
	}()

	readyContext, cancelReady := context.WithTimeout(context.Background(), time.Second)
	if err := server.WaitReady(readyContext); err != nil {
		cancelReady()
		t.Fatalf("WaitReady() error = %v", err)
	}
	cancelReady()

	response, err := http.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/v1/readyz")
	if err != nil {
		t.Fatalf("GET /v1/readyz: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("ready status = %d, want 200", response.StatusCode)
	}

	stopContext, cancelStop := context.WithTimeout(context.Background(), time.Second)
	if err := server.Stop(stopContext); err != nil {
		cancelStop()
		t.Fatalf("Stop() error = %v", err)
	}
	cancelStop()

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop")
	}
}
