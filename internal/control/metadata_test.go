package control

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

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
