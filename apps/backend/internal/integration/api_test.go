//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"go.uber.org/zap/zapcore"
)

// ---------------------------------------------------------------------------
// Authentication flow: register → login → GET /me
// ---------------------------------------------------------------------------

// TestAPI_AuthFlow_RegisterLoginGetMe verifies the complete authentication flow:
//
//	POST /api/v1/auth/register → 201
//	POST /api/v1/auth/login    → 200
//	GET  /api/v1/me            → 200 (authenticated)
//
// This exercises the full path: HTTP → middleware → handler → service → PostgreSQL.
func TestAPI_AuthFlow_RegisterLoginGetMe(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildTestAPIServer(t, pool, redisClient, nil)

	// ── 1. Register ──────────────────────────────────────────────────────────
	suffix := uniqueSuffix(t)
	handle := "u" + suffix[:15]
	email := "apime_" + suffix[:16] + "@example.com"
	password := "Password123!"

	regBody := fmt.Sprintf(
		`{"handle":%q,"display_name":"Me Test","email":%q,"password":%q}`,
		handle, email, password,
	)
	regResp := doJSON(t, http.MethodPost, srv.url("/api/v1/auth/register"),
		strings.NewReader(regBody), nil)
	defer regResp.Body.Close()

	if regResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(regResp.Body)
		t.Fatalf("register: want 201, got %d: %s", regResp.StatusCode, body)
	}

	// ── 2. Login ─────────────────────────────────────────────────────────────
	loginBody := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
	loginResp := doJSON(t, http.MethodPost, srv.url("/api/v1/auth/login"),
		strings.NewReader(loginBody), nil)
	defer loginResp.Body.Close()

	if loginResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(loginResp.Body)
		t.Fatalf("login: want 200, got %d: %s", loginResp.StatusCode, body)
	}
	tokens := parseTokenPair(t, loginResp.Body)

	// ── 3. GET /me with access token ─────────────────────────────────────────
	meResp := doJSON(t, http.MethodGet, srv.url("/api/v1/me"),
		nil, map[string]string{"Authorization": "Bearer " + tokens.AccessToken})
	defer meResp.Body.Close()

	if meResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(meResp.Body)
		t.Fatalf("GET /me: want 200, got %d: %s", meResp.StatusCode, body)
	}

	// Verify the profile contains the registered handle and email.
	var profile struct {
		Handle string `json:"handle"`
		Email  string `json:"email"`
	}
	if err := json.NewDecoder(meResp.Body).Decode(&profile); err != nil {
		t.Fatalf("GET /me: decode body: %v", err)
	}
	if profile.Handle != handle {
		t.Errorf("GET /me: handle = %q, want %q", profile.Handle, handle)
	}
	if profile.Email != email {
		t.Errorf("GET /me: email = %q, want %q", profile.Email, email)
	}
}

// ---------------------------------------------------------------------------
// Session / logout flow
// ---------------------------------------------------------------------------

// TestAPI_AuthFlow_Logout_InvalidatesRefresh verifies that after logout, the
// refresh token from the same session is no longer accepted.
//
// This specifically guards the Phase 8A-3 single-session logout change: only
// the specific session identified by the "sid" claim is revoked.
func TestAPI_AuthFlow_Logout_InvalidatesRefresh(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildTestAPIServer(t, pool, redisClient, nil)

	// Register and capture session tokens.
	tokens := registerTestUser(t, srv)

	// Logout session.
	logoutResp := doJSON(t, http.MethodPost, srv.url("/api/v1/auth/logout"),
		nil, map[string]string{"Authorization": "Bearer " + tokens.AccessToken})
	defer logoutResp.Body.Close()

	if logoutResp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(logoutResp.Body)
		t.Fatalf("logout: want 204, got %d: %s", logoutResp.StatusCode, body)
	}

	// After logout, the refresh token must no longer be accepted: the session row
	// was deleted, so GetSessionByHash will return ErrNotFound → 401.
	refreshResp := doJSON(t, http.MethodPost, srv.url("/api/v1/auth/refresh"),
		strings.NewReader(fmt.Sprintf(`{"refresh_token":%q}`, tokens.RefreshToken)),
		nil)
	defer refreshResp.Body.Close()

	if refreshResp.StatusCode == http.StatusOK {
		t.Fatal("refresh after logout should fail (session deleted), got 200")
	}
	// Must return a client error (4xx), not a server error.
	if refreshResp.StatusCode >= 500 {
		body, _ := io.ReadAll(refreshResp.Body)
		t.Fatalf("refresh after logout returned 5xx: %d: %s", refreshResp.StatusCode, body)
	}
}

