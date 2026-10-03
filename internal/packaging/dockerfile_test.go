package packaging

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDockerImagePlacesDefaultWorkerInProjectRoot(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dockerfilePath := filepath.Join(filepath.Dir(sourceFile), "..", "..", "Dockerfile")
	contents, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}

	if !strings.Contains(string(contents), "COPY worker.php /app/worker.php") {
		t.Fatal("Dockerfile does not place worker.php at the engine's default project-root path")
	}
}
