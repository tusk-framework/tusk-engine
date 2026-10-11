package toolchain

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const officialCatalogReleaseBaseURL = "https://github.com/tusk-framework/tusk-engine/releases/download"

const (
	defaultCatalogFetchTimeout = 30 * time.Second
	defaultCatalogMaxBytes     = 1 << 20
)

var allowedCatalogRedirectHosts = map[string]struct{}{
	"github.com":                           {},
	"release-assets.githubusercontent.com": {},
	"objects.githubusercontent.com":        {},
}

// CatalogFetcher retrieves catalog bytes for one Engine release. Callers must
// authenticate the returned bytes before storing or consuming them.
type CatalogFetcher interface {
	Fetch(ctx context.Context, releaseVersion string) ([]byte, error)
}

// HTTPReleaseCatalogFetcher downloads catalog release assets with bounded
// time and response size. Client is injectable for deterministic tests.
type HTTPReleaseCatalogFetcher struct {
	Client   *http.Client
	MaxBytes int64
}

// NewHTTPReleaseCatalogFetcher constructs the default public-release client.
func NewHTTPReleaseCatalogFetcher() HTTPReleaseCatalogFetcher {
	return HTTPReleaseCatalogFetcher{
		Client:   &http.Client{Timeout: defaultCatalogFetchTimeout},
		MaxBytes: defaultCatalogMaxBytes,
	}
}

// Fetch retrieves the catalog asset for the exact stable Engine release.
func (f HTTPReleaseCatalogFetcher) Fetch(ctx context.Context, releaseVersion string) ([]byte, error) {
	assetURL, err := ReleaseCatalogURL(releaseVersion)
	if err != nil {
		return nil, err
	}

	maxBytes := f.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultCatalogMaxBytes
	}
	client := f.Client
	if client == nil {
		client = NewHTTPReleaseCatalogFetcher().Client
	}
	clientCopy := *client
	previousRedirectCheck := clientCopy.CheckRedirect
	clientCopy.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("catalog redirect limit exceeded")
		}
		if err := validateCatalogRedirect(request.URL); err != nil {
			return err
		}
		if previousRedirectCheck != nil {
			return previousRedirectCheck(request, via)
		}
		return nil
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create catalog request: %w", err)
	}
	response, err := clientCopy.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download release catalog: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download release catalog: unexpected HTTP status %s", response.Status)
	}
	if response.ContentLength > maxBytes {
		return nil, fmt.Errorf("release catalog exceeds maximum size of %d bytes", maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read release catalog: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("release catalog exceeds maximum size of %d bytes", maxBytes)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("release catalog response is empty")
	}
	return data, nil
}

func validateCatalogRedirect(target *url.URL) error {
	if target.Scheme != "https" {
		return fmt.Errorf("catalog redirect must use HTTPS")
	}
	if target.User != nil {
		return fmt.Errorf("catalog redirect must not contain user information")
	}
	if port := target.Port(); port != "" && port != "443" {
		return fmt.Errorf("catalog redirect uses an unexpected port")
	}
	if _, ok := allowedCatalogRedirectHosts[strings.ToLower(target.Hostname())]; !ok {
		return fmt.Errorf("catalog redirect to unexpected host %q", target.Hostname())
	}
	return nil
}

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
