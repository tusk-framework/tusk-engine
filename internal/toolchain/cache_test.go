package toolchain

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtifactCachePutAndGet(t *testing.T) {
	cache := ArtifactCache{Root: t.TempDir()}
	data := []byte("verified artifact")
	digest := digestFor(data)

	path, err := cache.Put(digest, data)
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if path != filepath.Join(cache.Root, "artifacts", digest) {
		t.Fatalf("Put() path = %q, want content-addressed path", path)
	}

	got, err := cache.Get(digest)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("Get() = %q, want %q", got, data)
	}
}

func TestArtifactCacheRejectsMissingAndInvalidEntries(t *testing.T) {
	cache := ArtifactCache{Root: t.TempDir()}
	_, err := cache.Get(strings.Repeat("a", 64))
	if err == nil || !os.IsNotExist(err) {
		t.Fatalf("missing Get() error = %v, want not-exist error", err)
	}

	data := []byte("verified artifact")
	digest := digestFor(data)
	path, err := cache.Put(digest, data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = cache.Get(digest)
	if err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("tampered Get() error = %v, want digest error", err)
	}
}

func TestArtifactCacheRejectsDigestMismatchBeforePublication(t *testing.T) {
	cache := ArtifactCache{Root: t.TempDir()}
	data := []byte("artifact")
	wrongDigest := strings.Repeat("b", 64)
	_, err := cache.Put(wrongDigest, data)
	if err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("mismatch Put() error = %v, want digest error", err)
	}
	if _, err := os.Stat(filepath.Join(cache.Root, "artifacts", wrongDigest)); !os.IsNotExist(err) {
		t.Fatalf("mismatched artifact was published: stat error = %v", err)
	}
}

func digestFor(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
