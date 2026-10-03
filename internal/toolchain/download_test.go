package toolchain

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPDownloaderAcceptsHTTPSAndEnforcesLimit(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	downloader := HTTPDownloader{Client: server.Client()}
	data, err := downloader.Download(context.Background(), server.URL, 32)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("Download() = %q, want hello", data)
	}

	_, err = downloader.Download(context.Background(), server.URL, 4)
	if err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("limited Download() error = %v, want maximum-size error", err)
	}
}

func TestHTTPDownloaderRejectsInsecureAndNonSuccessResponses(t *testing.T) {
	insecure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer insecure.Close()

	downloader := HTTPDownloader{}
	_, err := downloader.Download(context.Background(), insecure.URL, 32)
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("insecure Download() error = %v, want HTTPS error", err)
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusBadGateway)
	}))
	defer server.Close()
	downloader.Client = server.Client()
	_, err = downloader.Download(context.Background(), server.URL, 32)
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("non-success Download() error = %v, want status error", err)
	}
}

func TestHTTPDownloaderHonorsCancellation(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (HTTPDownloader{Client: server.Client()}).Download(ctx, server.URL, 32)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Download() error = %v, want context.Canceled", err)
	}
}
