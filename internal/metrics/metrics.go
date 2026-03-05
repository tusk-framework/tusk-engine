package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	RequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tusk_requests_total",
		Help: "Total number of HTTP requests processed.",
	}, []string{"method", "status"})

	RequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "tusk_request_duration_seconds",
		Help:    "Request duration in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method"})

	WorkersActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "tusk_workers_active",
		Help: "Number of workers currently processing requests.",
	})

	WorkersTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "tusk_workers_total",
		Help: "Total number of workers in the pool.",
	})
)
