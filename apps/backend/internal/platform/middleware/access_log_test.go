package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	platformmw "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/middleware"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// newObservedLogger returns a zap logger whose output is captured in the returned
// observer.ObservedLogs. Use observed.All() after the request to inspect log entries.
func newObservedLogger(t *testing.T) (*zap.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	return zap.New(core), logs
}

// newRouter builds a chi router with chimw.RequestID and the AccessLog middleware
// followed by the provided handler registered at the given method+path.
func newRouter(log *zap.Logger, method, path string, h http.HandlerFunc) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(platformmw.AccessLog(log))
	r.Method(method, path, h)
	return r
}

// ── request ID tests ──────────────────────────────────────────────────────────

// TestRequestID_GeneratedWhenAbsent verifies that a request without an X-Request-Id
// header still gets a non-empty request ID assigned by chimw.RequestID.
func TestRequestID_GeneratedWhenAbsent(t *testing.T) {
	log, logs := newObservedLogger(t)
	router := newRouter(log, http.MethodGet, "/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	// Explicitly do NOT set X-Request-Id.
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	entries := logs.All()
	if len(entries) == 0 {
		t.Fatal("expected one log entry, got zero")
	}

	var requestID string
	for _, f := range entries[0].Context {
		if f.Key == "request_id" {
			requestID = f.String
			break
		}
	}

	if requestID == "" {
		t.Error("request_id log field should be non-empty when no X-Request-Id header is supplied")
	}
}

// TestRequestID_AppearsInResponseHeader verifies that the X-Request-Id header is
// present in the response (set by chimw.RequestID before our middleware runs).
func TestRequestID_AppearsInResponseHeader(t *testing.T) {
	log, _ := newObservedLogger(t)
	router := newRouter(log, http.MethodGet, "/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	// chimw.RequestID sets X-Request-Id on the response.
	if id := rr.Header().Get("X-Request-Id"); id == "" {
		t.Error("X-Request-Id response header should be set by chimw.RequestID")
	}
}

// TestRequestID_UniquePerRequest verifies that concurrent requests receive distinct
// request IDs.
func TestRequestID_UniquePerRequest(t *testing.T) {
	log, logs := newObservedLogger(t)
	router := newRouter(log, http.MethodGet, "/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	const concurrency = 20
	var wg sync.WaitGroup
	wg.Add(concurrency)

	for range concurrency {
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/ping", nil)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
		}()
	}
	wg.Wait()

	seen := make(map[string]struct{})
	for _, entry := range logs.All() {
		for _, f := range entry.Context {
			if f.Key == "request_id" {
				if _, dup := seen[f.String]; dup {
					t.Errorf("duplicate request_id detected: %q", f.String)
				}
				seen[f.String] = struct{}{}
			}
		}
	}

	if len(seen) != concurrency {
		t.Errorf("expected %d unique request IDs, got %d", concurrency, len(seen))
	}
}

// ── access log content tests ──────────────────────────────────────────────────

// TestAccessLog_SuccessLogsInfoWithFields verifies that a 200 response produces
// a zap.Info entry containing all required structured fields.
func TestAccessLog_SuccessLogsInfoWithFields(t *testing.T) {
	log, logs := newObservedLogger(t)
	router := newRouter(log, http.MethodGet, "/api/v1/posts/{postID}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts/some-uuid-here", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	entries := logs.All()
	if len(entries) == 0 {
		t.Fatal("expected one log entry, got zero")
	}

	entry := entries[0]

	if entry.Level != zapcore.InfoLevel {
		t.Errorf("log level: want Info, got %s", entry.Level)
	}

	fields := fieldMap(entry.Context)

	if _, ok := fields["request_id"]; !ok {
		t.Error("log entry missing field: request_id")
	}
	if method, _ := fields["method"]; method != "GET" {
		t.Errorf("method: want GET, got %q", method)
	}
	if _, ok := fields["route"]; !ok {
		t.Error("log entry missing field: route")
	}
	if status, _ := fields["status"]; status != "200" {
		t.Errorf("status: want 200, got %q", status)
	}
	if _, ok := fields["duration_ms"]; !ok {
		t.Error("log entry missing field: duration_ms")
	}
	if _, ok := fields["remote_ip"]; !ok {
		t.Error("log entry missing field: remote_ip")
	}
}

// TestAccessLog_RoutePatternNotRawPath verifies that the route field uses the
// normalized chi route pattern and not the raw URL path containing the actual UUID.
func TestAccessLog_RoutePatternNotRawPath(t *testing.T) {
	log, logs := newObservedLogger(t)
	router := newRouter(log, http.MethodGet, "/api/v1/posts/{postID}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	const rawUUID = "550e8400-e29b-41d4-a716-446655440000"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+rawUUID, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	entries := logs.All()
	if len(entries) == 0 {
		t.Fatal("expected one log entry, got zero")
	}

	fields := fieldMap(entries[0].Context)
	route, ok := fields["route"]
	if !ok {
		t.Fatal("log entry missing field: route")
	}

	if route == "/api/v1/posts/"+rawUUID {
		t.Errorf("route field must not contain raw UUID path, got %q", route)
	}
	if route != "/api/v1/posts/{postID}" {
		t.Errorf("route field: want /api/v1/posts/{postID}, got %q", route)
	}
}

// TestAccessLog_4xxLogsInfo verifies that a 4xx response logs at Info level (not Error).
func TestAccessLog_4xxLogsInfo(t *testing.T) {
	log, logs := newObservedLogger(t)
	router := newRouter(log, http.MethodGet, "/resource", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	req := httptest.NewRequest(http.MethodGet, "/resource", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	entries := logs.All()
	if len(entries) == 0 {
		t.Fatal("expected one log entry, got zero")
	}

	if entries[0].Level != zapcore.InfoLevel {
		t.Errorf("4xx: log level: want Info, got %s", entries[0].Level)
	}
}

// TestAccessLog_5xxLogsWarn verifies that a 5xx response logs at Warn level.
func TestAccessLog_5xxLogsWarn(t *testing.T) {
	log, logs := newObservedLogger(t)
	router := newRouter(log, http.MethodGet, "/broken", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	req := httptest.NewRequest(http.MethodGet, "/broken", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	entries := logs.All()
	if len(entries) == 0 {
		t.Fatal("expected one log entry, got zero")
	}

	if entries[0].Level != zapcore.WarnLevel {
		t.Errorf("5xx: log level: want Warn, got %s", entries[0].Level)
	}
}

// TestAccessLog_AuthorizationHeaderNotLogged verifies that the Authorization header
// value never appears in any log field (privacy/security protection).
func TestAccessLog_AuthorizationHeaderNotLogged(t *testing.T) {
	log, _ := newObservedLogger(t)

	// Use a JSON-encoding core to capture the actual serialized log output.
	core, buf := jsonObserver()
	jsonLog := zap.New(core)

	router := newRouter(jsonLog, http.MethodGet, "/secure", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	const sensitiveToken = "Bearer super-secret-token-value-12345"
	req := httptest.NewRequest(http.MethodGet, "/secure", nil)
	req.Header.Set("Authorization", sensitiveToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	_ = log // suppress unused warning
	output := buf.String()
	if containsSubstring(output, "super-secret-token-value-12345") {
		t.Errorf("Authorization header value must not appear in log output, but found it in: %s", output)
	}
}

// TestAccessLog_CookieHeaderNotLogged verifies that the Cookie header value never
// appears in any log field (privacy protection).
func TestAccessLog_CookieHeaderNotLogged(t *testing.T) {
	core, buf := jsonObserver()
	jsonLog := zap.New(core)

	router := newRouter(jsonLog, http.MethodGet, "/cookied", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	const sessionCookie = "session=abc123xyz-private-cookie-data"
	req := httptest.NewRequest(http.MethodGet, "/cookied", nil)
	req.Header.Set("Cookie", sessionCookie)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	output := buf.String()
	if containsSubstring(output, "abc123xyz-private-cookie-data") {
		t.Errorf("Cookie header value must not appear in log output, but found it in: %s", output)
	}
}

// TestAccessLog_RawPathNotLogged verifies that the raw URL path (containing actual
// parameter values) does not appear in the route log field.
func TestAccessLog_RawPathNotLogged(t *testing.T) {
	core, buf := jsonObserver()
	jsonLog := zap.New(core)

	router := newRouter(jsonLog, http.MethodGet, "/users/{userID}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	const rawID = "private-user-id-99999"
	req := httptest.NewRequest(http.MethodGet, "/users/"+rawID, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	// Parse the JSON log and check the route field specifically.
	output := buf.String()
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("failed to parse log JSON: %v (raw: %s)", err, output)
	}

	route, _ := logEntry["route"].(string)
	if route == "/users/"+rawID {
		t.Errorf("route field must not contain raw path value, got %q", route)
	}
	if route != "/users/{userID}" {
		t.Errorf("route field: want /users/{userID}, got %q", route)
	}
}

// TestAccessLog_DurationNonNegative verifies that the duration_ms field is
// present and non-negative in the log entry.
func TestAccessLog_DurationNonNegative(t *testing.T) {
	log, logs := newObservedLogger(t)
	router := newRouter(log, http.MethodGet, "/timing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/timing", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	entries := logs.All()
	if len(entries) == 0 {
		t.Fatal("expected one log entry, got zero")
	}

	var durationFound bool
	for _, f := range entries[0].Context {
		if f.Key == "duration_ms" {
			durationFound = true
			if f.Integer < 0 {
				t.Errorf("duration_ms should be non-negative, got %d", f.Integer)
			}
		}
	}

	if !durationFound {
		t.Error("log entry missing field: duration_ms")
	}
}

// ── test utilities ────────────────────────────────────────────────────────────

// fieldMap converts a slice of zap.Field into a string→string map for easy assertion.
// Integer and float fields are converted to their decimal string representation.
func fieldMap(fields []zap.Field) map[string]string {
	m := make(map[string]string, len(fields))
	for _, f := range fields {
		switch f.Type {
		case zapcore.StringType:
			m[f.Key] = f.String
		case zapcore.Int64Type, zapcore.Int32Type, zapcore.Int16Type, zapcore.Int8Type:
			m[f.Key] = intToString(f.Integer)
		case zapcore.Float64Type:
			m[f.Key] = "float64_present"
		default:
			m[f.Key] = "<non-string>"
		}
	}
	return m
}

func intToString(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := make([]byte, 0, 20)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

// jsonObserver creates a zapcore that writes JSON to an in-memory buffer.
// Returns the core and the buffer (as a *jsonBuffer).
func jsonObserver() (zapcore.Core, *jsonBuffer) {
	buf := &jsonBuffer{}
	enc := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	core := zapcore.NewCore(enc, buf, zapcore.DebugLevel)
	return core, buf
}

// jsonBuffer is a thread-safe in-memory write sink that zapcore.WriteSyncer can use.
type jsonBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *jsonBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *jsonBuffer) Sync() error { return nil }

func (b *jsonBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}

// containsSubstring reports whether s contains substr.
func containsSubstring(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