// ---------------------------------------------------------------------------
// Refresh flow
// ---------------------------------------------------------------------------

// TestAPI_AuthFlow_Refresh_ValidToken verifies that a valid refresh token
// produces a new token pair with HTTP 200.
func TestAPI_AuthFlow_Refresh_ValidToken(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildTestAPIServer(t, pool, redisClient, nil)

	tokens := registerTestUser(t, srv)

	refreshResp := doJSON(t, http.MethodPost, srv.url("/api/v1/auth/refresh"),
		strings.NewReader(fmt.Sprintf(`{"refresh_token":%q}`, tokens.RefreshToken)),
		nil)
	defer refreshResp.Body.Close()

	if refreshResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(refreshResp.Body)
		t.Fatalf("refresh: want 200, got %d: %s", refreshResp.StatusCode, body)
	}

	newTokens := parseTokenPair(t, refreshResp.Body)

	// New access token must work for authenticated requests.
	meResp := doJSON(t, http.MethodGet, srv.url("/api/v1/me"),
		nil, map[string]string{"Authorization": "Bearer " + newTokens.AccessToken})
	defer meResp.Body.Close()

	if meResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(meResp.Body)
		t.Fatalf("GET /me with refreshed token: want 200, got %d: %s", meResp.StatusCode, body)
	}
}

// TestAPI_AuthFlow_Refresh_InvalidToken verifies that an invalid/fabricated
// refresh token returns a non-2xx response with a structured error body.
func TestAPI_AuthFlow_Refresh_InvalidToken(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildTestAPIServer(t, pool, redisClient, nil)

	refreshResp := doJSON(t, http.MethodPost, srv.url("/api/v1/auth/refresh"),
		strings.NewReader(`{"refresh_token":"this-is-not-a-valid-refresh-token"}`),
		nil)
	defer refreshResp.Body.Close()

	if refreshResp.StatusCode == http.StatusOK {
		t.Fatal("refresh with invalid token should fail, got 200")
	}
	if refreshResp.StatusCode >= 500 {
		body, _ := io.ReadAll(refreshResp.Body)
		t.Fatalf("refresh with invalid token returned 5xx: %d: %s", refreshResp.StatusCode, body)
	}

	code := apiErrorCode(t, refreshResp.Body)
	if code == "" {
		t.Error("expected non-empty error code in refresh failure response")
	}
}

// ---------------------------------------------------------------------------
// Refresh rate limit (live Redis)
// ---------------------------------------------------------------------------

// TestAPI_RateLimit_RefreshExhausted_Returns429 exercises the real Redis-backed
// rate-limit middleware on the refresh endpoint.
//
// Rate limit: 20 requests / 15 minutes / client IP (auth/handler.go).
//
// Strategy: use X-Real-IP with a unique test IP so the rate-limit key is
// isolated to this test run. Send 20 requests with invalid tokens (they pass
// the rate-limit check and fail at the handler), then verify the 21st is
// rejected with HTTP 429 and Retry-After.
func TestAPI_RateLimit_RefreshExhausted_Returns429(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildTestAPIServer(t, pool, redisClient, nil)

	// Unique test IP — isolates rate-limit state from other tests.
	// chimw.RealIP rewrites r.RemoteAddr from X-Real-IP, which the
	// RateLimitMiddleware's realIP() function reads.
	testIP := "10.99." + uniqueSuffix(t)[:7]
	headers := map[string]string{"X-Real-IP": testIP}

	// Consume all 20 slots using invalid tokens.
	// The rate limit counter increments before the handler executes, so
	// invalid-token requests still consume quota.
	for i := 1; i <= 20; i++ {
		resp := doJSON(t, http.MethodPost, srv.url("/api/v1/auth/refresh"),
			strings.NewReader(`{"refresh_token":"invalid-token-rl-test"}`),
			headers)
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			t.Fatalf("request %d/20 was rate-limited prematurely (429)", i)
		}
	}

	// The 21st request must be rate-limited.
	resp21 := doJSON(t, http.MethodPost, srv.url("/api/v1/auth/refresh"),
		strings.NewReader(`{"refresh_token":"invalid-token-rl-test"}`),
		headers)
	defer resp21.Body.Close()

	if resp21.StatusCode != http.StatusTooManyRequests {
		body, _ := io.ReadAll(resp21.Body)
		t.Fatalf("request 21: want 429, got %d: %s", resp21.StatusCode, body)
	}

	// Retry-After header must be present.
	if resp21.Header.Get("Retry-After") == "" {
		t.Error("request 21: Retry-After header is absent on 429 response")
	}

	// Error code must be the apierror rate-limit code.
	code := apiErrorCode(t, resp21.Body)
	if code != "RATE_LIMITED" {
		t.Errorf("request 21: error code = %q, want RATE_LIMITED", code)
	}
}

