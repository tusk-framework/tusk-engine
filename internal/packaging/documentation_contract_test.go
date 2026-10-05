package packaging

import (
	"os"
	"strings"
	"testing"
)

func TestArchivedProductionHardeningDocumentsPointToCurrentRuntimeContract(t *testing.T) {
	paths := []string{
		"../../docs/superpowers/plans/2026-10-03-production-hardening.md",
		"../../docs/superpowers/specs/2026-10-03-production-hardening-design.md",
	}

	for _, path := range paths {
		document, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		contents := string(document)
		if !strings.Contains(contents, "Status: Obsolete") {
			t.Errorf("%s must be explicitly marked obsolete", path)
		}
		if !strings.Contains(contents, ".tusk/runtime/worker.php") || !strings.Contains(contents, "RoadRunner") {
			t.Errorf("%s must point readers to the current generated-worker RoadRunner contract", path)
		}
		for _, forbidden := range []string{"/app/worker.php", "COPY worker.php", "worker padrão", "NDJSON remains the contract"} {
			if strings.Contains(contents, forbidden) {
				t.Errorf("%s still contains obsolete guidance %q", path, forbidden)
			}
		}
	}
}
