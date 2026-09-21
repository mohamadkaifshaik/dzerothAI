// Package user — handler tests.
//
// These tests verify the HTTP body size limit enforcement in the user handler.
// They pass nil as the *Service because the MaxBytesError check happens during
// JSON decode, before any service method is called — nil is safe for these tests.
package user

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/auth"
)

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

var testJWTSecret = []byte("user-handler-test-secret-32bytes!")

// oversizedJSONBody returns a valid JSON object whose encoded size exceeds the
// 1 MiB application limit. The JSON decoder must start parsing valid JSON
// before it hits MaxBytesReader, otherwise the decoder returns a syntax error
// rather than *http.MaxBytesError.
func oversizedJSONBody() []byte {
	payload := map[string]string{"p": strings.Repeat("a", (1<<20)+100)}
	b, _ := json.Marshal(payload)
	return b
}

// makeAuthedRequest creates a request with a JWT Authorization header.
func makeAuthedRequest(t *testing.T, method, target string, body []byte, userID uuid.UUID) *http.Request {
	t.Helper()
	tok, err := auth.GenerateAccessToken(userID, uuid.New(), testJWTSecret)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	return req
}

// serveWithJWT chains JWTMiddleware before h and records the response.
func serveWithJWT(h http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	auth.JWTMiddleware(testJWTSecret, zap.NewNop())(h).ServeHTTP(rec, req)
	return rec
}

// assertPayloadTooLarge checks the recorder for 413 + VALIDATION_ERROR envelope.
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

// ---------------------------------------------------------------------------
// TestUpdateMeHandler_BodyTooLarge
// ---------------------------------------------------------------------------

// TestUpdateMeHandler_BodyTooLarge verifies that PUT /me with a body exceeding
// the 1 MiB application limit returns 413 with a VALIDATION_ERROR envelope.
// A nil *Service is safe because the MaxBytesError check fires during JSON
// decode, before any service call is made.
func TestUpdateMeHandler_BodyTooLarge(t *testing.T) {
	h := NewHandler(nil, zap.NewNop())

	userID := uuid.New()
	req := makeAuthedRequest(t, http.MethodPut, "/me", oversizedJSONBody(), userID)
	rec := httptest.NewRecorder()
	// Simulate the global LimitRequestBody middleware.
	req.Body = http.MaxBytesReader(rec, req.Body, 1<<20)

	auth.JWTMiddleware(testJWTSecret, zap.NewNop())(http.HandlerFunc(h.updateMe)).ServeHTTP(rec, req)

	assertPayloadTooLarge(t, rec)
}

// ---------------------------------------------------------------------------
// TestUpdateSettingsHandler_BodyTooLarge
// ---------------------------------------------------------------------------

// TestUpdateSettingsHandler_BodyTooLarge verifies that PUT /me/settings with a
// body exceeding the 1 MiB application limit returns 413 with a VALIDATION_ERROR
// envelope.
func TestUpdateSettingsHandler_BodyTooLarge(t *testing.T) {
	h := NewHandler(nil, zap.NewNop())

	userID := uuid.New()
	req := makeAuthedRequest(t, http.MethodPut, "/me/settings", oversizedJSONBody(), userID)
	rec := httptest.NewRecorder()
	// Simulate the global LimitRequestBody middleware.
	req.Body = http.MaxBytesReader(rec, req.Body, 1<<20)

	auth.JWTMiddleware(testJWTSecret, zap.NewNop())(http.HandlerFunc(h.updateSettings)).ServeHTTP(rec, req)

	assertPayloadTooLarge(t, rec)
}
