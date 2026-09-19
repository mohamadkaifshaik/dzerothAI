package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/metrics"
)

// newTestMetrics creates a Metrics instance backed by a fresh isolated registry.
// Using a fresh registry per test prevents cross-test pollution of counter values.
func newTestMetrics(t *testing.T) (*metrics.Metrics, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	m := metrics.New(reg)
	return m, reg
}

// TestMiddleware_CounterIncrement verifies that a request through the middleware
// increments http_requests_total with the correct method, route, and status_code labels.
func TestMiddleware_CounterIncrement(t *testing.T) {
	m, reg := newTestMetrics(t)

	router := chi.NewRouter()
	router.Use(m.Middleware)
	router.Get("/api/v1/posts/{postID}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts/abc-123-uuid", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	var found bool
	for _, mf := range families {
		if mf.GetName() != "http_requests_total" {
			continue
		}
		for _, metric := range mf.GetMetric() {
			var method, route, status string
			for _, lp := range metric.GetLabel() {
				switch lp.GetName() {
				case "method":
					method = lp.GetValue()
				case "route":
					route = lp.GetValue()
				case "status_code":
					status = lp.GetValue()
				}
			}
			if method == "GET" && route == "/api/v1/posts/{postID}" && status == "200" {
				if metric.GetCounter().GetValue() != 1 {
					t.Errorf("http_requests_total: want 1, got %v", metric.GetCounter().GetValue())
				}
				found = true
			}
		}
	}

	if !found {
		t.Error("http_requests_total{method=GET,route=/api/v1/posts/{postID},status_code=200} not found")
	}
}

// TestMiddleware_RoutePatternNotRawPath verifies that the route label uses the chi
// route pattern, not the raw URL path. A UUID in the URL must NOT appear as the label.
func TestMiddleware_RoutePatternNotRawPath(t *testing.T) {
	m, reg := newTestMetrics(t)

	const rawUUID = "550e8400-e29b-41d4-a716-446655440000"

	router := chi.NewRouter()
	router.Use(m.Middleware)
	router.Get("/users/{userID}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/users/"+rawUUID, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	for _, mf := range families {
		if mf.GetName() != "http_requests_total" {
			continue
		}
		for _, metric := range mf.GetMetric() {
			for _, lp := range metric.GetLabel() {
				if lp.GetName() == "route" {
					if lp.GetValue() == "/users/"+rawUUID {
						t.Errorf("route label must not contain raw UUID, got %q", lp.GetValue())
					}
					if lp.GetValue() != "/users/{userID}" {
						t.Errorf("route label: want /users/{userID}, got %q", lp.GetValue())
					}
				}
			}
		}
	}
}

// TestMiddleware_DurationRecorded verifies that http_request_duration_seconds has an
// observation after a request completes.
func TestMiddleware_DurationRecorded(t *testing.T) {
	m, reg := newTestMetrics(t)

	router := chi.NewRouter()
	router.Use(m.Middleware)
	router.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	var found bool
	for _, mf := range families {
		if mf.GetName() == "http_request_duration_seconds" {
			for _, metric := range mf.GetMetric() {
				if metric.GetHistogram().GetSampleCount() > 0 {
					found = true
				}
			}
		}
	}

	if !found {
		t.Error("http_request_duration_seconds: no observations recorded")
	}
}

// TestMiddleware_UnmatchedRoute verifies that unmatched routes use "unknown" as the
// route label instead of the raw URL path (cardinality protection).
func TestMiddleware_UnmatchedRoute(t *testing.T) {
	m, reg := newTestMetrics(t)

	router := chi.NewRouter()
	router.Use(m.Middleware)
	// Deliberately no matching route for the request below.

	req := httptest.NewRequest(http.MethodGet, "/does/not/exist", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	for _, mf := range families {
		if mf.GetName() != "http_requests_total" {
			continue
		}
		for _, metric := range mf.GetMetric() {
			for _, lp := range metric.GetLabel() {
				if lp.GetName() == "route" && lp.GetValue() == "/does/not/exist" {
					t.Errorf("route label for unmatched route must not be raw path, got %q", lp.GetValue())
				}
			}
		}
	}
}

// TestMiddleware_StatusCodeLabel verifies that the status_code label reflects the
// actual HTTP status code returned by the handler.
func TestMiddleware_StatusCodeLabel(t *testing.T) {
	cases := []struct {
		name   string
		code   int
		wantSC string
	}{
		{"200 OK", http.StatusOK, "200"},
		{"404 Not Found", http.StatusNotFound, "404"},
		{"503 Service Unavailable", http.StatusServiceUnavailable, "503"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, reg := newTestMetrics(t)

			router := chi.NewRouter()
			router.Use(m.Middleware)
			code := tc.code
			router.Get("/probe", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			})

			req := httptest.NewRequest(http.MethodGet, "/probe", nil)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			families, err := reg.Gather()
			if err != nil {
				t.Fatalf("gather metrics: %v", err)
			}

			var found bool
			for _, mf := range families {
				if mf.GetName() != "http_requests_total" {
					continue
				}
				for _, metric := range mf.GetMetric() {
					for _, lp := range metric.GetLabel() {
						if lp.GetName() == "status_code" && lp.GetValue() == tc.wantSC {
							found = true
						}
					}
				}
			}

			if !found {
				t.Errorf("status_code=%s not found in http_requests_total labels", tc.wantSC)
			}
		})
	}
}
