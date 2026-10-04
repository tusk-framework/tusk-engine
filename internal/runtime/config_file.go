package runtime

import (
	"fmt"
	"os"
	"path/filepath"
)

// ConfigFile owns one Engine-generated RoadRunner configuration file.
type ConfigFile struct {
	Path      string
	generated bool
}

// NewConfigFile writes projected configuration below the project-local
// runtime directory without touching a user-authored .rr.yaml.
func NewConfigFile(root string, contents []byte) (*ConfigFile, error) {
	directory := filepath.Join(root, ".tusk", "runtime")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create runtime directory: %w", err)
	}
	file, err := os.CreateTemp(directory, "rr-*.yaml")
	if err != nil {
		return nil, fmt.Errorf("create generated RoadRunner config: %w", err)
	}
	path := file.Name()
	defer func() {
		_ = file.Close()
	}()
	if err := file.Chmod(0o600); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("restrict generated RoadRunner config: %w", err)
	}
	if _, err := file.Write(contents); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("write generated RoadRunner config: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("close generated RoadRunner config: %w", err)
	}
	return &ConfigFile{Path: path, generated: true}, nil
}

// Cleanup removes only the generated file owned by this ConfigFile.
func (f *ConfigFile) Cleanup() error {
	if f == nil || !f.generated || f.Path == "" {
		return nil
	}
	if err := os.Remove(f.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove generated RoadRunner config: %w", err)
	}
	f.generated = false
	return nil
}
