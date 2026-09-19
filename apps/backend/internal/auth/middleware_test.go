package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// handlerSentinel is an HTTP handler that records whether it was called and
// stores the request context for inspection.
type handlerSentinel struct {
	called bool
	ctx    context.Context
}

func (s *handlerSentinel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.called = true
	s.ctx = r.Context()
	w.WriteHeader(http.StatusOK)
}

// errorCodeFromResponse parses the apierror envelope and returns the code.
func errorCodeFromResponse(t *testing.T, body []byte) string {
	t.Helper()
	var resp apierror.ErrorResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("could not unmarshal error response: %v\nbody: %s", err, body)
	}
	return resp.Error.Code
}

// ---------------------------------------------------------------------------
// JWTMiddleware — valid token
// ---------------------------------------------------------------------------

func TestJWTMiddleware_ValidTokenCallsNextHandler(t *testing.T) {
	secret := testSecret
	userID := uuid.New()

	tokenStr, err := GenerateAccessToken(userID, uuid.New(), secret)
	if err != nil {
		t.Fatalf("GenerateAccessToken error: %v", err)
	}

	sentinel := &handlerSentinel{}
	mw := JWTMiddleware(secret, zap.NewNop())(sentinel)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if !sentinel.called {
		t.Fatal("next handler was not called for a valid token")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestJWTMiddleware_ValidTokenSetsUserIDInContext(t *testing.T) {
	secret := testSecret
	userID := uuid.New()

	tokenStr, err := GenerateAccessToken(userID, uuid.New(), secret)
	if err != nil {
		t.Fatalf("GenerateAccessToken error: %v", err)
	}

	sentinel := &handlerSentinel{}
	mw := JWTMiddleware(secret, zap.NewNop())(sentinel)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if !sentinel.called {
		t.Fatal("handler was not called")
	}

	ctxUserID, ok := UserIDFromContext(sentinel.ctx)
	if !ok {
		t.Fatal("UserIDFromContext returned false — user ID not set in context")
	}
	if ctxUserID != userID {
		t.Errorf("context user ID = %v, want %v", ctxUserID, userID)
	}
}

func TestJWTMiddleware_ValidTokenSetsSessionJTIInContext(t *testing.T) {
	secret := testSecret
	userID := uuid.New()
	sessionID := uuid.New()

	tokenStr, err := GenerateAccessToken(userID, sessionID, secret)
	if err != nil {
		t.Fatalf("GenerateAccessToken error: %v", err)
	}

	// Parse token to extract expected jti.
	claims, err := ValidateAccessToken(tokenStr, secret)
	if err != nil {
		t.Fatalf("ValidateAccessToken error: %v", err)
	}
	expectedJTI := claims.ID

	sentinel := &handlerSentinel{}
	mw := JWTMiddleware(secret, zap.NewNop())(sentinel)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	jti, ok := SessionJTIFromContext(sentinel.ctx)
	if !ok {
		t.Fatal("SessionJTIFromContext returned false")
	}
	if jti != expectedJTI {
		t.Errorf("context jti = %q, want %q", jti, expectedJTI)
	}
}

// ---------------------------------------------------------------------------
// JWTMiddleware — missing / malformed header
// ---------------------------------------------------------------------------

func TestJWTMiddleware_MissingAuthorizationHeader_Returns401(t *testing.T) {
	sentinel := &handlerSentinel{}
	mw := JWTMiddleware(testSecret, zap.NewNop())(sentinel)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if sentinel.called {
		t.Fatal("next handler should not have been called without an Authorization header")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}

	code := errorCodeFromResponse(t, rec.Body.Bytes())
	if code != apierror.CodeUnauthorized {
		t.Errorf("error code = %q, want %q", code, apierror.CodeUnauthorized)
	}
}

func TestJWTMiddleware_MalformedBearerHeader_Returns401(t *testing.T) {
	sentinel := &handlerSentinel{}
	mw := JWTMiddleware(testSecret, zap.NewNop())(sentinel)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	// "Bearer " prefix without an actual token value.
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for malformed Bearer header", rec.Code)
	}
}

func TestJWTMiddleware_NonBearerScheme_Returns401(t *testing.T) {
	sentinel := &handlerSentinel{}
	mw := JWTMiddleware(testSecret, zap.NewNop())(sentinel)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for non-Bearer scheme", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// JWTMiddleware — expired / tampered token
// ---------------------------------------------------------------------------

func TestJWTMiddleware_ExpiredToken_Returns401(t *testing.T) {
	userID := uuid.New()
	jti := uuid.New()
	now := time.Now().UTC()

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now.Add(-30 * time.Minute)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-15 * time.Minute)),
			ID:        jti.String(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(testSecret)
	if err != nil {
		t.Fatalf("signing error: %v", err)
	}

	sentinel := &handlerSentinel{}
	mw := JWTMiddleware(testSecret, zap.NewNop())(sentinel)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if sentinel.called {
		t.Fatal("next handler should not have been called with an expired token")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestJWTMiddleware_TamperedToken_Returns401(t *testing.T) {
	userID := uuid.New()
	tokenStr, err := GenerateAccessToken(userID, uuid.New(), testSecret)
	if err != nil {
		t.Fatalf("GenerateAccessToken error: %v", err)
	}

	// Corrupt the signature (last segment).
	parts := splitJWT(tokenStr)
	if len(parts) != 3 {
		t.Fatalf("unexpected JWT structure")
	}
	parts[2] = "invalidsignatureXXXXXX"
	tampered := parts[0] + "." + parts[1] + "." + parts[2]

	sentinel := &handlerSentinel{}
	mw := JWTMiddleware(testSecret, zap.NewNop())(sentinel)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tampered)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if sentinel.called {
		t.Fatal("next handler should not have been called with a tampered token")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// splitJWT splits a JWT string into its three parts without importing strings
// inside a test that already uses it.
func splitJWT(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// ---------------------------------------------------------------------------
// UserIDFromContext / SessionJTIFromContext — without middleware
// ---------------------------------------------------------------------------

func TestUserIDFromContext_ReturnsFalseWhenNotSet(t *testing.T) {
	ctx := context.Background()
	_, ok := UserIDFromContext(ctx)
	if ok {
		t.Fatal("UserIDFromContext returned true on an empty context")
	}
}

func TestSessionJTIFromContext_ReturnsFalseWhenNotSet(t *testing.T) {
	ctx := context.Background()
	_, ok := SessionJTIFromContext(ctx)
	if ok {
		t.Fatal("SessionJTIFromContext returned true on an empty context")
	}
}

func TestUserIDFromContext_ReturnsCorrectValueWhenSet(t *testing.T) {
	id := uuid.New()
	ctx := context.WithValue(context.Background(), ctxKeyUserID, id)
	got, ok := UserIDFromContext(ctx)
	if !ok {
		t.Fatal("UserIDFromContext returned false when value was explicitly set")
	}
	if got != id {
		t.Errorf("got %v, want %v", got, id)
	}
}

func TestSessionJTIFromContext_ReturnsCorrectValueWhenSet(t *testing.T) {
	jti := "test-jti-value"
	ctx := context.WithValue(context.Background(), ctxKeySessionJTI, jti)
	got, ok := SessionJTIFromContext(ctx)
	if !ok {
		t.Fatal("SessionJTIFromContext returned false when value was explicitly set")
	}
	if got != jti {
		t.Errorf("got %q, want %q", got, jti)
	}
}

// ---------------------------------------------------------------------------
// SessionIDFromContext — without middleware
// ---------------------------------------------------------------------------

func TestSessionIDFromContext_ReturnsFalseWhenNotSet(t *testing.T) {
	ctx := context.Background()
	_, ok := SessionIDFromContext(ctx)
	if ok {
		t.Fatal("SessionIDFromContext returned true on an empty context")
	}
}

func TestSessionIDFromContext_ReturnsFalseForNilUUID(t *testing.T) {
	// A zero UUID (uuid.Nil) must not be treated as a valid session identifier.
	ctx := context.WithValue(context.Background(), ctxKeySessionID, uuid.Nil)
	_, ok := SessionIDFromContext(ctx)
	if ok {
		t.Fatal("SessionIDFromContext returned true for uuid.Nil — zero UUID is not a valid session")
	}
}

func TestSessionIDFromContext_ReturnsCorrectValueWhenSet(t *testing.T) {
	sid := uuid.New()
	ctx := context.WithValue(context.Background(), ctxKeySessionID, sid)
	got, ok := SessionIDFromContext(ctx)
	if !ok {
		t.Fatal("SessionIDFromContext returned false when a non-nil session ID was set")
	}
	if got != sid {
		t.Errorf("got %v, want %v", got, sid)
	}
}

// ---------------------------------------------------------------------------
// JWTMiddleware — session ID propagation
// ---------------------------------------------------------------------------

func TestJWTMiddleware_ValidTokenSetsSessionIDInContext(t *testing.T) {
	secret := testSecret
	userID := uuid.New()
	sessionID := uuid.New()

	tokenStr, err := GenerateAccessToken(userID, sessionID, secret)
	if err != nil {
		t.Fatalf("GenerateAccessToken error: %v", err)
	}

	sentinel := &handlerSentinel{}
	mw := JWTMiddleware(secret, zap.NewNop())(sentinel)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if !sentinel.called {
		t.Fatal("next handler was not called for a valid token")
	}

	got, ok := SessionIDFromContext(sentinel.ctx)
	if !ok {
		t.Fatal("SessionIDFromContext returned false — session ID not propagated by JWTMiddleware")
	}
	if got != sessionID {
		t.Errorf("context session ID = %v, want %v", got, sessionID)
	}
}

func TestJWTMiddleware_TokenWithNilSessionID_SessionIDFromContextReturnsFalse(t *testing.T) {
	// A token crafted with SessionID == uuid.Nil (simulates old tokens without sid claim)
	// must cause SessionIDFromContext to return false after passing through JWTMiddleware.
	secret := testSecret
	userID := uuid.New()

	// Generate with uuid.Nil as the session ID to simulate a legacy token.
	tokenStr, err := GenerateAccessToken(userID, uuid.Nil, secret)
	if err != nil {
		t.Fatalf("GenerateAccessToken error: %v", err)
	}

	sentinel := &handlerSentinel{}
	mw := JWTMiddleware(secret, zap.NewNop())(sentinel)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if !sentinel.called {
		t.Fatal("next handler was not called for a valid token")
	}

	_, ok := SessionIDFromContext(sentinel.ctx)
	if ok {
		t.Fatal("SessionIDFromContext must return false when token carries uuid.Nil as session ID")
	}
}

// ---------------------------------------------------------------------------
// RateLimitMiddleware — fail-closed unit test (no integration tag)
// The Redis-unavailable path must return 503 to fail closed.
// This test runs WITHOUT the integration build tag so it always runs in CI.
// ---------------------------------------------------------------------------

func TestRateLimitMiddleware_NilRedisClient_FailsClosed503(t *testing.T) {
	log, _ := zap.NewDevelopment()
	cfg := RateLimitConfig{
		Operation:   "login",
		MaxAttempts: 10,
		Window:      15 * time.Minute,
	}

	sentinel := &handlerSentinel{}
	// Passing nil as the Redis client simulates Redis being unavailable.
	// events is nil — counter instrumentation is a no-op in this test.
	mw := RateLimitMiddleware(nil, cfg, log, nil)(sentinel)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if sentinel.called {
		t.Fatal("next handler must not be called when Redis is nil (fail-closed)")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 (fail-closed on nil Redis)", rec.Code)
	}

	code := errorCodeFromResponse(t, rec.Body.Bytes())
	if code != apierror.CodeServiceUnavailable {
		t.Errorf("error code = %q, want %q", code, apierror.CodeServiceUnavailable)
	}
}

// ---------------------------------------------------------------------------
// JWTMiddleware — structured WARN logging for authentication failures
// ---------------------------------------------------------------------------
//
// These tests use zaptest/observer to capture log output without real I/O.
// zaptest/observer ships inside go.uber.org/zap — no new dependency required.

// TestJWTMiddleware_MalformedToken_EmitsWarn verifies that a completely
// malformed token (not a valid JWT structure) causes JWTMiddleware to return
// 401 and emit exactly one WARN log with msg "jwt authentication failure".
func TestJWTMiddleware_MalformedToken_EmitsWarn(t *testing.T) {
	log, logs := newObservedLogger(zapcore.WarnLevel)

	sentinel := &handlerSentinel{}
	mw := JWTMiddleware(testSecret, log)(sentinel)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer this.is.not.a.jwt")
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if sentinel.called {
		t.Fatal("next handler must not be called for malformed token")
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 WARN log entry, got %d", len(entries))
	}
	if entries[0].Level != zapcore.WarnLevel {
		t.Errorf("log level = %v, want WARN", entries[0].Level)
	}
	if entries[0].Message != "jwt authentication failure" {
		t.Errorf("log message = %q, want %q", entries[0].Message, "jwt authentication failure")
	}

	reasonField := entries[0].ContextMap()["reason"]
	if reasonField == "" {
		t.Error("WARN log must include a non-empty 'reason' field")
	}
}

// TestJWTMiddleware_InvalidSignature_EmitsWarn verifies that a token signed
// with a different secret emits a WARN log with reason "invalid_signature".
func TestJWTMiddleware_InvalidSignature_EmitsWarn(t *testing.T) {
	log, logs := newObservedLogger(zapcore.WarnLevel)

	// Generate with a different secret so ValidateAccessToken will reject it.
	differentSecret := []byte("different-secret-that-is-at-least-32-bytes!")
	tokenStr, err := GenerateAccessToken(uuid.New(), uuid.New(), differentSecret)
	if err != nil {
		t.Fatalf("GenerateAccessToken error: %v", err)
	}

	sentinel := &handlerSentinel{}
	mw := JWTMiddleware(testSecret, log)(sentinel)

	req := httptest.NewRequest(http.MethodPost, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 WARN log entry, got %d", len(entries))
	}
	if entries[0].Level != zapcore.WarnLevel {
		t.Errorf("log level = %v, want WARN", entries[0].Level)
	}
	if entries[0].Message != "jwt authentication failure" {
		t.Errorf("log message = %q, want %q", entries[0].Message, "jwt authentication failure")
	}

	reason := entries[0].ContextMap()["reason"]
	if reason != "invalid_signature" {
		t.Errorf("reason field = %q, want %q", reason, "invalid_signature")
	}
}

// TestJWTMiddleware_ExpiredToken_EmitsWarn verifies that an expired token
// emits a WARN log with reason "expired".
func TestJWTMiddleware_ExpiredToken_EmitsWarn(t *testing.T) {
	log, logs := newObservedLogger(zapcore.WarnLevel)

	userID := uuid.New()
	now := time.Now().UTC()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now.Add(-30 * time.Minute)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-15 * time.Minute)),
			ID:        uuid.New().String(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(testSecret)
	if err != nil {
		t.Fatalf("signing error: %v", err)
	}

	sentinel := &handlerSentinel{}
	mw := JWTMiddleware(testSecret, log)(sentinel)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 WARN log entry, got %d", len(entries))
	}
	if entries[0].Level != zapcore.WarnLevel {
		t.Errorf("log level = %v, want WARN", entries[0].Level)
	}

	reason := entries[0].ContextMap()["reason"]
	if reason != "expired" {
		t.Errorf("reason field = %q, want %q", reason, "expired")
	}
}

// TestJWTMiddleware_ValidToken_NoWarn verifies that a valid token does NOT
// emit any WARN log — the happy path must remain silent.
func TestJWTMiddleware_ValidToken_NoWarn(t *testing.T) {
	log, logs := newObservedLogger(zapcore.WarnLevel)

	tokenStr, err := GenerateAccessToken(uuid.New(), uuid.New(), testSecret)
	if err != nil {
		t.Fatalf("GenerateAccessToken error: %v", err)
	}

	sentinel := &handlerSentinel{}
	mw := JWTMiddleware(testSecret, log)(sentinel)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if !sentinel.called {
		t.Fatal("next handler should have been called for valid token")
	}
	if logs.Len() != 0 {
		t.Errorf("expected 0 WARN log entries for valid token, got %d", logs.Len())
	}
}

// TestJWTMiddleware_NoAuthorizationHeader_NoWarn verifies that a missing
// Authorization header returns 401 but does NOT emit a WARN log.
// A missing header is not a forgery attempt — it is a common client error.
func TestJWTMiddleware_NoAuthorizationHeader_NoWarn(t *testing.T) {
	log, logs := newObservedLogger(zapcore.WarnLevel)

	sentinel := &handlerSentinel{}
	mw := JWTMiddleware(testSecret, log)(sentinel)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	// No Authorization header.
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if logs.Len() != 0 {
		t.Errorf("expected 0 WARN log entries for missing header, got %d", logs.Len())
	}
}

// TestJWTMiddleware_FailureLog_NoRawToken verifies that the WARN log fields
// do NOT contain the raw Authorization header value or any substring of the
// JWT token string. This is a security invariant: the raw token must never
// appear in logs.
func TestJWTMiddleware_FailureLog_NoRawToken(t *testing.T) {
	log, logs := newObservedLogger(zapcore.WarnLevel)

	// Generate a token with a recognizable prefix substring.
	tokenStr, err := GenerateAccessToken(uuid.New(), uuid.New(), testSecret)
	if err != nil {
		t.Fatalf("GenerateAccessToken error: %v", err)
	}

	// Tamper the signature so the middleware rejects it — this exercises the
	// WARN log path while keeping a valid, recognizable JWT structure.
	parts := splitJWT(tokenStr)
	if len(parts) != 3 {
		t.Fatalf("unexpected JWT structure")
	}
	parts[2] = "invalidsignatureXXXXXX"
	tampered := parts[0] + "." + parts[1] + "." + parts[2]

	sentinel := &handlerSentinel{}
	mw := JWTMiddleware(testSecret, log)(sentinel)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tampered)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 WARN log entry, got %d", len(entries))
	}

	// Reconstruct the full log entry as a string for substring search.
	entry := entries[0]
	logStr := entry.Message
	for k, v := range entry.ContextMap() {
		logStr += " " + k + "=" + strings.Join(toStringSlice(v), ",")
	}

	// The raw token (or any of its dot-separated segments) must not appear.
	for i, part := range parts {
		if strings.Contains(logStr, part) {
			t.Errorf("WARN log contains JWT segment[%d] (first 10 chars: %q) — raw token must never be logged", i, truncate(part, 10))
		}
	}
	if strings.Contains(logStr, "Bearer") {
		t.Error("WARN log contains 'Bearer' scheme value — Authorization header must never be logged")
	}
}

// TestJWTMiddleware_FailureLog_HasRequestID verifies that when the chi
// RequestID middleware has placed a request ID in the context, the WARN log
// includes a "request_id" field.
func TestJWTMiddleware_FailureLog_HasRequestID(t *testing.T) {
	log, logs := newObservedLogger(zapcore.WarnLevel)

	sentinel := &handlerSentinel{}
	mw := JWTMiddleware(testSecret, log)(sentinel)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer this.is.not.a.jwt")

	// Inject a chi request ID into the context using the public chimw.RequestIDKey.
	// chimw.GetReqID (used by ctxlog.RequestIDField) reads from this key.
	const testRequestID = "test-request-id-12345"
	ctx := context.WithValue(req.Context(), chimw.RequestIDKey, testRequestID)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 WARN log entry, got %d", len(entries))
	}

	requestIDField, ok := entries[0].ContextMap()["request_id"]
	if !ok {
		t.Fatal("WARN log must include 'request_id' field when request ID is present in context")
	}
	if requestIDField != testRequestID {
		t.Errorf("request_id field = %q, want %q", requestIDField, testRequestID)
	}
}

// ---------------------------------------------------------------------------
// test helpers for WARN log tests
// ---------------------------------------------------------------------------

// toStringSlice converts an interface{} log field value to a string slice
// for substring matching. Only handles common zap field value types.
func toStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	return []string{fmt.Sprint(v)}
}

// truncate returns up to n runes of s, for safe log output in test failures.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
