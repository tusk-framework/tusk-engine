package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/tusk-framework/tusk-engine/internal/components"
	"github.com/tusk-framework/tusk-engine/internal/config"
)

func TestNewComponentRegistryActivatesFirstPartyProviders(t *testing.T) {
	registry, err := newComponentRegistry()
	if err != nil {
		t.Fatalf("newComponentRegistry() error = %v", err)
	}
	if err := registry.Activate(context.Background(), nil); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	for _, check := range []struct {
		name       string
		capability components.Capability
	}{
		{name: components.BuiltInInvocationComponentName, capability: components.CapabilityServiceInvocation},
		{name: components.BuiltInResilienceComponentName, capability: components.CapabilityResilience},
	} {
		if _, err := registry.Resolve(check.name, check.capability); err != nil {
			t.Fatalf("Resolve(%q, %q) error = %v", check.name, check.capability, err)
		}
	}
}

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
