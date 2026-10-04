package control

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/tusk-framework/tusk-engine/internal/config"
)

func authorized(request *http.Request, cfg config.ControlConfig) bool {
	if isLoopbackAddress(cfg.Address) {
		return true
	}

	const scheme = "Bearer "
	header := request.Header.Get("Authorization")
	if !strings.HasPrefix(header, scheme) {
		return false
	}

	provided := strings.TrimSpace(strings.TrimPrefix(header, scheme))
	expected := strings.TrimSpace(cfg.Token)
	if provided == "" || expected == "" {
		return false
	}

	expectedHash := sha256.Sum256([]byte(expected))
	providedHash := sha256.Sum256([]byte(provided))
	return subtle.ConstantTimeCompare(expectedHash[:], providedHash[:]) == 1
}
