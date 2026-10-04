package packaging

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDockerImageRequiresProjectBootstrapAndGeneratedWorker(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dockerfilePath := filepath.Join(filepath.Dir(sourceFile), "..", "..", "Dockerfile")
	contents, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}

	dockerfile := string(contents)
	for _, line := range strings.Split(dockerfile, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "COPY worker.php ") {
			t.Error("Dockerfile must not install the Engine repository's legacy worker.php in the project")
		}
	}
	for _, requirement := range []string{"bootstrap/app.php", ".tusk/runtime/worker.php", "tusk start"} {
		if !strings.Contains(dockerfile, requirement) {
			t.Errorf("Dockerfile must document %s for the mounted modern project", requirement)
		}
	}
}
