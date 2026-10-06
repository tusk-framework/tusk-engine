package packaging

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func engineRoot(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller failed")
	}
	return filepath.Join(filepath.Dir(sourceFile), "..", "..")
}

func TestLegacyEngineArtifactsAreAbsent(t *testing.T) {
	root := engineRoot(t)
	for _, relative := range []string{
		"worker.php",
		"internal/ipc",
		"internal/migration",
		"internal/server",
		"internal/worker",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); !os.IsNotExist(err) {
			t.Errorf("legacy Engine artifact %s still exists: %v", relative, err)
		}
	}
}

func TestCliDoesNotAdvertiseLegacyMigration(t *testing.T) {
	root := engineRoot(t)
	for _, relative := range []string{"internal/cli/cli.go", "internal/cli/doctor.go"} {
		contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"tusk migrate", "internal/migration", "rejectLegacyWorker", "runMigrateTo"} {
			if strings.Contains(string(contents), forbidden) {
				t.Errorf("%s still contains legacy migration reference %q", relative, forbidden)
			}
		}
	}
}

func TestActiveDocumentationDoesNotAdvertiseLegacyRuntime(t *testing.T) {
	root := engineRoot(t)
	for _, relative := range []string{
		"README.md",
		"docs/guides/project-runtime.md",
		"docs/guides/framework-smoke-contract.md",
	} {
		contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"tusk migrate",
			"root worker.php",
			"echo-worker",
			"native server",
			"legacy native",
			"legacy NDJSON",
		} {
			if strings.Contains(strings.ToLower(string(contents)), strings.ToLower(forbidden)) {
				t.Errorf("%s still contains legacy runtime guidance %q", relative, forbidden)
			}
		}
	}
}
