package toolchain

import (
	"fmt"
	"strings"
)

const officialCatalogReleaseBaseURL = "https://github.com/tusk-framework/tusk-engine/releases/download"

// NormalizeReleaseVersion validates a stable semantic version as injected by
// GoReleaser. Release versions intentionally omit the Git tag's leading "v".
func NormalizeReleaseVersion(version string) (string, error) {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid stable Engine release version %q", version)
	}
	for _, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return "", fmt.Errorf("invalid stable Engine release version %q", version)
		}
		for _, char := range part {
			if char < '0' || char > '9' {
				return "", fmt.Errorf("invalid stable Engine release version %q", version)
			}
		}
	}
	return version, nil
}

// ReleaseCatalogURL returns the official catalog asset URL for one Engine
// release. It never resolves a moving latest-release alias.
func ReleaseCatalogURL(version string) (string, error) {
	normalized, err := NormalizeReleaseVersion(version)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/v%s/toolchain-catalog.json", officialCatalogReleaseBaseURL, normalized), nil
}
