// Package metrics provides Prometheus instrumentation helpers for the Dzeroth backend.
// It defines HTTP metrics (request counter and latency histogram) and a chi-compatible
// middleware that records them per request using normalized chi route patterns as labels.
//
// Label cardinality: route labels are always the chi route pattern (e.g. /posts/{postID}),
// never raw URL paths. This keeps cardinality bounded regardless of URL parameter values.
package metrics

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
)

// durationBuckets are the histogram bucket boundaries for HTTP request latency.
// They cover fast cache responses (~5ms) through degraded (1s) to critically slow (2.5s).
var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5}

// Metrics holds the Prometheus instruments used for HTTP observability.
// Create one instance via New and pass it to Middleware.
type Metrics struct {
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
}

// New registers the HTTP metrics with the provided registerer and returns a Metrics
// instance. Pass prometheus.DefaultRegisterer for production use. Pass a fresh
// prometheus.NewRegistry() in tests to avoid cross-test pollution.
func New(reg prometheus.Registerer) *Metrics {
	requestsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests, partitioned by method, route, and status_code.",
		},
		[]string{"method", "route", "status_code"},
	)

	requestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds, partitioned by method, route, and status_code.",
			Buckets: durationBuckets,
		},
		[]string{"method", "route", "status_code"},
	)

	reg.MustRegister(requestsTotal, requestDuration)

	return &Metrics{
		requestsTotal:   requestsTotal,
		requestDuration: requestDuration,
	}
}

// Middleware returns a chi-compatible middleware that records http_requests_total and
// http_request_duration_seconds for every request handled by the router it wraps.
//
// Route label: uses chi.RouteContext(r.Context()).RoutePattern() evaluated AFTER the
// handler executes so that chi has matched and populated the route pattern. If the route
// context is absent or the pattern is empty (unmatched route), "unknown" is used.
//
// This middleware must only be applied to the public API router, not the admin listener.
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wrap ResponseWriter so we can capture the status code written by the handler.
		rw := newStatusRecorder(w)

		// Start timer BEFORE calling next so the full handler latency is measured.
		timer := prometheus.NewTimer(prometheus.ObserverFunc(func(duration float64) {
			route := routePattern(r)
			status := strconv.Itoa(rw.status)
			m.requestsTotal.WithLabelValues(r.Method, route, status).Inc()
			m.requestDuration.WithLabelValues(r.Method, route, status).Observe(duration)
		}))

		next.ServeHTTP(rw, r)

		// ObserverFunc is invoked here with the elapsed duration.
		timer.ObserveDuration()
	})
}

// routePattern extracts the normalized chi route pattern from the request context.
// This must be called after the handler runs so chi has populated the route context.
// Returns "unknown" for unmatched routes to avoid unbounded cardinality.
func routePattern(r *http.Request) string {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return "unknown"
	}
	pattern := rctx.RoutePattern()
	if pattern == "" {
		return "unknown"
	}
	return pattern
}

// statusRecorder wraps http.ResponseWriter to capture the response status code.
// The default status is 200 (matching net/http behaviour when WriteHeader is not called).
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func newStatusRecorder(w http.ResponseWriter) *statusRecorder {
	return &statusRecorder{ResponseWriter: w, status: http.StatusOK}
}

// WriteHeader captures the status code before delegating to the underlying writer.
func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
