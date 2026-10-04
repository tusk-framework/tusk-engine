package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry contains only Tusk-owned metrics. RoadRunner's native exposition
// is composed into the protected Control API handler without registering its
// families a second time in this process.
var Registry = prometheus.NewRegistry()

var (
	RequestsTotal = promauto.With(Registry).NewCounterVec(prometheus.CounterOpts{
		Name: "tusk_requests_total",
		Help: "Total number of HTTP requests processed by the native runtime.",
	}, []string{"method", "status"})

	RequestDuration = promauto.With(Registry).NewHistogramVec(prometheus.HistogramOpts{
		Name:    "tusk_request_duration_seconds",
		Help:    "Request duration in seconds for the native runtime.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "status"})

	WorkersActive = promauto.With(Registry).NewGauge(prometheus.GaugeOpts{
		Name: "tusk_workers_active",
		Help: "Number of native workers currently processing requests.",
	})

	WorkersTotal = promauto.With(Registry).NewGauge(prometheus.GaugeOpts{
		Name: "tusk_workers_total",
		Help: "Total number of native workers in the pool.",
	})

	WorkerStartsTotal = promauto.With(Registry).NewCounter(prometheus.CounterOpts{
		Name: "tusk_worker_starts_total",
		Help: "Total number of PHP worker processes started by the native runtime.",
	})

	WorkerStopsTotal = promauto.With(Registry).NewCounter(prometheus.CounterOpts{
		Name: "tusk_worker_stops_total",
		Help: "Total number of PHP worker processes stopped by the native runtime.",
	})

	WorkerCrashesTotal = promauto.With(Registry).NewCounter(prometheus.CounterOpts{
		Name: "tusk_worker_crashes_total",
		Help: "Total number of unexpected PHP worker exits observed by the native runtime.",
	})

	WorkerTimeoutsTotal = promauto.With(Registry).NewCounter(prometheus.CounterOpts{
		Name: "tusk_worker_timeouts_total",
		Help: "Total number of PHP worker requests that exceeded their timeout.",
	})

	WorkerQueueDepth = promauto.With(Registry).NewGauge(prometheus.GaugeOpts{
		Name: "tusk_worker_queue_depth",
		Help: "Number of PHP workers currently available in the native runtime.",
	})

	RoadRunnerStarts = promauto.With(Registry).NewCounter(prometheus.CounterOpts{
		Name: "tusk_roadrunner_starts_total",
		Help: "Total number of RoadRunner processes successfully started by the Engine.",
	})

	RoadRunnerStops = promauto.With(Registry).NewCounter(prometheus.CounterOpts{
		Name: "tusk_roadrunner_stops_total",
		Help: "Total number of RoadRunner processes stopped by the Engine.",
	})

	RoadRunnerCrashes = promauto.With(Registry).NewCounter(prometheus.CounterOpts{
		Name: "tusk_roadrunner_crashes_total",
		Help: "Total number of unexpected RoadRunner process exits.",
	})

	RoadRunnerReadinessTimeouts = promauto.With(Registry).NewCounter(prometheus.CounterOpts{
		Name: "tusk_roadrunner_readiness_timeouts_total",
		Help: "Total number of RoadRunner readiness deadlines exceeded.",
	})

	RoadRunnerMetricsUp = promauto.With(Registry).NewGauge(prometheus.GaugeOpts{
		Name: "tusk_roadrunner_metrics_up",
		Help: "Whether the latest local RoadRunner metrics scrape succeeded.",
	})
)

var allowedMethods = map[string]struct{}{
	"CONNECT": {}, "DELETE": {}, "GET": {}, "HEAD": {}, "OPTIONS": {},
	"PATCH": {}, "POST": {}, "PUT": {}, "TRACE": {},
}

// ObserveRequest records a native request with finite method/status labels.
func ObserveRequest(method string, status int, duration time.Duration) {
	method = normalizeMethod(method)
	statusLabel := normalizeStatus(status)
	RequestsTotal.WithLabelValues(method, statusLabel).Inc()
	RequestDuration.WithLabelValues(method, statusLabel).Observe(duration.Seconds())
}

func normalizeMethod(method string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if _, ok := allowedMethods[method]; !ok {
		return "OTHER"
	}
	return method
}

func normalizeStatus(status int) string {
	if status < 100 || status > 599 {
		return "unknown"
	}
	return strconv.Itoa(status)
}

// NewHandler returns a Prometheus handler that exposes Engine metrics and,
// when configured, appends the local RoadRunner scrape.
func NewHandler(backendURL string) http.Handler {
	return &compositeHandler{
		backendURL: strings.TrimSpace(backendURL),
		client:     &http.Client{Timeout: 2 * time.Second},
	}
}

type compositeHandler struct {
	backendURL string
	client     *http.Client
}

func (h *compositeHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	backend, ok := h.scrapeRoadRunner(request)
	RoadRunnerMetricsUp.Set(boolToFloat(ok))

	engine := httptest.NewRecorder()
	promhttp.HandlerFor(Registry, promhttp.HandlerOpts{}).ServeHTTP(engine, request)

	writer.Header().Set("Content-Type", engine.Header().Get("Content-Type"))
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(engine.Body.Bytes())
	if ok {
		if body := engine.Body.Bytes(); len(body) > 0 && body[len(body)-1] != '\n' {
			_, _ = writer.Write([]byte{'\n'})
		}
		_, _ = writer.Write(backend)
	}
}

func (h *compositeHandler) scrapeRoadRunner(incoming *http.Request) ([]byte, bool) {
	if h.backendURL == "" {
		return nil, false
	}
	request, err := http.NewRequestWithContext(incoming.Context(), http.MethodGet, h.backendURL, nil)
	if err != nil {
		return nil, false
	}
	response, err := h.client.Do(request)
	if err != nil {
		return nil, false
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, false
	}
	return body, true
}

func boolToFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
