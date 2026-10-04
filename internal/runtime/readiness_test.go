package runtime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPReadinessProbeRetriesStatusesAndTransportErrors(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		switch calls.Add(1) {
		case 1:
			writer.WriteHeader(http.StatusServiceUnavailable)
		case 2:
			writer.WriteHeader(http.StatusBadGateway)
		default:
			writer.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	probe := HTTPReadinessProbe{URL: server.URL + "/ready?plugin=http"}
	if err := probe.Check(context.Background()); !errors.Is(err, ErrNotReady) {
		t.Fatalf("first Check() error = %v, want not ready", err)
	}
	if err := probe.Check(context.Background()); err == nil || !errors.Is(err, ErrProbeFailed) {
		t.Fatalf("second Check() error = %v, want probe failure", err)
	}
	if err := probe.Check(context.Background()); err != nil {
		t.Fatalf("third Check() error = %v, want success", err)
	}
}

func TestHTTPReadinessProbeHonorsContextDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()

	probe := HTTPReadinessProbe{URL: server.URL + "/ready?plugin=http"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := probe.Check(ctx); err == nil {
		t.Fatal("Check() succeeded after context deadline")
	}
}

func TestHTTPReadinessProbeRejectsMalformedURL(t *testing.T) {
	probe := HTTPReadinessProbe{URL: "://bad"}
	if err := probe.Check(context.Background()); err == nil || !errors.Is(err, ErrProbeFailed) {
		t.Fatalf("Check() error = %v, want probe failure", err)
	}
}
