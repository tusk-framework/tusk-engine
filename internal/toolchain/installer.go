package toolchain

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

const defaultMaxExtractedBytes int64 = 512 * 1024 * 1024

// Installer stages and atomically publishes one verified tool artifact.
type Installer struct {
	Root              string
	GOOS              string
	GOARCH            string
	MaxExtractedBytes int64
}

// Install extracts or writes an artifact and returns the published executable
// path. The final directory is never overwritten.
func (i Installer) Install(artifact Artifact, data []byte) (string, error) {
	if i.Root == "" {
		return "", errors.New("installer root is required")
	}
	if artifact.Tool != PHP && artifact.Tool != Composer && artifact.Tool != RoadRunner {
		return "", fmt.Errorf("unsupported tool %q", artifact.Tool)
	}
	if strings.TrimSpace(artifact.Version) == "" {
		return "", errors.New("artifact version is required")
	}
	goos := i.GOOS
	if goos == "" {
		goos = artifact.GOOS
	}
	goarch := i.GOARCH
	if goarch == "" {
		goarch = artifact.GOARCH
	}
	if artifact.GOOS != "" && goos != artifact.GOOS {
		return "", fmt.Errorf("artifact platform %s does not match installer %s", artifact.GOOS, goos)
	}
	if artifact.GOARCH != "" && goarch != artifact.GOARCH {
		return "", fmt.Errorf("artifact architecture %s does not match installer %s", artifact.GOARCH, goarch)
	}
	if err := safePathSegment("version", artifact.Version); err != nil {
		return "", err
	}
	if err := safePathSegment("operating system", goos); err != nil {
		return "", err
	}
	if err := safePathSegment("architecture", goarch); err != nil {
		return "", err
	}
	entrypoint, err := safeRelativePath(artifact.EntryPoint)
	if err != nil {
		return "", fmt.Errorf("invalid artifact entrypoint: %w", err)
	}
	if artifact.Format != "raw" && artifact.Format != "zip" && artifact.Format != "tar.gz" {
		return "", fmt.Errorf("unsupported artifact format %q", artifact.Format)
	}

	platform := goos + "-" + goarch
	finalRoot := filepath.Join(i.Root, "toolchain", string(artifact.Tool), artifact.Version, platform)
	if _, err := os.Stat(finalRoot); err == nil {
		return "", fmt.Errorf("installation target already exists: %s", finalRoot)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect installation target: %w", err)
	}
	parent := filepath.Dir(finalRoot)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", fmt.Errorf("create installation parent: %w", err)
	}
	stage, err := os.MkdirTemp(parent, ".staging-")
	if err != nil {
		return "", fmt.Errorf("create installation staging directory: %w", err)
	}
	defer os.RemoveAll(stage)

	maxBytes := i.MaxExtractedBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxExtractedBytes
	}
	if err := i.extract(stage, artifact, entrypoint, data, maxBytes); err != nil {
		return "", err
	}
	entrypointPath := filepath.Join(stage, entrypoint)
	info, err := os.Stat(entrypointPath)
	if err != nil {
		return "", fmt.Errorf("installed entrypoint %q is missing: %w", artifact.EntryPoint, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("installed entrypoint %q is not a regular file", artifact.EntryPoint)
	}
	if err := os.Rename(stage, finalRoot); err != nil {
		return "", fmt.Errorf("publish installation: %w", err)
	}
	return filepath.Join(finalRoot, entrypoint), nil
}

func (i Installer) extract(stage string, artifact Artifact, entrypoint string, data []byte, maxBytes int64) error {
	switch artifact.Format {
	case "raw":
		path := filepath.Join(stage, entrypoint)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create raw entrypoint directory: %w", err)
		}
		if int64(len(data)) > maxBytes {
			return fmt.Errorf("artifact exceeds maximum extracted size of %d bytes", maxBytes)
		}
		if err := os.WriteFile(path, data, 0o755); err != nil {
			return fmt.Errorf("write raw artifact: %w", err)
		}
		return nil
	case "zip":
		return extractZip(stage, data, maxBytes)
	case "tar.gz":
		return extractTarGz(stage, data, maxBytes)
	default:
		return fmt.Errorf("unsupported artifact format %q", artifact.Format)
	}
}

