// Package middleware provides chi-compatible HTTP middleware for the Dzeroth backend.
// It reuses the existing zap structured logging conventions established across the project.
package middleware

import (
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

// AccessLog returns a chi-compatible middleware that emits one structured zap log entry
// per HTTP request after the handler returns.
//
// Fields logged per request:
//   - request_id   — chi request ID from context (chimw.GetReqID)
//   - method       — HTTP verb (GET, POST, …)
//   - route        — normalized chi route pattern (/posts/{postID}), never raw path
//   - status       — HTTP response status code written by the handler
//   - duration_ms  — elapsed time in milliseconds (float64)
//   - remote_ip    — client IP address (port stripped)
//
// Fields deliberately NOT logged: Authorization, Cookie, request body, raw URL path,
// query string, user IDs, or tokens.
//
// Log level rules:
//   - 5xx → zap.Warn  (server errors are actionable; use Warn to keep severity reasonable)
//   - 4xx → zap.Info  (normal client errors; not operator-actionable)
//   - 2xx/3xx → zap.Info
//
// The middleware also sets the X-Request-Id response header so callers can correlate
// responses to server-side log entries. chimw.RequestID must be registered before this
// middleware in the router's stack.
//
// The logger is received as a parameter; no global logger is used.
func AccessLog(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Propagate the request ID in the response so callers can correlate
			// their request to the server-side log entry.
			requestID := chimw.GetReqID(r.Context())
			if requestID != "" {
				w.Header().Set("X-Request-Id", requestID)
			}

			// Wrap the ResponseWriter to capture the status code.
			rw := &responseRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rw, r)

			durationMS := float64(time.Since(start).Microseconds()) / 1000.0

			fields := []zap.Field{
				zap.String("request_id", chimw.GetReqID(r.Context())),
				zap.String("method", r.Method),
				zap.String("route", routePattern(r)),
				zap.Int("status", rw.status),
				zap.Float64("duration_ms", durationMS),
				zap.String("remote_ip", remoteIP(r.RemoteAddr)),
			}

			if rw.status >= 500 {
				log.Warn("http access", fields...)
			} else {
				log.Info("http access", fields...)
			}
		})
	}
}

// routePattern extracts the normalized chi route pattern from the request context.
// This is called after the handler so chi has had time to populate the route context.
// Returns "unknown" when the route context is absent or the pattern is empty (unmatched route).
func routePattern(r *http.Request) string {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return "unknown"
	}
	p := rctx.RoutePattern()
	if p == "" {
		return "unknown"
	}
	return p
}

// remoteIP returns only the IP portion of an addr string formatted as "host:port".
// If the address cannot be split (e.g. a Unix socket path or already a bare IP),
// the original string is returned unchanged.
func remoteIP(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

// responseRecorder wraps http.ResponseWriter to capture the status code written by the handler.
// The default status is 200 (matching net/http behaviour when WriteHeader is never called).
type responseRecorder struct {
	http.ResponseWriter
	status int
}

// WriteHeader captures the status code before delegating to the underlying writer.
func (rr *responseRecorder) WriteHeader(code int) {
	rr.status = code
	rr.ResponseWriter.WriteHeader(code)
}
