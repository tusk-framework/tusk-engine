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

func TestOfflineDocsPackagingContract(t *testing.T) {
	workflow, err := os.ReadFile("../../.github/workflows/test.yml")
	if err != nil {
		t.Fatalf("read test workflow: %v", err)
	}
	for _, required := range []string{"Verify offline embedded CLI docs", "docs runtime", "HTTPS_PROXY=http://127.0.0.1:9"} {
		if !strings.Contains(string(workflow), required) {
			t.Errorf("test workflow must include offline docs contract %q", required)
		}
	}

	cli, err := os.ReadFile("../../internal/cli/cli.go")
	if err != nil {
		t.Fatalf("read CLI: %v", err)
	}
	for _, required := range []string{"case \"docs\"", "tusk docs [topic]"} {
		if !strings.Contains(string(cli), required) {
			t.Errorf("CLI must expose offline docs contract %q", required)
		}
	}

	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	guides, err := os.ReadFile("../../docs/guides/README.md")
	if err != nil {
		t.Fatalf("read guides index: %v", err)
	}
	if !strings.Contains(string(readme), "tusk docs") || !strings.Contains(string(guides), "../user-guide/index.md") {
		t.Fatal("repository documentation must link to the canonical offline user guide")
	}
}