// ---------------------------------------------------------------------------
// Health endpoints
// ---------------------------------------------------------------------------

// TestAPI_Health_LivezOK verifies that GET /livez returns 200 with status="ok".
func TestAPI_Health_LivezOK(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildTestAPIServer(t, pool, redisClient, nil)

	resp := doJSON(t, http.MethodGet, srv.url("/livez"), nil, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("livez: want 200, got %d", resp.StatusCode)
	}

	var body testLivezResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("livez: decode: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("livez: status = %q, want %q", body.Status, "ok")
	}
}

// TestAPI_Health_ReadyzOK verifies that GET /readyz returns "ready" when both
// PostgreSQL and Redis are healthy.
func TestAPI_Health_ReadyzOK(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildTestAPIServer(t, pool, redisClient, nil)

	resp := doJSON(t, http.MethodGet, srv.url("/readyz"), nil, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("readyz: want 200, got %d: %s", resp.StatusCode, body)
	}

	var body testReadyzResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("readyz: decode: %v", err)
	}
	if body.Status != "ready" {
		t.Errorf("readyz: status = %q, want %q", body.Status, "ready")
	}
}

// TestAPI_Health_HealthOK verifies that GET /health returns status="ok" and
// includes db/redis fields when both dependencies are healthy.
func TestAPI_Health_HealthOK(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildTestAPIServer(t, pool, redisClient, nil)

	resp := doJSON(t, http.MethodGet, srv.url("/health"), nil, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("health: want 200, got %d: %s", resp.StatusCode, body)
	}

	var body testHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("health: decode: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("health: status = %q, want %q", body.Status, "ok")
	}
	if body.DB != "ok" {
		t.Errorf("health: db = %q, want %q", body.DB, "ok")
	}
	if body.Redis != "ok" {
		t.Errorf("health: redis = %q, want %q", body.Redis, "ok")
	}
}

// ---------------------------------------------------------------------------
// Request ID middleware
// ---------------------------------------------------------------------------

// TestAPI_RequestID_PresentInResponseHeader verifies that every response
// includes an X-Request-Id header set by the chi RequestID middleware.
func TestAPI_RequestID_PresentInResponseHeader(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildTestAPIServer(t, pool, redisClient, nil)

	resp := doJSON(t, http.MethodGet, srv.url("/livez"), nil, nil)
	defer resp.Body.Close()

	if resp.Header.Get("X-Request-Id") == "" {
		t.Error("X-Request-Id header is absent — RequestID middleware may not be wired")
	}
}

// TestAPI_RequestID_SuppliedHeaderEchoed verifies that when the client provides
// X-Request-Id, the AccessLog middleware echoes it back in the response.
func TestAPI_RequestID_SuppliedHeaderEchoed(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildTestAPIServer(t, pool, redisClient, nil)

	const clientReqID = "test-client-request-id-12345"
	resp := doJSON(t, http.MethodGet, srv.url("/livez"),
		nil, map[string]string{"X-Request-Id": clientReqID})
	defer resp.Body.Close()

	got := resp.Header.Get("X-Request-Id")
	if got != clientReqID {
		t.Errorf("X-Request-Id: got %q, want %q", got, clientReqID)
	}
}

// ---------------------------------------------------------------------------
// JWT failure logging
// ---------------------------------------------------------------------------

