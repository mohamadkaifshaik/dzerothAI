// Package main tests for the corsAllowList middleware.
// These tests use net/http/httptest — no external dependencies required.
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

// noopHandler is a trivial http.Handler that returns 200 OK.
var noopHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// newTestMiddleware builds the corsAllowList middleware with a no-op zap logger.
func newTestMiddleware(origins []string) func(http.Handler) http.Handler {
	return corsAllowList(origins, zap.NewNop())
}

// TestCORSAllowList_AllowedOrigin verifies that a request from a configured origin
// receives the correct CORS response headers.
func TestCORSAllowList_AllowedOrigin(t *testing.T) {
	mw := newTestMiddleware([]string{"https://app.example.com"})
	handler := mw(noopHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("Access-Control-Allow-Origin: got %q, want %q", got, "https://app.example.com")
	}
	if got := rr.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary: got %q, want %q", got, "Origin")
	}
	if got := rr.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("Access-Control-Allow-Methods should be set for allowed origin")
	}
	if got := rr.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Error("Access-Control-Allow-Headers should be set for allowed origin")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusOK)
	}
}

// TestCORSAllowList_DisallowedOrigin verifies that a request from an origin not in
// the allowlist does NOT receive any CORS headers — the browser will block it.
func TestCORSAllowList_DisallowedOrigin(t *testing.T) {
	mw := newTestMiddleware([]string{"https://app.example.com"})
	handler := mw(noopHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Origin", "https://evil.attacker.com")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin should be absent for disallowed origin, got %q", got)
	}
	// The underlying handler still executes (the browser enforces CORS, not the server
	// by dropping the request body — but the absence of the header is the enforcement).
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusOK)
	}
}

// TestCORSAllowList_NoOriginHeader verifies that a same-origin or non-browser request
// (no Origin header) passes through without any CORS headers being added.
func TestCORSAllowList_NoOriginHeader(t *testing.T) {
	mw := newTestMiddleware([]string{"https://app.example.com"})
	handler := mw(noopHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	// Deliberately do NOT set Origin header.
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin should be absent when no Origin header, got %q", got)
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusOK)
	}
}

// TestCORSAllowList_Preflight_AllowedOrigin verifies that an OPTIONS preflight from
// an allowed origin receives 204 with full CORS headers.
func TestCORSAllowList_Preflight_AllowedOrigin(t *testing.T) {
	mw := newTestMiddleware([]string{"https://app.example.com"})
	handler := mw(noopHandler)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/posts", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("preflight status: got %d, want %d", rr.Code, http.StatusNoContent)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("Access-Control-Allow-Origin: got %q, want %q", got, "https://app.example.com")
	}
	if got := rr.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("Access-Control-Allow-Methods should be set on allowed preflight")
	}
	if got := rr.Header().Get("Access-Control-Max-Age"); got == "" {
		t.Error("Access-Control-Max-Age should be set on allowed preflight")
	}
}

// TestCORSAllowList_Preflight_DisallowedOrigin verifies that an OPTIONS preflight
// from a disallowed origin returns 204 with NO CORS headers.
func TestCORSAllowList_Preflight_DisallowedOrigin(t *testing.T) {
	mw := newTestMiddleware([]string{"https://app.example.com"})
	handler := mw(noopHandler)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/posts", nil)
	req.Header.Set("Origin", "https://evil.attacker.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("preflight status: got %d, want %d", rr.Code, http.StatusNoContent)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin must be absent for disallowed preflight, got %q", got)
	}
}

// TestCORSAllowList_MultipleOrigins verifies that each configured origin is
// individually allowed and that an origin not in the list is rejected.
func TestCORSAllowList_MultipleOrigins(t *testing.T) {
	allowed := []string{"https://app.example.com", "https://www.example.com"}
	mw := newTestMiddleware(allowed)
	handler := mw(noopHandler)

	for _, origin := range allowed {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
		req.Header.Set("Origin", origin)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("origin %q: Access-Control-Allow-Origin = %q, want %q", origin, got, origin)
		}
	}

	// A third origin not in the list must be blocked.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Origin", "https://other.example.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("unlisted origin: Access-Control-Allow-Origin should be absent, got %q", got)
	}
}

// TestCORSAllowList_EmptyList verifies that when no origins are configured no
// request receives a CORS header.
func TestCORSAllowList_EmptyList(t *testing.T) {
	mw := newTestMiddleware([]string{})
	handler := mw(noopHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set("Origin", "https://any.example.com")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("empty list: Access-Control-Allow-Origin should be absent, got %q", got)
	}
}

// TestCORSAllowList_NeverWildcard asserts that the allowlist middleware never emits
// Access-Control-Allow-Origin: * regardless of the request origin.
func TestCORSAllowList_NeverWildcard(t *testing.T) {
	cases := []struct {
		name      string
		origins   []string
		reqOrigin string
	}{
		{"allowed origin", []string{"https://app.example.com"}, "https://app.example.com"},
		{"disallowed origin", []string{"https://app.example.com"}, "https://evil.com"},
		{"empty list with origin", []string{}, "https://any.com"},
		{"no request origin", []string{"https://app.example.com"}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mw := newTestMiddleware(tc.origins)
			handler := mw(noopHandler)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.reqOrigin != "" {
				req.Header.Set("Origin", tc.reqOrigin)
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if got := rr.Header().Get("Access-Control-Allow-Origin"); got == "*" {
				t.Errorf("Access-Control-Allow-Origin must never be *, got * for case %q", tc.name)
			}
		})
	}
}
