package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSecurityHeaders_Present verifies that SecurityHeaders sets the three
// required security response headers on every response.
func TestSecurityHeaders_Present(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	tests := []struct {
		header string
		want   string
	}{
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "DENY"},
		{"Referrer-Policy", "no-referrer"},
	}

	for _, tc := range tests {
		got := rr.Header().Get(tc.header)
		if got != tc.want {
			t.Errorf("SecurityHeaders: %s: want %q, got %q", tc.header, tc.want, got)
		}
	}
}

// TestSecurityHeaders_CallsNext verifies that SecurityHeaders passes the request
// to the next handler and does not interfere with the response status or body.
func TestSecurityHeaders_CallsNext(t *testing.T) {
	const wantStatus = http.StatusCreated
	const wantBody = `{"ok":true}`

	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(wantStatus)
		_, _ = w.Write([]byte(wantBody))
	}))

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != wantStatus {
		t.Errorf("SecurityHeaders: status: want %d, got %d", wantStatus, rr.Code)
	}
	if rr.Body.String() != wantBody {
		t.Errorf("SecurityHeaders: body: want %q, got %q", wantBody, rr.Body.String())
	}
}

// TestSecurityHeaders_NoSTS verifies that Strict-Transport-Security is NOT set
// by this middleware (it belongs at the reverse proxy layer).
func TestSecurityHeaders_NoSTS(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if sts := rr.Header().Get("Strict-Transport-Security"); sts != "" {
		t.Errorf("SecurityHeaders: must NOT set Strict-Transport-Security (belongs at proxy), got: %q", sts)
	}
}
