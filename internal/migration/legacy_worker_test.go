package migration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func put(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDetectLegacyWorkerWithoutExecutingIt(t *testing.T) {
	root := t.TempDir()
	put(t, root, "worker.php", "<?php syntax error and dangerous side effects")
	result, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != Legacy || !strings.Contains(result.Message, "RoadRunner") || !strings.Contains(result.Action, "tusk migrate") {
		t.Fatalf("detection = %#v", result)
	}
}

func TestDetectModernProject(t *testing.T) {
	root := t.TempDir()
	put(t, root, "bootstrap/app.php", "<?php return null;")
	result, err := Detect(root)
	if err != nil || result.State != Modern {
		t.Fatalf("detection = %#v, %v", result, err)
	}
}

func TestDetectProjectWithNoWorker(t *testing.T) {
	result, err := Detect(t.TempDir())
	if err != nil || result.State != Missing || !strings.Contains(result.Action, "bootstrap/app.php") || strings.Contains(result.Action, "complete") {
		t.Fatalf("detection = %#v, %v", result, err)
	}
}

func TestMigrateRejectsSymlinkedProjectRootBeforeCreatingFiles(t *testing.T) {
	target := t.TempDir()
	put(t, target, "worker.php", "legacy")
	link := filepath.Join(t.TempDir(), "project-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	created, err := Migrate(link)
	if err == nil || !strings.Contains(err.Error(), "ambiguous project root") || len(created) != 0 {
		t.Fatalf("created = %#v, error = %v", created, err)
	}
	if _, err := os.Stat(filepath.Join(target, "bootstrap")); !os.IsNotExist(err) {
		t.Fatalf("migration created files through symlink: %v", err)
	}
}

func TestMigrateRejectsVolumeRootBeforeInspectingProject(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("volume roots are platform-specific")
	}
	root := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)
	created, err := Migrate(root)
	if err == nil || !strings.Contains(err.Error(), "volume root") || len(created) != 0 {
		t.Fatalf("created = %#v, error = %v", created, err)
	}
}

func TestMigrateLegacyWorkerReportsCreatedPathsAndPreservesWorker(t *testing.T) {
	root := t.TempDir()
	put(t, root, "worker.php", "legacy worker")
	created, err := Migrate(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"bootstrap/app.php", "bootstrap/providers.php", "config/app.php", "routes/web.php", "public/index.php"}
	if !reflect.DeepEqual(created, want) {
		t.Fatalf("created = %#v, want %#v", created, want)
	}
	for _, name := range want {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil || len(data) == 0 {
			t.Fatalf("%s: %v", name, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "worker.php"))
	if err != nil || string(data) != "legacy worker" {
		t.Fatalf("worker changed: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".tusk", "runtime", "worker.php")); !os.IsNotExist(err) {
		t.Fatalf("migration created runtime worker: %v", err)
	}
}

func TestMigrateReportsAllConflictsWithoutOverwritingOrPartialWrites(t *testing.T) {
	root := t.TempDir()
	put(t, root, "worker.php", "legacy")
	put(t, root, "bootstrap/app.php", "custom bootstrap")
	put(t, root, "config/app.php", "custom config")
	created, err := Migrate(root)
	if err == nil || !strings.Contains(err.Error(), "bootstrap/app.php") || !strings.Contains(err.Error(), "config/app.php") || !strings.Contains(err.Error(), "review") {
		t.Fatalf("created = %#v, error = %v", created, err)
	}
	if len(created) != 0 {
		t.Fatalf("created on conflict: %#v", created)
	}
	for name, want := range map[string]string{"bootstrap/app.php": "custom bootstrap", "config/app.php": "custom config"} {
		data, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if readErr != nil || string(data) != want {
			t.Fatalf("%s changed: %q, %v", name, data, readErr)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "routes", "web.php")); !os.IsNotExist(err) {
		t.Fatalf("partial migration: %v", err)
	}
}

func TestMigratePartiallyGeneratedProjectUsesEmptyDirectories(t *testing.T) {
	root := t.TempDir()
	put(t, root, "worker.php", "legacy")
	for _, name := range []string{"bootstrap", "config", "routes", "public"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	created, err := Migrate(root)
	if err != nil || len(created) != 5 {
		t.Fatalf("created = %#v, error = %v", created, err)
	}
}

func TestMigrateNoWorkerAndNoBootstrapIsDiagnosticOnly(t *testing.T) {
	root := t.TempDir()
	created, err := Migrate(root)
	if err == nil || !strings.Contains(err.Error(), "bootstrap/app.php") || strings.Contains(err.Error(), "complete") || len(created) != 0 {
		t.Fatalf("created = %#v, error = %v", created, err)
	}
	if _, err := os.Stat(filepath.Join(root, "bootstrap")); !os.IsNotExist(err) {
		t.Fatalf("unexpected bootstrap: %v", err)
	}
}

func TestMigratedPublicIndexBuildsNestedUploadedFiles(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("PHP is unavailable for migrated entrypoint execution")
	}
	root := t.TempDir()
	put(t, root, "worker.php", "legacy")
	if _, err := Migrate(root); err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(root, "upload-result.json")
	put(t, root, "vendor/autoload.php", `<?php
namespace Nyholm\Psr7;
class UploadedFile {
    public function __construct(public string $path, public int $size, public int $error, public ?string $clientFilename, public ?string $clientMediaType) {}
}
class ServerRequest {
    public array $uploadedFiles = [];
    public function __construct(...$args) {}
    public function withParsedBody($body): self { return $this; }
    public function withCookieParams($cookies): self { return $this; }
    public function withUploadedFiles(array $files): self { $this->uploadedFiles = $files; return $this; }
}
`)
	put(t, root, "bootstrap/app.php", `<?php
return new class {
    public function handle($request) {
        $file = $request->uploadedFiles['documents']['identity'] ?? null;
        file_put_contents(getenv('TUSK_UPLOAD_RESULT'), json_encode([
            'uploaded_file' => $file instanceof \Nyholm\Psr7\UploadedFile,
            'filename' => $file?->clientFilename,
        ]));
        return new class {
            public function getStatusCode() { return 200; }
            public function getHeaders() { return []; }
            public function getBody() { return 'ok'; }
        };
    }
};
`)
	put(t, root, "run-upload.php", `<?php
$_SERVER['REQUEST_METHOD'] = 'POST';
$_SERVER['REQUEST_URI'] = '/';
$_POST = [];
$_COOKIE = [];
$_FILES = [
    'documents' => [
        'name' => ['identity' => 'identity.txt'],
        'type' => ['identity' => 'text/plain'],
        'tmp_name' => ['identity' => '/tmp/identity.txt'],
        'error' => ['identity' => UPLOAD_ERR_OK],
        'size' => ['identity' => 12],
    ],
];
require __DIR__.'/public/index.php';
`)
	command := exec.Command(php, filepath.Join(root, "run-upload.php"))
	command.Dir = root
	command.Env = append(os.Environ(), "TUSK_UPLOAD_RESULT="+capture)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("migrated entrypoint failed: %v\n%s", err, output)
	}
	var result struct {
		UploadedFile bool   `json:"uploaded_file"`
		Filename     string `json:"filename"`
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if !result.UploadedFile || result.Filename != "identity.txt" {
		t.Fatalf("nested upload result = %#v", result)
	}
}
