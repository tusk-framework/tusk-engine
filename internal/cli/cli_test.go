package cli

import (
	"strings"
	"testing"

	"github.com/tusk-framework/tusk-engine/internal/config"
)

func TestRunServerWithConfigRejectsInvalidRuntimeBeforeResolution(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ProjectRoot = t.TempDir()
	cfg.Runtime.StatusAddress = "0.0.0.0:2114"

	err := runServerWithConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "invalid runtime configuration") {
		t.Fatalf("runServerWithConfig() error = %v, want validation failure", err)
	}
}

func TestRunServerWithConfigDoesNotFallbackWhenRoadRunnerIsMissing(t *testing.T) {
	root := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.ProjectRoot = root
	t.Setenv("PATH", root)

	err := runServerWithConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "resolve RoadRunner") {
		t.Fatalf("runServerWithConfig() error = %v, want RoadRunner resolution failure", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "native") || strings.Contains(strings.ToLower(err.Error()), "worker pool") {
		t.Fatalf("missing RoadRunner must not fall back to native runtime: %v", err)
	}
}