func extractZip(stage string, data []byte, maxBytes int64) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("read ZIP artifact: %w", err)
	}
	var extracted int64
	for _, file := range reader.File {
		rel, err := safeRelativePath(file.Name)
		if err != nil {
			return fmt.Errorf("reject ZIP member %q path: %w", file.Name, err)
		}
		if file.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("reject ZIP symlink member %q", file.Name)
		}
		destination := filepath.Join(stage, rel)
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(destination, 0o755); err != nil {
				return fmt.Errorf("create ZIP directory: %w", err)
			}
			continue
		}
		if file.UncompressedSize64 > uint64(maxBytes-extracted) {
			return fmt.Errorf("ZIP artifact exceeds maximum extracted size of %d bytes", maxBytes)
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return fmt.Errorf("create ZIP entrypoint directory: %w", err)
		}
		input, err := file.Open()
		if err != nil {
			return fmt.Errorf("open ZIP member %q: %w", file.Name, err)
		}
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			_ = input.Close()
			return fmt.Errorf("create ZIP member %q: %w", file.Name, err)
		}
		written, copyErr := io.Copy(output, io.LimitReader(input, maxBytes-extracted+1))
		closeErr := output.Close()
		_ = input.Close()
		if copyErr != nil {
			return fmt.Errorf("extract ZIP member %q: %w", file.Name, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close ZIP member %q: %w", file.Name, closeErr)
		}
		if written > maxBytes-extracted || written != int64(file.UncompressedSize64) {
			return fmt.Errorf("invalid ZIP member %q size", file.Name)
		}
		extracted += written
	}
	return nil
}

func extractTarGz(stage string, data []byte, maxBytes int64) error {
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("read gzip artifact: %w", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	var extracted int64
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read TAR entry: %w", err)
		}
		rel, err := safeRelativePath(header.Name)
		if err != nil {
			return fmt.Errorf("reject TAR member %q path: %w", header.Name, err)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(filepath.Join(stage, rel), 0o755); err != nil {
				return fmt.Errorf("create TAR directory: %w", err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || header.Size > maxBytes-extracted {
				return fmt.Errorf("TAR artifact exceeds maximum extracted size of %d bytes", maxBytes)
			}
			destination := filepath.Join(stage, rel)
			if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
				return fmt.Errorf("create TAR entrypoint directory: %w", err)
			}
			output, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
			if err != nil {
				return fmt.Errorf("create TAR member %q: %w", header.Name, err)
			}
			written, copyErr := io.CopyN(output, tarReader, header.Size)
			closeErr := output.Close()
			if copyErr != nil {
				return fmt.Errorf("extract TAR member %q: %w", header.Name, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close TAR member %q: %w", header.Name, closeErr)
			}
			if written != header.Size {
				return fmt.Errorf("invalid TAR member %q size", header.Name)
			}
			extracted += written
		default:
			return fmt.Errorf("reject TAR special member %q", header.Name)
		}
	}
	return nil
}

func safeRelativePath(value string) (string, error) {
	value = strings.ReplaceAll(value, "\\", "/")
	if value == "" || strings.HasPrefix(value, "/") || hasWindowsVolumePrefix(value) || filepath.VolumeName(value) != "" {
		return "", errors.New("path must be relative")
	}
	clean := pathpkg.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("path traversal is not allowed")
	}
	return filepath.FromSlash(clean), nil
}

func hasWindowsVolumePrefix(value string) bool {
	return len(value) >= 2 && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) && value[1] == ':'
}

func safePathSegment(label, value string) error {
	if strings.TrimSpace(value) == "" || value == "." || value == ".." || strings.ContainsAny(value, `/\\:`) {
		return fmt.Errorf("artifact %s must be a single safe path segment", label)
	}
	return nil
}
