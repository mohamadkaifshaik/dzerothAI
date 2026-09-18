package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

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

	tokenStr, err := GenerateAccessToken(userID, secret)
	if err != nil {
		t.Fatalf("GenerateAccessToken error: %v", err)
	}

	sentinel := &handlerSentinel{}
	mw := JWTMiddleware(secret)(sentinel)

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

	tokenStr, err := GenerateAccessToken(userID, secret)
	if err != nil {
		t.Fatalf("GenerateAccessToken error: %v", err)
	}

	sentinel := &handlerSentinel{}
	mw := JWTMiddleware(secret)(sentinel)

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

	tokenStr, err := GenerateAccessToken(userID, secret)
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
	mw := JWTMiddleware(secret)(sentinel)

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
	mw := JWTMiddleware(testSecret)(sentinel)

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
	mw := JWTMiddleware(testSecret)(sentinel)

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
	mw := JWTMiddleware(testSecret)(sentinel)

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
	mw := JWTMiddleware(testSecret)(sentinel)

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
	tokenStr, err := GenerateAccessToken(userID, testSecret)
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
	mw := JWTMiddleware(testSecret)(sentinel)

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
	mw := RateLimitMiddleware(nil, cfg, log)(sentinel)

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
