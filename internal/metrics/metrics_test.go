package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestRoadRunnerCollectorsAreRegistered(t *testing.T) {
	for _, name := range []string{
		"tusk_roadrunner_starts_total",
		"tusk_roadrunner_stops_total",
		"tusk_roadrunner_crashes_total",
		"tusk_roadrunner_readiness_timeouts_total",
		"tusk_roadrunner_metrics_up",
	} {
		if findFamilyMustGather(t, name) == nil {
			t.Fatalf("metric family %q was not registered", name)
		}
	}
}

func TestHandlerComposesRoadRunnerScrapeAndReportsAvailability(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = writer.Write([]byte("# HELP rr_http_requests_queue Queue\nrr_http_requests_queue 2\n"))
	}))
	defer backend.Close()

	handler := NewHandler(backend.URL)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/metrics", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	contents := recorder.Body.String()
	if !strings.Contains(contents, "tusk_roadrunner_metrics_up 1") {
		t.Fatalf("availability metric missing from scrape:\n%s", contents)
	}
	if !strings.Contains(contents, "rr_http_requests_queue 2") {
		t.Fatalf("RoadRunner scrape missing:\n%s", contents)
	}
}

func TestHandlerKeepsEngineMetricsWhenRoadRunnerUnavailable(t *testing.T) {
	handler := NewHandler("http://127.0.0.1:1/metrics")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/metrics", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "tusk_roadrunner_metrics_up 0") {
		t.Fatalf("failed availability metric missing:\n%s", recorder.Body.String())
	}
}

func findFamily(families []*dto.MetricFamily, name string) *dto.MetricFamily {
	for _, family := range families {
		if family.GetName() == name {
			return family
		}
	}
	return nil
}

func findFamilyMustGather(t *testing.T, name string) *dto.MetricFamily {
	t.Helper()
	families, err := Registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	return findFamily(families, name)
}

var _ prometheus.Gatherer = Registry
