package toolchain

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallerPublishesRawExecutable(t *testing.T) {
	installer := Installer{Root: t.TempDir(), GOOS: "windows", GOARCH: "amd64"}
	artifact := installerArtifact("raw", "rr.exe")

	path, err := installer.Install(artifact, []byte("binary"))
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join("roadrunner", "2025.1.0", "windows-amd64", "rr.exe")) {
		t.Fatalf("Install() path = %q, want versioned target", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "binary" {
		t.Fatalf("installed data = %q, want binary", data)
	}
}

func TestInstallerPublishesZipAndTarGzExecutables(t *testing.T) {
	installer := Installer{Root: t.TempDir(), GOOS: "linux", GOARCH: "amd64"}

	zipData := makeZip(t, "bin/rr", []byte("zip-binary"))
	zipPath, err := installer.Install(installerArtifact("zip", "bin/rr"), zipData)
	if err != nil {
		t.Fatalf("zip Install() error = %v", err)
	}
	assertFileContent(t, zipPath, "zip-binary")

	tarData := makeTarGz(t, "bin/php", []byte("tar-binary"))
	tarPath, err := installer.Install(Artifact{
		Tool:       PHP,
		Version:    "8.3.0",
		GOOS:       "linux",
		GOARCH:     "amd64",
		Format:     "tar.gz",
		EntryPoint: "bin/php",
	}, tarData)
	if err != nil {
		t.Fatalf("tar.gz Install() error = %v", err)
	}
	assertFileContent(t, tarPath, "tar-binary")
}

func TestInstallerRejectsUnsafeEntrypointsAndMembers(t *testing.T) {
	installer := Installer{Root: t.TempDir(), GOOS: "windows", GOARCH: "amd64"}
	for _, entrypoint := range []string{"../rr.exe", `C:\rr.exe`, "/rr.exe"} {
		_, err := installer.Install(installerArtifact("raw", entrypoint), []byte("binary"))
		if err == nil || !strings.Contains(err.Error(), "entrypoint") {
			t.Fatalf("entrypoint %q error = %v, want validation error", entrypoint, err)
		}
	}

	archive := makeZip(t, "../escape.exe", []byte("escape"))
	_, err := installer.Install(installerArtifact("zip", "escape.exe"), archive)
	if err == nil || !strings.Contains(err.Error(), "path") {
		t.Fatalf("archive traversal error = %v, want path validation error", err)
	}
	if _, err := os.Stat(filepath.Join(installer.Root, "escape.exe")); !os.IsNotExist(err) {
		t.Fatalf("archive traversal escaped installation root: %v", err)
	}
}

func TestInstallerRejectsMissingEntrypointUnsupportedFormatAndExistingTarget(t *testing.T) {
	installer := Installer{Root: t.TempDir(), GOOS: "linux", GOARCH: "amd64"}

	_, err := installer.Install(installerArtifact("zip", "rr"), makeZip(t, "other", []byte("binary")))
	if err == nil || !strings.Contains(err.Error(), "entrypoint") {
		t.Fatalf("missing entrypoint error = %v", err)
	}
	_, err = installer.Install(installerArtifact("msi", "rr.exe"), []byte("binary"))
	if err == nil || !strings.Contains(err.Error(), "format") {
		t.Fatalf("unsupported format error = %v", err)
	}

	artifact := installerArtifact("raw", "rr")
	if _, err := installer.Install(artifact, []byte("first")); err != nil {
		t.Fatal(err)
	}
	_, err = installer.Install(artifact, []byte("second"))
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing target error = %v", err)
	}
}

func TestInstallerRejectsPathTraversalInArtifactVersion(t *testing.T) {
	installer := Installer{Root: t.TempDir(), GOOS: "linux", GOARCH: "amd64"}
	artifact := installerArtifact("raw", "rr")
	artifact.Version = "../escape"

	_, err := installer.Install(artifact, []byte("binary"))
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("version traversal error = %v, want version validation error", err)
	}
	if _, err := os.Stat(filepath.Join(installer.Root, "escape")); !os.IsNotExist(err) {
		t.Fatalf("version traversal escaped installation root: %v", err)
	}
}

func TestInstallerRejectsNormalizedArchivePathTraversal(t *testing.T) {
	installer := Installer{Root: t.TempDir(), GOOS: "linux", GOARCH: "amd64"}
	for _, member := range []string{"bin/../escape", "./bin/../../escape"} {
		_, err := installer.Install(installerArtifact("zip", "escape"), makeZip(t, member, []byte("escape")))
		if err == nil || !strings.Contains(err.Error(), "path") {
			t.Fatalf("archive member %q error = %v, want path rejection", member, err)
		}
	}
}

func installerArtifact(format, entrypoint string) Artifact {
	return Artifact{
		Tool:       RoadRunner,
		Version:    "2025.1.0",
		Format:     format,
		EntryPoint: entrypoint,
	}
}

func makeZip(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	file, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func makeTarGz(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, data, want)
	}
}
