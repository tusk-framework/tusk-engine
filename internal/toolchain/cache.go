package toolchain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ArtifactCache stores verified artifact bytes by their SHA-256 digest.
type ArtifactCache struct {
	Root string
}

// Put verifies data and publishes it atomically under the expected digest.
func (c ArtifactCache) Put(expectedSHA256 string, data []byte) (string, error) {
	digest, err := normalizeDigest(expectedSHA256)
	if err != nil {
		return "", err
	}
	actual := sha256.Sum256(data)
	if hex.EncodeToString(actual[:]) != digest {
		return "", fmt.Errorf("artifact digest mismatch: expected %s", digest)
	}

	directory := filepath.Join(c.Root, "artifacts")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", fmt.Errorf("create artifact cache: %w", err)
	}
	destination := filepath.Join(directory, digest)
	if existing, err := os.ReadFile(destination); err == nil {
		if existingDigest := sha256.Sum256(existing); hex.EncodeToString(existingDigest[:]) == digest {
			return destination, nil
		}
		return "", fmt.Errorf("cached artifact digest mismatch: %s", destination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect cached artifact: %w", err)
	}

	temporary, err := os.CreateTemp(directory, ".artifact-*")
	if err != nil {
		return "", fmt.Errorf("create temporary artifact: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("secure temporary artifact: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("write temporary artifact: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("sync temporary artifact: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close temporary artifact: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return "", fmt.Errorf("publish cached artifact: %w", err)
	}
	return destination, nil
}

// Get reads and revalidates an artifact from the cache.
func (c ArtifactCache) Get(expectedSHA256 string) ([]byte, error) {
	digest, err := normalizeDigest(expectedSHA256)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(c.Root, "artifacts", digest))
	if err != nil {
		return nil, err
	}
	actual := sha256.Sum256(data)
	if hex.EncodeToString(actual[:]) != digest {
		return nil, fmt.Errorf("cached artifact digest mismatch: expected %s", digest)
	}
	return data, nil
}

func normalizeDigest(value string) (string, error) {
	digest := strings.ToLower(strings.TrimSpace(value))
	if len(digest) != sha256.Size*2 {
		return "", errors.New("artifact digest must be 64 hexadecimal characters")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return "", errors.New("artifact digest must be hexadecimal")
	}
	return digest, nil
}
