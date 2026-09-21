// Package auth — handler tests.
//
// These tests verify the HTTP body size limit enforcement in the auth handler.
// A nil *Service is safe for all tests below because the MaxBytesError check
// fires during JSON decode, before any service method is invoked.
package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
)

// assertPayloadTooLarge checks for 413 + VALIDATION_ERROR + expected message.
func assertPayloadTooLarge(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", rec.Code)
	}
	var resp apierror.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error response: %v\nbody: %s", err, rec.Body.Bytes())
	}
	if resp.Error.Code != apierror.CodeValidation {
		t.Errorf("error code = %q, want %q", resp.Error.Code, apierror.CodeValidation)
	}
	if resp.Error.Message != "Request body too large." {
		t.Errorf("message = %q, want %q", resp.Error.Message, "Request body too large.")
	}
}

// oversizedJSONBody returns a valid JSON object whose encoded size exceeds
// the 1 MiB application limit. The JSON decoder must start parsing valid JSON
// before it hits MaxBytesReader, otherwise the decoder returns a syntax error
// rather than *http.MaxBytesError.
func oversizedJSONBody() []byte {
	// {"p":"aaaa..."} — the value string is longer than 1 MiB so the decoder
	// hits the reader limit while reading the string.
	payload := map[string]string{"p": strings.Repeat("a", (1<<20)+100)}
	b, _ := json.Marshal(payload)
	return b
}

// ---------------------------------------------------------------------------
// TestRegisterHandler_BodyTooLarge
// ---------------------------------------------------------------------------

// TestRegisterHandler_BodyTooLarge verifies that POST /auth/register with a body
// exceeding the 1 MiB limit returns 413 with a VALIDATION_ERROR envelope.
func TestRegisterHandler_BodyTooLarge(t *testing.T) {
	h := NewHandler(nil, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(oversizedJSONBody()))
	rec := httptest.NewRecorder()
	// Simulate the global LimitRequestBody middleware.
	req.Body = http.MaxBytesReader(rec, req.Body, 1<<20)

	h.register(rec, req)

	assertPayloadTooLarge(t, rec)
}

// ---------------------------------------------------------------------------
// TestLoginHandler_BodyTooLarge
// ---------------------------------------------------------------------------

// TestLoginHandler_BodyTooLarge verifies that POST /auth/login with a body
// exceeding the 1 MiB limit returns 413 with a VALIDATION_ERROR envelope.
func TestLoginHandler_BodyTooLarge(t *testing.T) {
	h := NewHandler(nil, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(oversizedJSONBody()))
	rec := httptest.NewRecorder()
	req.Body = http.MaxBytesReader(rec, req.Body, 1<<20)

	h.login(rec, req)

	assertPayloadTooLarge(t, rec)
}

// ---------------------------------------------------------------------------
// TestRefreshHandler_BodyTooLarge
// ---------------------------------------------------------------------------

// TestRefreshHandler_BodyTooLarge verifies that POST /auth/refresh with a body
// exceeding the 1 MiB limit returns 413 with a VALIDATION_ERROR envelope.
func TestRefreshHandler_BodyTooLarge(t *testing.T) {
	h := NewHandler(nil, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewReader(oversizedJSONBody()))
	rec := httptest.NewRecorder()
	req.Body = http.MaxBytesReader(rec, req.Body, 1<<20)

	h.refresh(rec, req)

	assertPayloadTooLarge(t, rec)
}