// TestAPI_JWTFailure_Returns401AndLogsWarn verifies that a request with a
// malformed JWT to an authenticated endpoint:
//   - returns HTTP 401
//   - emits a WARN log entry with message "jwt authentication failure"
//   - does NOT log the raw JWT or Authorization header value
//
// This exercises the Phase 8A-5 JWTMiddleware at the HTTP boundary.
func TestAPI_JWTFailure_Returns401AndLogsWarn(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildTestAPIServer(t, pool, redisClient, nil)

	const malformedToken = "this.is.not.a.valid.jwt"
	resp := doJSON(t, http.MethodGet, srv.url("/api/v1/me"),
		nil, map[string]string{"Authorization": "Bearer " + malformedToken})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}

	// Find the WARN log entry from JWTMiddleware.
	var jwtWarnFound bool
	for _, entry := range srv.logs.All() {
		if entry.Level == zapcore.WarnLevel && entry.Message == "jwt authentication failure" {
			jwtWarnFound = true

			// Raw token must not appear in any log field.
			for k, v := range entry.ContextMap() {
				valStr := fmt.Sprint(v)
				if strings.Contains(valStr, malformedToken) {
					t.Errorf("log field %q contains raw token — tokens must not be logged", k)
				}
				if strings.Contains(valStr, "Bearer") {
					t.Errorf("log field %q contains 'Bearer' — Authorization header must not be logged", k)
				}
			}
			break
		}
	}
	if !jwtWarnFound {
		t.Fatal("expected WARN log 'jwt authentication failure' — not found in captured logs")
	}
}

// ---------------------------------------------------------------------------
// CORS
// ---------------------------------------------------------------------------

