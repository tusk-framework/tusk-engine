package control

import (
	"net/http/httptest"
	"testing"

	"github.com/tusk-framework/tusk-engine/internal/config"
)

func TestRemoteControlRequiresExactBearerToken(t *testing.T) {
	cfg := config.ControlConfig{
		Enabled: true,
		Address: "0.0.0.0",
		Port:    9091,
		Token:   "secret-token",
	}

	tests := []struct {
		name          string
		authorization string
		want          bool
	}{
		{name: "missing", want: false},
		{name: "wrong scheme", authorization: "Basic secret-token", want: false},
		{name: "wrong token", authorization: "Bearer wrong", want: false},
		{name: "empty token", authorization: "Bearer ", want: false},
		{name: "valid", authorization: "Bearer secret-token", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "http://example.test/v1/healthz", nil)
			request.Header.Set("Authorization", tt.authorization)

			if got := authorized(request, cfg); got != tt.want {
				t.Fatalf("authorized() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoopbackControlDoesNotRequireToken(t *testing.T) {
	request := httptest.NewRequest("GET", "http://localhost/v1/healthz", nil)
	cfg := config.ControlConfig{Enabled: true, Address: "127.0.0.1", Port: 9091}

	if !authorized(request, cfg) {
		t.Fatal("loopback control should not require a token")
	}
}
