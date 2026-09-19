// Package block — handler tests.
//
// Unit tests for block/mute HTTP handler behavior. These tests exercise the
// HTTP layer without a real database or Redis connection.
package block

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/auth"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// signTestToken creates a valid access JWT for the given user ID.
func signTestToken(userID uuid.UUID, secret []byte) (string, error) {
	return auth.GenerateAccessToken(userID, secret)
}

// buildChiRequest builds an *http.Request with chi URL params injected so
// that chi.URLParam works in handler unit tests.
func buildChiRequest(method, urlPath, paramName, paramValue string) (*http.Request, *httptest.ResponseRecorder) {
	r := httptest.NewRequest(method, urlPath, nil)
	w := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(paramName, paramValue)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	return r, w
}

// injectAuthContext runs the request through JWTMiddleware with the given
// token and returns the enriched request (with user ID in context).
// Returns (nil, false) if the middleware rejected the token.
func injectAuthContext(r *http.Request, token string, secret []byte) (*http.Request, bool) {
	var enriched *http.Request
	called := false
	inner := http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		enriched = req
		called = true
	})
	w := httptest.NewRecorder()
	r.Header.Set("Authorization", "Bearer "+token)
	auth.JWTMiddleware(secret)(inner).ServeHTTP(w, r)
	return enriched, called
}

// handleServiceErrorForTest replicates the handler's error mapping for the
// testable handler path (CodeValidation → 422).
func handleServiceErrorForTest(w http.ResponseWriter, err error, log *zap.Logger) {
	var apiErr *apierror.APIError
	if apiErrAs(err, &apiErr) {
		switch apiErr.Code {
		case apierror.CodeValidation:
			apierror.Render(w, http.StatusUnprocessableEntity, apiErr.ToResponse())
		case apierror.CodeForbidden:
			apierror.Render(w, http.StatusForbidden, apiErr.ToResponse())
		default:
			log.Error("block test: unexpected error", zap.Error(err))
			apierror.Render(w, http.StatusInternalServerError,
				apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
		}
		return
	}
	log.Error("block test: unexpected error", zap.Error(err))
	apierror.Render(w, http.StatusInternalServerError,
		apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
}

func apiErrAs(err error, target **apierror.APIError) bool {
	ae, ok := err.(*apierror.APIError)
	if ok {
		*target = ae
	}
	return ok
}

// ---------------------------------------------------------------------------
// TestBlockHandler_Unauthenticated
// ---------------------------------------------------------------------------

// TestBlockHandler_Unauthenticated verifies that POST /users/{userID}/block
// without a JWT returns HTTP 401.
func TestBlockHandler_Unauthenticated(t *testing.T) {
	log, _ := zap.NewDevelopment()

	// Use a nil service — the 401 must fire before any service call.
	h := &Handler{svc: nil, log: log}

	targetID := uuid.New()
	r, w := buildChiRequest(http.MethodPost, "/users/"+targetID.String()+"/block", "userID", targetID.String())
	// No Authorization header — JWTMiddleware would reject, but we call the
	// handler directly here to test the handler's own auth guard.

	h.block(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// TestBlockHandler_RateLimitNilRedis
// ---------------------------------------------------------------------------

// TestBlockHandler_RateLimitNilRedis verifies that when Redis is nil the
// rate limit middleware returns HTTP 503 (fail-closed).
func TestBlockHandler_RateLimitNilRedis(t *testing.T) {
	log, _ := zap.NewDevelopment()

	jwtSecret := []byte("test-secret-32-bytes-long-padding!")
	callerID := uuid.New()

	tokenStr, err := signTestToken(callerID, jwtSecret)
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}

	// nil Redis — rate limit middleware must fail closed.
	// events is nil — counter instrumentation is a no-op in this test.
	rl := blockRateLimitMiddleware(nil, log, nil)

	handlerCalled := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusNoContent)
	})

	// Chain: JWTMiddleware → rate limit → inner
	chain := auth.JWTMiddleware(jwtSecret)(rl(inner))

	r := httptest.NewRequest(http.MethodPost, "/users/"+uuid.New().String()+"/block", nil)
	r.Header.Set("Authorization", "Bearer "+tokenStr)
	w := httptest.NewRecorder()

	chain.ServeHTTP(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 ServiceUnavailable with nil Redis, got %d", w.Code)
	}
	if handlerCalled {
		t.Error("inner handler must not be called when rate limit fails closed")
	}
}

// ---------------------------------------------------------------------------
// TestBlockHandler_SelfBlock
// ---------------------------------------------------------------------------

// TestBlockHandler_SelfBlock verifies that when callerID == the userID path
// parameter the service returns CodeValidation and the handler renders 422.
func TestBlockHandler_SelfBlock(t *testing.T) {
	log, _ := zap.NewDevelopment()
	jwtSecret := []byte("test-secret-32-bytes-long-padding!")

	callerID := uuid.New()
	tokenStr, err := signTestToken(callerID, jwtSecret)
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}

	// Build the request with chi URL param set to callerID (self-block).
	r, _ := buildChiRequest(http.MethodPost, "/users/"+callerID.String()+"/block", "userID", callerID.String())

	// Inject authenticated caller context.
	enriched, ok := injectAuthContext(r, tokenStr, jwtSecret)
	if !ok {
		t.Fatal("JWTMiddleware rejected the test token — check token setup")
	}

	// Use a testable service with fake repo — the self-block check fires before
	// any repository call.
	fake := &fakeBlockRepo{}
	svc := newTestableService(fake)

	// Execute the handler logic inline: auth guard → parse param → service call.
	w := httptest.NewRecorder()

	callerFromCtx, authed := auth.UserIDFromContext(enriched.Context())
	if !authed {
		t.Fatal("expected user ID in context after JWTMiddleware")
	}

	targetIDStr := chi.URLParam(enriched, "userID")
	targetID, parseErr := uuid.Parse(targetIDStr)
	if parseErr != nil {
		t.Fatalf("uuid.Parse failed: %v", parseErr)
	}

	serviceErr := svc.Block(enriched.Context(), callerFromCtx, targetID)
	if serviceErr == nil {
		t.Fatal("expected self-block to return an error")
	}

	handleServiceErrorForTest(w, serviceErr, log)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 Unprocessable Entity for self-block, got %d", w.Code)
	}
}
