package packaging

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestFrameworkSmokeContractUsesPublishedImmutableRef(t *testing.T) {
	workflow, err := os.ReadFile("../../.github/workflows/test.yml")
	if err != nil {
		t.Fatalf("read smoke workflow: %v", err)
	}
	contents := string(workflow)

	if !strings.Contains(contents, "FRAMEWORK_REF: main") {
		t.Fatalf("workflow must use the published Framework ref")
	}
	if !regexp.MustCompile(`(?m)^\s+FRAMEWORK_SHA: [0-9a-f]{40}\r?$`).MatchString(contents) {
		t.Fatalf("workflow must pin the Framework branch to a full commit SHA")
	}
	if !strings.Contains(contents, `git fetch --depth 1 origin "$FRAMEWORK_REF"`) {
		t.Fatalf("workflow must fetch the coordinated branch instead of relying on a local-only object")
	}
	if !strings.Contains(contents, `git rev-parse FETCH_HEAD`) || !strings.Contains(contents, `"$FRAMEWORK_SHA"`) {
		t.Fatalf("workflow must verify that the published branch resolves to the pinned Framework SHA")
	}
	if !strings.Contains(contents, "coordinated modern Framework ref") {
		t.Fatalf("workflow must fail with actionable publication guidance when the ref is unavailable or moved")
	}
}
