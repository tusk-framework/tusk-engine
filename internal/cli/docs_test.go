package cli

import (
	"errors"
	"io"
	"strings"
	"testing"

	"testing/fstest"
)

func testDocumentationFS() fstest.MapFS {
	return fstest.MapFS{
		"user-guide/index.md":   {Data: []byte("# Topics\n- runtime\n")},
		"user-guide/runtime.md": {Data: []byte("# Runtime guide\nRoadRunner owns workers.\n")},
	}
}

func TestRunDocsPrintsVersionedIndex(t *testing.T) {
	var output strings.Builder
	if err := runDocs(nil, &output, testDocumentationFS(), "1.2.3"); err != nil {
		t.Fatalf("runDocs returned error: %v", err)
	}
	if got := output.String(); !strings.Contains(got, "Tusk Engine documentation (1.2.3)") || !strings.Contains(got, "# Topics") {
		t.Fatalf("output = %q, want version and index", got)
	}
}

func TestRunDocsPrintsRequestedTopic(t *testing.T) {
	var output strings.Builder
	if err := runDocs([]string{"runtime"}, &output, testDocumentationFS(), "dev"); err != nil {
		t.Fatalf("runDocs returned error: %v", err)
	}
	if got := output.String(); !strings.Contains(got, "# Runtime guide") || !strings.Contains(got, "RoadRunner owns workers") {
		t.Fatalf("output = %q, want runtime guide", got)
	}
}

func TestRunDocsRejectsUnknownAndTraversalTopics(t *testing.T) {
	for _, topic := range []string{"missing", "../secret", "runtime/../../secret", "C:\\secret"} {
		t.Run(topic, func(t *testing.T) {
			var output strings.Builder
			err := runDocs([]string{topic}, &output, testDocumentationFS(), "dev")
			if err == nil {
				t.Fatalf("runDocs(%q) succeeded", topic)
			}
			if !strings.Contains(err.Error(), "runtime") {
				t.Fatalf("error %q does not list valid topics", err)
			}
		})
	}
}

func TestRunDocsRejectsTooManyArguments(t *testing.T) {
	err := runDocs([]string{"runtime", "extra"}, io.Discard, testDocumentationFS(), "dev")
	if err == nil {
		t.Fatal("runDocs accepted more than one topic")
	}
}

func TestRunDocsRejectsEmptyGuide(t *testing.T) {
	files := fstest.MapFS{"user-guide/empty.md": {Data: []byte(" \n")}}
	err := runDocs([]string{"empty"}, io.Discard, files, "dev")
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("runDocs error = %v, want empty-guide error", err)
	}
}

type failedDocumentationWriter struct{}

func (failedDocumentationWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestRunDocsReturnsWriterErrors(t *testing.T) {
	err := runDocs([]string{"runtime"}, failedDocumentationWriter{}, testDocumentationFS(), "dev")
	if err == nil || !strings.Contains(err.Error(), "broken pipe") {
		t.Fatalf("runDocs error = %v, want writer error", err)
	}
}
