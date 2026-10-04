package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestRequestMetricsUseOnlyBoundedLabels(t *testing.T) {
	RequestsTotal.WithLabelValues("GET", "200").Inc()
	RequestDuration.WithLabelValues("GET", "200").Observe(0.001)

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	for _, name := range []string{"tusk_requests_total", "tusk_request_duration_seconds"} {
		family := findFamily(families, name)
		if family == nil || len(family.Metric) == 0 {
			t.Fatalf("metric family %q was not collected", name)
		}
		for _, metric := range family.Metric {
			if len(metric.Label) != 2 {
				t.Fatalf("metric %q has %d labels, want 2", name, len(metric.Label))
			}
		}
	}
}

func TestWorkerLifecycleCollectorsAreRegistered(t *testing.T) {
	for _, name := range []string{
		"tusk_worker_starts_total",
		"tusk_worker_stops_total",
		"tusk_worker_crashes_total",
		"tusk_worker_timeouts_total",
		"tusk_worker_queue_depth",
	} {
		if findFamilyMustGather(t, name) == nil {
			t.Fatalf("metric family %q was not registered", name)
		}
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
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	return findFamily(families, name)
}
