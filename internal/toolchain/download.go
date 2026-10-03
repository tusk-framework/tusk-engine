package toolchain

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Downloader retrieves a single artifact while enforcing transport and size
// policy.
type Downloader interface {
	Download(ctx context.Context, rawURL string, maxBytes int64) ([]byte, error)
}

// HTTPDownloader is the standard HTTPS artifact downloader.
type HTTPDownloader struct {
	Client  *http.Client
	Timeout time.Duration
}

// Download retrieves an HTTPS response, rejecting non-success responses and
// bodies larger than maxBytes.
func (d HTTPDownloader) Download(ctx context.Context, rawURL string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("download maximum must be positive")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, fmt.Errorf("download URL must use HTTPS and include a host")
	}

	client := d.Client
	if client == nil {
		client = &http.Client{}
	}
	requestContext := ctx
	var cancel context.CancelFunc
	if d.Timeout > 0 {
		requestContext, cancel = context.WithTimeout(ctx, d.Timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(requestContext, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create download request: %w", err)
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download artifact: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("download artifact: HTTP status %s", response.Status)
	}

	limit := maxBytes
	if maxBytes < int64(^uint64(0)>>1) {
		limit++
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit))
	if err != nil {
		return nil, fmt.Errorf("read artifact: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("artifact exceeds maximum size of %d bytes", maxBytes)
	}
	return data, nil
}
