package toolchain

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCatalogAtomicCreatesAndReplacesCatalog(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".tusk", "toolchain.catalog.json")
	first := []byte("first verified catalog")
	if err := WriteCatalogAtomic(root, first); err != nil {
		t.Fatalf("WriteCatalogAtomic() create error = %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(first) {
		t.Fatalf("created catalog = %q, error = %v", got, err)
	}

	second := []byte("replacement verified catalog")
	if err := WriteCatalogAtomic(root, second); err != nil {
		t.Fatalf("WriteCatalogAtomic() replace error = %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(second) {
		t.Fatalf("replaced catalog = %q, error = %v", got, err)
	}
}

func TestWriteCatalogAtomicPreservesExistingBytesOnReplaceFailure(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".tusk", "toolchain.catalog.json")
	original := []byte("last known-good catalog")
	if err := WriteCatalogAtomic(root, original); err != nil {
		t.Fatal(err)
	}

	err := writeCatalogAtomic(root, []byte("new catalog"), func(string, string) error {
		return errors.New("simulated atomic replace failure")
	})
	if err == nil || !strings.Contains(err.Error(), "simulated atomic replace failure") {
		t.Fatalf("writeCatalogAtomic() error = %v, want replace failure", err)
	}
	if got, readErr := os.ReadFile(path); readErr != nil || string(got) != string(original) {
		t.Fatalf("existing catalog = %q, error = %v; want unchanged original", got, readErr)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("catalog directory entries = %#v, want only existing catalog", entries)
	}
}

func TestWriteCatalogAtomicRejectsEmptyDataWithoutReplacingCache(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".tusk", "toolchain.catalog.json")
	original := []byte("last known-good catalog")
	if err := WriteCatalogAtomic(root, original); err != nil {
		t.Fatal(err)
	}
	if err := WriteCatalogAtomic(root, nil); err == nil {
		t.Fatal("WriteCatalogAtomic() accepted empty data")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("existing catalog = %q, error = %v; want unchanged original", got, err)
	}
}
