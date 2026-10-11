package toolchain

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNormalizeReleaseVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
		wantErr bool
	}{
		{name: "stable release", version: "0.3.2", want: "0.3.2"},
		{name: "empty", version: "", wantErr: true},
		{name: "development build", version: "dev", wantErr: true},
		{name: "tag prefix", version: "v0.3.2", wantErr: true},
		{name: "malformed", version: "three", wantErr: true},
		{name: "prerelease", version: "0.3.2-rc.1", wantErr: true},
		{name: "leading zero", version: "00.3.2", wantErr: true},
		{name: "whitespace", version: " 0.3.2", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeReleaseVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NormalizeReleaseVersion(%q) error = %v, wantErr %t", tt.version, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("NormalizeReleaseVersion(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

func TestReleaseCatalogURL(t *testing.T) {
	got, err := ReleaseCatalogURL("0.3.2")
	if err != nil {
		t.Fatalf("ReleaseCatalogURL() error = %v", err)
	}
	const want = "https://github.com/tusk-framework/tusk-engine/releases/download/v0.3.2/toolchain-catalog.json"
	if got != want {
		t.Fatalf("ReleaseCatalogURL() = %q, want %q", got, want)
	}

	if _, err := ReleaseCatalogURL("dev"); err == nil {
		t.Fatal("ReleaseCatalogURL(\"dev\") succeeded, want invalid release version error")
	}
}

type catalogFetchRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip catalogFetchRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestHTTPReleaseCatalogFetcher(t *testing.T) {
	tests := []struct {
		name    string
		client  func(t *testing.T, requests *[]*http.Request) *http.Client
		max     int64
		want    string
		wantErr string
	}{
		{
			name: "downloads successful response",
			client: func(_ *testing.T, requests *[]*http.Request) *http.Client {
				return catalogFetchClient(func(request *http.Request) (*http.Response, error) {
					*requests = append(*requests, request)
					return catalogFetchResponse(request, http.StatusOK, `{"catalog":true}`, ""), nil
				})
			},
			max: 64, want: `{"catalog":true}`,
		},
		{
			name: "rejects non-success status",
			client: func(_ *testing.T, requests *[]*http.Request) *http.Client {
				return catalogFetchClient(func(request *http.Request) (*http.Response, error) {
					*requests = append(*requests, request)
					return catalogFetchResponse(request, http.StatusNotFound, "missing", ""), nil
				})
			},
			max: 64, wantErr: "404",
		},
		{
			name: "rejects empty body",
			client: func(_ *testing.T, requests *[]*http.Request) *http.Client {
				return catalogFetchClient(func(request *http.Request) (*http.Response, error) {
					*requests = append(*requests, request)
					return catalogFetchResponse(request, http.StatusOK, "", ""), nil
				})
			},
			max: 64, wantErr: "empty",
		},
		{
			name: "rejects oversized body",
			client: func(_ *testing.T, requests *[]*http.Request) *http.Client {
				return catalogFetchClient(func(request *http.Request) (*http.Response, error) {
					*requests = append(*requests, request)
					return catalogFetchResponse(request, http.StatusOK, "123456", ""), nil
				})
			},
			max: 5, wantErr: "exceeds",
		},
		{
			name: "follows allowed https asset redirect",
			client: func(_ *testing.T, requests *[]*http.Request) *http.Client {
				return catalogFetchClient(func(request *http.Request) (*http.Response, error) {
					*requests = append(*requests, request)
					if request.URL.Host == "github.com" {
						return catalogFetchResponse(request, http.StatusFound, "", "https://release-assets.githubusercontent.com/catalog.json"), nil
					}
					return catalogFetchResponse(request, http.StatusOK, "signed catalog", ""), nil
				})
			},
			max: 64, want: "signed catalog",
		},
		{
			name: "rejects https redirect to unexpected host",
			client: func(_ *testing.T, requests *[]*http.Request) *http.Client {
				return catalogFetchClient(func(request *http.Request) (*http.Response, error) {
					*requests = append(*requests, request)
					return catalogFetchResponse(request, http.StatusFound, "", "https://evil.example/catalog.json"), nil
				})
			},
			max: 64, wantErr: "redirect",
		},
		{
			name: "rejects redirect downgrade to http",
			client: func(_ *testing.T, requests *[]*http.Request) *http.Client {
				return catalogFetchClient(func(request *http.Request) (*http.Response, error) {
					*requests = append(*requests, request)
					return catalogFetchResponse(request, http.StatusFound, "", "http://objects.githubusercontent.com/catalog.json"), nil
				})
			},
			max: 64, wantErr: "https",
		},
		{
			name: "rejects redirect loop",
			client: func(_ *testing.T, requests *[]*http.Request) *http.Client {
				return catalogFetchClient(func(request *http.Request) (*http.Response, error) {
					*requests = append(*requests, request)
					return catalogFetchResponse(request, http.StatusFound, "", request.URL.String()), nil
				})
			},
			max: 64, wantErr: "redirect",
		},
		{
			name: "does not send authorization header",
			client: func(_ *testing.T, requests *[]*http.Request) *http.Client {
				return catalogFetchClient(func(request *http.Request) (*http.Response, error) {
					*requests = append(*requests, request)
					if request.Header.Get("Authorization") != "" {
						return nil, errors.New("unexpected authorization header")
					}
					return catalogFetchResponse(request, http.StatusOK, "catalog", ""), nil
				})
			},
			max: 64, want: "catalog",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests []*http.Request
			fetcher := HTTPReleaseCatalogFetcher{Client: tt.client(t, &requests), MaxBytes: tt.max}
			got, err := fetcher.Fetch(context.Background(), "0.3.2")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.wantErr)) {
					t.Fatalf("Fetch() error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Fetch() error = %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("Fetch() = %q, want %q", got, tt.want)
			}
			if len(requests) == 0 {
				t.Fatal("Fetch() sent no request")
			}
		})
	}
}

func TestHTTPReleaseCatalogFetcherHonorsContextCancellation(t *testing.T) {
	client := catalogFetchClient(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fetcher := HTTPReleaseCatalogFetcher{Client: client, MaxBytes: 64}
	if _, err := fetcher.Fetch(ctx, "0.3.2"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Fetch() error = %v, want context.Canceled", err)
	}
}

func TestHTTPReleaseCatalogFetcherHonorsClientTimeout(t *testing.T) {
	client := catalogFetchClient(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	client.Timeout = time.Millisecond
	fetcher := HTTPReleaseCatalogFetcher{Client: client, MaxBytes: 64}
	if _, err := fetcher.Fetch(context.Background(), "0.3.2"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Fetch() error = %v, want context.DeadlineExceeded", err)
	}
}

func catalogFetchClient(roundTrip catalogFetchRoundTripper) *http.Client {
	return &http.Client{Transport: roundTrip, Timeout: time.Second}
}

func catalogFetchResponse(request *http.Request, status int, body, location string) *http.Response {
	header := make(http.Header)
	if location != "" {
		header.Set("Location", location)
	}
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}
