package control

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tusk-framework/tusk-engine/internal/components"
	"github.com/tusk-framework/tusk-engine/internal/config"
)

func TestMetadataResponseContainsOnlySafeFields(t *testing.T) {
	server, err := NewServer(
		config.ControlConfig{Enabled: true, Address: "127.0.0.1", Port: 9091},
		fakeProvider{snapshot: readySnapshot()},
		Metadata{
			EngineName:     "tusk-engine",
			Version:        "0.1.0",
			GoVersion:      "go1.23.0",
			OS:             "windows",
			Arch:           "amd64",
			WorkerCount:    4,
			TimeoutSeconds: 30,
			Capabilities:   []string{"http", "persistent-workers", "metrics"},
			RemoteAccess:   false,
			Components: []components.Descriptor{{
				Name: "http-invocation", Version: "1.0.0", SchemaVersion: "v1",
				Capabilities: []components.Capability{components.CapabilityServiceInvocation},
				Health:       components.HealthOnStartup,
				Schema: components.Schema{Fields: map[string]components.FieldSchema{
					"token": {Type: components.FieldString, Secret: true},
				}},
				SecretFields: []string{"token"},
			}},
		},
	)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/v1/metadata", nil)
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != 200 {
		t.Fatalf("status = %d, want 200; body = %s", recorder.Code, recorder.Body.String())
	}

	var body map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}

	if body["version"] != "v1" {
		t.Fatalf("version = %v, want v1", body["version"])
	}
	if _, ok := body["token"]; ok {
		t.Fatal("metadata must not expose token")
	}
	if _, ok := body["executable_path"]; ok {
		t.Fatal("metadata must not expose executable path")
	}
	if _, ok := body["environment"]; ok {
		t.Fatal("metadata must not expose environment")
	}
	componentsValue, ok := body["components"].([]interface{})
	if !ok || len(componentsValue) != 1 {
		t.Fatalf("components = %v, want one safe descriptor", body["components"])
	}
	if strings.Contains(recorder.Body.String(), "top-secret") {
		t.Fatal("metadata must not expose component configuration values")
	}
}

func TestResilienceMetadataProjectsFreshStaleAndRemoteRedaction(t *testing.T) {
	ingest, err := NewResilienceIngestServer()
	if err != nil {
		t.Fatal(err)
	}
	defer ingest.Stop(context.Background())
	metadata := Metadata{EngineName: "tusk-engine", Resilience: ingest}
	server, err := NewServer(config.ControlConfig{Enabled: true, Address: "127.0.0.1", Port: 9091}, fakeProvider{snapshot: readySnapshot()}, metadata)
	if err != nil {
		t.Fatal(err)
	}
	read := func() string {
		t.Helper()
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, httptest.NewRequest("GET", "/v1/metadata", nil))
		return recorder.Body.String()
	}
	if body := read(); !strings.Contains(body, `"application":{"resilience":{"schema_version":"v1","status":"unavailable"`) {
		t.Fatalf("unavailable metadata = %s", body)
	}
	if err := ingest.Store().Record(report("private-worker", 1, "payments", "open"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if body := read(); !strings.Contains(body, `"status":"fresh"`) || !strings.Contains(body, `"name":"payments"`) || strings.Contains(body, "private-worker") || strings.Contains(body, ingest.Token()) {
		t.Fatalf("fresh metadata = %s", body)
	}
	if err := ingest.Store().Record(report("private-worker", 2, "payments", "open"), time.Now().Add(-61*time.Second)); err != nil {
		t.Fatal(err)
	}
	if body := read(); !strings.Contains(body, `"status":"stale"`) || !strings.Contains(body, `"unknown":1`) {
		t.Fatalf("stale metadata = %s", body)
	}
	remote, err := NewServer(config.ControlConfig{Enabled: true, Address: "0.0.0.0", Port: 9091, Token: "control-secret"}, fakeProvider{snapshot: readySnapshot()}, metadata)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/v1/metadata", nil)
	request.Header.Set("Authorization", "Bearer control-secret")
	remote.Handler().ServeHTTP(recorder, request)
	if strings.Contains(recorder.Body.String(), "payments") || strings.Contains(recorder.Body.String(), "private-worker") || !strings.Contains(recorder.Body.String(), `"unknown":1`) {
		t.Fatalf("remote metadata = %s", recorder.Body.String())
	}
}

func TestMetadataProjectionSortsAndCopiesComponentDescriptors(t *testing.T) {
	metadata := Metadata{Components: []components.Descriptor{
		{Name: "zeta", Version: "1.0.0", SchemaVersion: "v1", Capabilities: []components.Capability{components.CapabilityState}, Health: components.HealthDisabled, Schema: components.Schema{Fields: map[string]components.FieldSchema{"token": {Type: components.FieldString, Secret: true}}}},
		{Name: "alpha", Version: "1.0.0", SchemaVersion: "v1", Capabilities: []components.Capability{components.CapabilityConfiguration}, Health: components.HealthOnDemand},
	}}
	response := metadata.safeResponse(false)
	if len(response.Components) != 2 || response.Components[0].Name != "alpha" || response.Components[1].Name != "zeta" {
		t.Fatalf("component order = %#v, want alpha then zeta", response.Components)
	}
	metadata.Components[0].Schema.Fields["token"] = components.FieldSchema{Type: components.FieldBoolean}
	if response.Components[1].Schema.Fields["token"].Type != components.FieldString {
		t.Fatal("metadata projection shares schema field map with caller")
	}
}
