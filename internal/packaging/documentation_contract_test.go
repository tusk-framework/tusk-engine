package packaging

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	if !strings.Contains(string(readme), "docs/user-guide/index.md") || !strings.Contains(string(guides), "../user-guide/index.md") {
		t.Fatal("repository documentation must link to the canonical offline user guide")
	}

	testEmbeddedDocsFromBuiltBinary(t)
}

func testEmbeddedDocsFromBuiltBinary(t *testing.T) {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "tusk")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}

	build := exec.Command("go", "build", "-o", binary, "./cmd/tusk")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI for offline docs smoke test: %v\n%s", err, output)
	}

	environment := []string{"PATH=" + filepath.Dir(binary)}
	if runtime.GOOS == "windows" {
		environment = append(environment, "SystemRoot="+os.Getenv("SystemRoot"))
	}
	for _, proxy := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		environment = append(environment, proxy+"=http://127.0.0.1:9")
	}

	for _, test := range []struct {
		args []string
		want string
	}{
		{args: []string{"docs"}, want: "Tusk Engine documentation (dev)"},
		{args: []string{"docs"}, want: "toolchain.md"},
		{args: []string{"docs", "runtime"}, want: "Application bootstrap and RoadRunner runtime"},
	} {
		command := exec.Command(binary, test.args...)
		command.Env = environment
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("run built CLI %v without ambient dependencies: %v\n%s", test.args, err, output)
		}
		if !strings.Contains(string(output), test.want) {
			t.Errorf("built CLI %v output must contain %q; got:\n%s", test.args, test.want, output)
		}
	}
}
