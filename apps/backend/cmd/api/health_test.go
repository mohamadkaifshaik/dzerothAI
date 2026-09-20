// Package main health handler tests.
// These tests use httptest — no real database or Redis connection is required.
// The testable *FromCheckers variants of the health handlers accept interface types
// so mock implementations can be injected.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

// ── Mock helpers ──────────────────────────────────────────────────────────────

// mockDBPinger implements dbPinger for testing.
type mockDBPinger struct {
	err error
}

func (m *mockDBPinger) Ping(_ context.Context) error { return m.err }

// mockDBChecker implements dbChecker (dbPinger + dbStatter) for testing.
type mockDBChecker struct {
	pingErr error
	stats   dbPoolStats
}

func (m *mockDBChecker) Ping(_ context.Context) error { return m.pingErr }

func (m *mockDBChecker) PoolStats() dbPoolStats { return m.stats }

// mockRedisChecker implements redisChecker for testing.
type mockRedisChecker struct {
	available bool
}

func (m *mockRedisChecker) IsAvailable(_ context.Context) bool { return m.available }

// nopLog is a no-op zap logger for use in handler tests.
var nopLog = zap.NewNop()

// ── /livez tests ──────────────────────────────────────────────────────────────

// TestLivez_AlwaysOK verifies that /livez always returns 200 with status="ok"
// regardless of dependency state. Liveness must not check dependencies.
func TestLivez_AlwaysOK(t *testing.T) {
	handler := buildLivezHandler()

	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("livez: want 200, got %d", rr.Code)
	}

	var body livezResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("livez: decode body: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("livez: status: want %q, got %q", "ok", body.Status)
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("livez: Content-Type: want application/json, got %q", ct)
	}
}

// TestLivez_NoSensitiveData verifies that the livez response body contains no
// sensitive data such as credentials, connection strings, or internal addresses.
func TestLivez_NoSensitiveData(t *testing.T) {
	handler := buildLivezHandler()
	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	body := rr.Body.String()
	// The only acceptable content is the status field.
	for _, forbidden := range []string{"password", "secret", "token", "dsn", "addr", "host"} {
		if containsCI(body, forbidden) {
			t.Errorf("livez: response contains forbidden term %q: %s", forbidden, body)
		}
	}
}

// ── /readyz tests ─────────────────────────────────────────────────────────────

// TestReadyz_AllHealthy verifies that when both DB and Redis are reachable,
// /readyz returns 200 with status="ready".
func TestReadyz_AllHealthy(t *testing.T) {
	db := &mockDBPinger{err: nil}
	rc := &mockRedisChecker{available: true}

	handler := buildReadyzHandlerFromCheckers(db, rc, nopLog, nil)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("readyz healthy: want 200, got %d", rr.Code)
	}

	var body readyzResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("readyz healthy: decode body: %v", err)
	}
	if body.Status != "ready" {
		t.Errorf("readyz healthy: status: want %q, got %q", "ready", body.Status)
	}
}

// TestReadyz_DBDown verifies that a DB ping failure causes /readyz to return 503
// with db="unavailable" and the overall status="not_ready".
func TestReadyz_DBDown(t *testing.T) {
	db := &mockDBPinger{err: errors.New("connection refused")}
	rc := &mockRedisChecker{available: true}

	handler := buildReadyzHandlerFromCheckers(db, rc, nopLog, nil)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("readyz db down: want 503, got %d", rr.Code)
	}

	var body readyzResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("readyz db down: decode body: %v", err)
	}
	if body.Status != "not_ready" {
		t.Errorf("readyz db down: status: want %q, got %q", "not_ready", body.Status)
	}
	if body.DB != "unavailable" {
		t.Errorf("readyz db down: db: want %q, got %q", "unavailable", body.DB)
	}
	if body.Redis != "ok" {
		t.Errorf("readyz db down: redis: want %q, got %q", "ok", body.Redis)
	}
}

// TestReadyz_RedisDown verifies that Redis unavailability causes /readyz to return 503
// with redis="unavailable".
func TestReadyz_RedisDown(t *testing.T) {
	db := &mockDBPinger{err: nil}
	rc := &mockRedisChecker{available: false}

	handler := buildReadyzHandlerFromCheckers(db, rc, nopLog, nil)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("readyz redis down: want 503, got %d", rr.Code)
	}

	var body readyzResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("readyz redis down: decode body: %v", err)
	}
	if body.Status != "not_ready" {
		t.Errorf("readyz redis down: status: want %q, got %q", "not_ready", body.Status)
	}
	if body.DB != "ok" {
		t.Errorf("readyz redis down: db: want %q, got %q", "ok", body.DB)
	}
	if body.Redis != "unavailable" {
		t.Errorf("readyz redis down: redis: want %q, got %q", "unavailable", body.Redis)
	}
}

// TestReadyz_BothDown verifies that when both DB and Redis are unavailable,
// /readyz returns 503 with both fields showing unavailable.
func TestReadyz_BothDown(t *testing.T) {
	db := &mockDBPinger{err: errors.New("timeout")}
	rc := &mockRedisChecker{available: false}

	handler := buildReadyzHandlerFromCheckers(db, rc, nopLog, nil)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("readyz both down: want 503, got %d", rr.Code)
	}

	var body readyzResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("readyz both down: decode body: %v", err)
	}
	if body.DB != "unavailable" {
		t.Errorf("readyz both down: db: want %q, got %q", "unavailable", body.DB)
	}
	if body.Redis != "unavailable" {
		t.Errorf("readyz both down: redis: want %q, got %q", "unavailable", body.Redis)
	}
}

