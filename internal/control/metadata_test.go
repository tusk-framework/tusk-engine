package control

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

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
}
