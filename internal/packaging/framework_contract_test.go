package packaging

import (
	"os"
	"strings"
	"testing"
)

func TestFrameworkSmokeContractResolvesPublishedRefPerRun(t *testing.T) {
	workflow, err := os.ReadFile("../../.github/workflows/test.yml")
	if err != nil {
		t.Fatalf("read smoke workflow: %v", err)
	}
	contents := string(workflow)

	if !strings.Contains(contents, "FRAMEWORK_REF: main") {
		t.Fatalf("workflow must use the published Framework ref")
	}
	if strings.Contains(contents, "FRAMEWORK_SHA:") {
		t.Fatalf("workflow must not require a manually maintained Framework SHA")
	}
	if !strings.Contains(contents, `git fetch --depth 1 origin "$FRAMEWORK_REF"`) {
		t.Fatalf("workflow must fetch the coordinated branch instead of relying on a local-only object")
	}
	if !strings.Contains(contents, `resolved_framework_sha=$(git rev-parse FETCH_HEAD)`) {
		t.Fatalf("workflow must resolve the published Framework ref to the SHA tested in the run")
	}
	if !strings.Contains(contents, `git checkout --detach "$resolved_framework_sha"`) {
		t.Fatalf("workflow must test the resolved Framework commit in detached mode")
	}
	if !strings.Contains(contents, `echo "Using Framework $FRAMEWORK_REF at $resolved_framework_sha"`) {
		t.Fatalf("workflow must report the exact Framework commit used by the smoke test")
	}
}