// ── /health tests ─────────────────────────────────────────────────────────────

// TestHealth_AllHealthy verifies that when both DB and Redis are reachable,
// /health returns 200 with status="ok" and db_pool populated.
func TestHealth_AllHealthy(t *testing.T) {
	db := &mockDBChecker{pingErr: nil}
	rc := &mockRedisChecker{available: true}

	handler := buildHealthHandlerFromCheckers(db, rc, nopLog, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("health healthy: want 200, got %d", rr.Code)
	}

	var body healthResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("health healthy: decode body: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("health healthy: status: want %q, got %q", "ok", body.Status)
	}
	if body.DB != "ok" {
		t.Errorf("health healthy: db: want %q, got %q", "ok", body.DB)
	}
	if body.Redis != "ok" {
		t.Errorf("health healthy: redis: want %q, got %q", "ok", body.Redis)
	}
	// db_pool should be present when DB ping succeeds.
	if body.DBPool == nil {
		t.Error("health healthy: db_pool should be present when DB is reachable")
	}
}

// TestHealth_DBUnavailable verifies that a DB ping failure causes /health to return 503
// with status="degraded" and no db_pool field.
func TestHealth_DBUnavailable(t *testing.T) {
	db := &mockDBChecker{pingErr: errors.New("dial error")}
	rc := &mockRedisChecker{available: true}

	handler := buildHealthHandlerFromCheckers(db, rc, nopLog, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("health db down: want 503, got %d", rr.Code)
	}

	var body healthResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("health db down: decode body: %v", err)
	}
	if body.Status != "degraded" {
		t.Errorf("health db down: status: want %q, got %q", "degraded", body.Status)
	}
	if body.DB != "unavailable" {
		t.Errorf("health db down: db: want %q, got %q", "unavailable", body.DB)
	}
	if body.Redis != "ok" {
		t.Errorf("health db down: redis: want %q, got %q", "ok", body.Redis)
	}
	// db_pool must be absent when DB ping fails (stats would be misleading).
	if body.DBPool != nil {
		t.Error("health db down: db_pool should be absent when DB ping fails")
	}
}

// TestHealth_RedisUnavailable verifies that Redis unavailability causes /health to
// return 503 with status="degraded" and redis="unavailable".
func TestHealth_RedisUnavailable(t *testing.T) {
	db := &mockDBChecker{pingErr: nil}
	rc := &mockRedisChecker{available: false}

	handler := buildHealthHandlerFromCheckers(db, rc, nopLog, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("health redis down: want 503, got %d", rr.Code)
	}

	var body healthResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("health redis down: decode body: %v", err)
	}
	if body.Status != "degraded" {
		t.Errorf("health redis down: status: want %q, got %q", "degraded", body.Status)
	}
	if body.DB != "ok" {
		t.Errorf("health redis down: db: want %q, got %q", "ok", body.DB)
	}
	if body.Redis != "unavailable" {
		t.Errorf("health redis down: redis: want %q, got %q", "unavailable", body.Redis)
	}
}

// TestHealth_BothUnavailable verifies that when both dependencies are down,
// /health returns 503 with both fields showing unavailable.
func TestHealth_BothUnavailable(t *testing.T) {
	db := &mockDBChecker{pingErr: errors.New("connection refused")}
	rc := &mockRedisChecker{available: false}

	handler := buildHealthHandlerFromCheckers(db, rc, nopLog, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("health both down: want 503, got %d", rr.Code)
	}

	var body healthResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("health both down: decode body: %v", err)
	}
	if body.Status != "degraded" {
		t.Errorf("health both down: status: want %q, got %q", "degraded", body.Status)
	}
	if body.DB != "unavailable" {
		t.Errorf("health both down: db: want %q, got %q", "unavailable", body.DB)
	}
	if body.Redis != "unavailable" {
		t.Errorf("health both down: redis: want %q, got %q", "unavailable", body.Redis)
	}
}

// TestHealth_NoSensitiveData verifies that health responses never contain sensitive
// fields such as database DSN, passwords, or token values.
func TestHealth_NoSensitiveData(t *testing.T) {
	db := &mockDBChecker{pingErr: nil}
	rc := &mockRedisChecker{available: true}

	handler := buildHealthHandlerFromCheckers(db, rc, nopLog, nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	body := rr.Body.String()
	for _, forbidden := range []string{"password", "secret", "token", "dsn", "postgres://"} {
		if containsCI(body, forbidden) {
			t.Errorf("health: response contains forbidden term %q: %s", forbidden, body)
		}
	}
}

// ── Security headers ──────────────────────────────────────────────────────────
//
// Note: The SecurityHeaders middleware is wired into the chi router in main.go.
// The handler tests below call the handler functions directly (not through the
// full router middleware stack), so security headers are not exercised here.
// Security header presence on real HTTP responses is verified separately in
// internal/platform/middleware/security_headers_test.go.

// ── helpers ───────────────────────────────────────────────────────────────────

// containsCI reports whether s contains substr (case-insensitive).
func containsCI(s, substr string) bool {
	sLow := toLower(s)
	subLow := toLower(substr)
	return len(subLow) > 0 && len(sLow) >= len(subLow) && contains(sLow, subLow)
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
