package server

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/tusk-framework/tusk-engine/internal/config"
)

type fakeWorker struct {
	calls int
}

func (f *fakeWorker) HandleRequest(map[string]interface{}) (map[string]interface{}, error) {
	f.calls++
	return map[string]interface{}{
		"status": 200,
		"headers": map[string]interface{}{
			"Content-Type": []interface{}{"text/plain"},
		},
		"body": "ok",
	}, nil
}

func TestHandleRequestRejectsStaticPathTraversal(t *testing.T) {
	root := t.TempDir()
	publicDir := filepath.Join(root, "public")
	if err := os.Mkdir(publicDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.ProjectRoot = root
	cfg.PublicDir = "public"
	worker := &fakeWorker{}
	srv := NewServer(cfg, worker)
	req := httptest.NewRequest("GET", "/../secret.txt", nil)
	recorder := httptest.NewRecorder()

	srv.handleRequest(recorder, req)

	if recorder.Code != 404 {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
	if recorder.Body.String() == "secret" {
		t.Fatal("static handler served a file outside public directory")
	}
	if worker.calls != 0 {
		t.Fatalf("worker calls = %d, want 0", worker.calls)
	}
}

func TestHandleRequestRejectsStaticSymlinkOutsidePublic(t *testing.T) {
	root := t.TempDir()
	publicDir := filepath.Join(root, "public")
	if err := os.Mkdir(publicDir, 0700); err != nil {
		t.Fatal(err)
	}
	secretPath := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(secretPath, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secretPath, filepath.Join(publicDir, "link.txt")); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.ProjectRoot = root
	cfg.PublicDir = "public"
	worker := &fakeWorker{}
	srv := NewServer(cfg, worker)
	req := httptest.NewRequest("GET", "/link.txt", nil)
	recorder := httptest.NewRecorder()

	srv.handleRequest(recorder, req)

	if recorder.Code != 404 {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
	if worker.calls != 0 {
		t.Fatalf("worker calls = %d, want 0", worker.calls)
	}
}

func TestHandleRequestRejectsBodyAboveLimit(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ProjectRoot = t.TempDir()
	cfg.MaxBodyBytes = 4
	worker := &fakeWorker{}
	srv := NewServer(cfg, worker)
	req := httptest.NewRequest("POST", "/submit", bytes.NewBufferString("12345"))
	recorder := httptest.NewRecorder()

	srv.handleRequest(recorder, req)

	if recorder.Code != 413 {
		t.Fatalf("status = %d, want 413", recorder.Code)
	}
	if worker.calls != 0 {
		t.Fatalf("worker calls = %d, want 0", worker.calls)
	}
}

func TestHandleRequestRejectsOversizedMultipartFile(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ProjectRoot = t.TempDir()
	cfg.MaxUploadBytes = 4
	worker := &fakeWorker{}
	srv := NewServer(cfg, worker)

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("document", "large.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("12345"))
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/upload", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	recorder := httptest.NewRecorder()

	srv.handleRequest(recorder, req)

	if recorder.Code != 413 {
		t.Fatalf("status = %d, want 413", recorder.Code)
	}
	if worker.calls != 0 {
		t.Fatalf("worker calls = %d, want 0", worker.calls)
	}
}

func TestHandleRequestRejectsTooManyMultipartFiles(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ProjectRoot = t.TempDir()
	cfg.MaxUploadFiles = 1
	worker := &fakeWorker{}
	srv := NewServer(cfg, worker)

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for _, name := range []string{"one.txt", "two.txt"} {
		part, err := form.CreateFormFile("documents", name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write([]byte("ok"))
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/upload", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	recorder := httptest.NewRecorder()

	srv.handleRequest(recorder, req)

	if recorder.Code != 413 {
		t.Fatalf("status = %d, want 413", recorder.Code)
	}
	if worker.calls != 0 {
		t.Fatalf("worker calls = %d, want 0", worker.calls)
	}
}

func TestHandleRequestForwardsRequestWithinLimits(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ProjectRoot = t.TempDir()
	cfg.MaxBodyBytes = 16
	worker := &fakeWorker{}
	srv := NewServer(cfg, worker)
	req := httptest.NewRequest("POST", "/submit", bytes.NewBufferString("1234"))
	recorder := httptest.NewRecorder()

	srv.handleRequest(recorder, req)

	if recorder.Code != 200 {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if worker.calls != 1 {
		t.Fatalf("worker calls = %d, want 1", worker.calls)
	}
}