// TestAPI_CORS_TestEnv_AllowsAllOrigins verifies that the test-environment
// CORS policy adds Access-Control-Allow-Origin: * to responses.
func TestAPI_CORS_TestEnv_AllowsAllOrigins(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	// nil corsOrigins → test environment → corsAllowAllMW.
	srv := buildTestAPIServer(t, pool, redisClient, nil)

	resp := doJSON(t, http.MethodGet, srv.url("/livez"),
		nil, map[string]string{"Origin": "https://some.origin.example.com"})
	defer resp.Body.Close()

	got := resp.Header.Get("Access-Control-Allow-Origin")
	if got != "*" {
		t.Errorf("CORS allow-all: Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
}

// TestAPI_CORS_AllowList_AllowedOrigin verifies that a request from an allowed
// origin receives the correct Access-Control-Allow-Origin header.
func TestAPI_CORS_AllowList_AllowedOrigin(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	allowedOrigin := "https://app.dzeroth.example.com"
	srv := buildTestAPIServer(t, pool, redisClient, []string{allowedOrigin})

	resp := doJSON(t, http.MethodGet, srv.url("/livez"),
		nil, map[string]string{"Origin": allowedOrigin})
	defer resp.Body.Close()

	got := resp.Header.Get("Access-Control-Allow-Origin")
	if got != allowedOrigin {
		t.Errorf("CORS allowlist: Access-Control-Allow-Origin = %q, want %q", got, allowedOrigin)
	}
	if got == "*" {
		t.Error("CORS allowlist must never emit wildcard Access-Control-Allow-Origin")
	}
}

// TestAPI_CORS_AllowList_DisallowedOrigin verifies that a request from an
// origin not in the allowlist does not receive an ACAO header.
func TestAPI_CORS_AllowList_DisallowedOrigin(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildTestAPIServer(t, pool, redisClient, []string{"https://app.dzeroth.example.com"})

	resp := doJSON(t, http.MethodGet, srv.url("/livez"),
		nil, map[string]string{"Origin": "https://attacker.example.com"})
	defer resp.Body.Close()

	got := resp.Header.Get("Access-Control-Allow-Origin")
	if got != "" {
		t.Errorf("CORS allowlist: disallowed origin should get no ACAO header, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Error response format
// ---------------------------------------------------------------------------

// TestAPI_ErrorFormat_Unauthorized verifies that unauthenticated requests to
// protected endpoints return HTTP 401 in the apierror envelope format.
func TestAPI_ErrorFormat_Unauthorized(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildTestAPIServer(t, pool, redisClient, nil)

	resp := doJSON(t, http.MethodGet, srv.url("/api/v1/me"), nil, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
	code := apiErrorCode(t, resp.Body)
	if code != "UNAUTHORIZED" {
		t.Errorf("error code = %q, want UNAUTHORIZED", code)
	}
}

// TestAPI_ErrorFormat_InvalidJSON verifies that a malformed JSON body returns
// HTTP 400 in the apierror envelope format.
func TestAPI_ErrorFormat_InvalidJSON(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildTestAPIServer(t, pool, redisClient, nil)

	resp := doJSON(t, http.MethodPost, srv.url("/api/v1/auth/register"),
		strings.NewReader("this is not json"), nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("want 400, got %d", resp.StatusCode)
	}
	code := apiErrorCode(t, resp.Body)
	if code == "" {
		t.Error("expected a non-empty error code in 400 response body")
	}
}

// TestAPI_ErrorFormat_InvalidCredentials verifies that login with wrong
// credentials returns HTTP 401 in the apierror envelope format.
func TestAPI_ErrorFormat_InvalidCredentials(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildTestAPIServer(t, pool, redisClient, nil)

	resp := doJSON(t, http.MethodPost, srv.url("/api/v1/auth/login"),
		strings.NewReader(`{"email":"nonexistent@example.com","password":"WrongPassword1!"}`),
		nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
	code := apiErrorCode(t, resp.Body)
	if code != "UNAUTHORIZED" {
		t.Errorf("error code = %q, want UNAUTHORIZED", code)
	}
}

// ---------------------------------------------------------------------------
// PostgreSQL + HTTP integration
// ---------------------------------------------------------------------------

// TestAPI_PostgreSQL_GetMe_ReturnsPersistedData verifies the complete chain:
//
//	HTTP → JWT middleware → user handler → user.Service → PostgreSQL → response
//
// After registration, GET /me must return the profile stored in PostgreSQL.
func TestAPI_PostgreSQL_GetMe_ReturnsPersistedData(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildTestAPIServer(t, pool, redisClient, nil)

	suffix := uniqueSuffix(t)
	handle := "u" + suffix[:15]
	email := "pgme_" + suffix[:16] + "@example.com"
	displayName := "PG Me Test"
	password := "Password123!"

	regBody := fmt.Sprintf(
		`{"handle":%q,"display_name":%q,"email":%q,"password":%q}`,
		handle, displayName, email, password,
	)
	regResp := doJSON(t, http.MethodPost, srv.url("/api/v1/auth/register"),
		strings.NewReader(regBody), nil)
	defer regResp.Body.Close()
	tokens := parseTokenPair(t, regResp.Body)

	meResp := doJSON(t, http.MethodGet, srv.url("/api/v1/me"),
		nil, map[string]string{"Authorization": "Bearer " + tokens.AccessToken})
	defer meResp.Body.Close()

	if meResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(meResp.Body)
		t.Fatalf("GET /me: want 200, got %d: %s", meResp.StatusCode, body)
	}

	var profile struct {
		Handle      string `json:"handle"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
	}
	if err := json.NewDecoder(meResp.Body).Decode(&profile); err != nil {
		t.Fatalf("GET /me: decode: %v", err)
	}
	if profile.Handle != handle {
		t.Errorf("handle = %q, want %q", profile.Handle, handle)
	}
	if profile.DisplayName != displayName {
		t.Errorf("display_name = %q, want %q", profile.DisplayName, displayName)
	}
	if profile.Email != email {
		t.Errorf("email = %q, want %q", profile.Email, email)
	}
}

// ---------------------------------------------------------------------------
// Redis + HTTP integration
// ---------------------------------------------------------------------------

// TestAPI_Redis_RateLimitStateCreated verifies that after a refresh request,
// a real Redis key exists with a counter for the rate-limit IP.
// This proves Redis is genuinely involved in the refresh middleware flow.
func TestAPI_Redis_RateLimitStateCreated(t *testing.T) {
	pool := connectTestDB(t)
	redisClient := connectTestRedis(t)
	srv := buildTestAPIServer(t, pool, redisClient, nil)

	testIP := "10.88." + uniqueSuffix(t)[:7]
	headers := map[string]string{"X-Real-IP": testIP}

	// One refresh request — invalid token is fine; rate limit runs first.
	resp := doJSON(t, http.MethodPost, srv.url("/api/v1/auth/refresh"),
		strings.NewReader(`{"refresh_token":"redis-integration-test-token"}`),
		headers)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// The rate-limit key must now exist in Redis with count ≥ 1.
	key := fmt.Sprintf("rl:auth:refresh:%s", testIP)
	val, err := redisClient.Get(context.Background(), key).Int64()
	if err != nil {
		t.Fatalf("Redis key %q: %v — rate-limit state was not created in Redis", key, err)
	}
	if val < 1 {
		t.Errorf("Redis key %q: count = %d, want ≥ 1", key, val)
	}
}
