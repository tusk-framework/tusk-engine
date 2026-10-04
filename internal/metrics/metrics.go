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
	}, []string{"method", "status"})

	WorkersActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "tusk_workers_active",
		Help: "Number of workers currently processing requests.",
	})

	WorkersTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "tusk_workers_total",
		Help: "Total number of workers in the pool.",
	})

	WorkerStartsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tusk_worker_starts_total",
		Help: "Total number of PHP worker processes started by the native runtime.",
	})

	WorkerStopsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tusk_worker_stops_total",
		Help: "Total number of PHP worker processes stopped by the native runtime.",
	})

	WorkerCrashesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tusk_worker_crashes_total",
		Help: "Total number of unexpected PHP worker exits observed by the native runtime.",
	})

	WorkerTimeoutsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tusk_worker_timeouts_total",
		Help: "Total number of PHP worker requests that exceeded their timeout.",
	})

	WorkerQueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "tusk_worker_queue_depth",
		Help: "Number of PHP workers currently available for a request in the native runtime.",
	})
)
