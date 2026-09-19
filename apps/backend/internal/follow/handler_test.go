// Package follow — handler tests.
//
// HTTP handler tests use net/http/httptest and a testableFollowHandler that
// accepts a followSvc interface, allowing a fakeFollowService to be injected
// without a real database or Redis connection.
//
// CRITICAL INVARIANT: the nil-Redis → 503 test for followRateLimitMiddleware
// must pass. This mirrors the same contract enforced in post/handler_test.go
// and auth/middleware_test.go (CLAUDE.md §12).
package follow

import (
	"context"
	"encoding/json"
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
// followSvc interface and fakeFollowService
// ---------------------------------------------------------------------------

// followSvc is the minimal interface of Service methods called by the handler.
type followSvc interface {
	Follow(ctx context.Context, callerID, targetID uuid.UUID) error
	Unfollow(ctx context.Context, callerID, targetID uuid.UUID) error
	ListFollowing(ctx context.Context, callerID, targetID uuid.UUID, cursorStr string) (FollowPage, error)
	ListFollowers(ctx context.Context, callerID, targetID uuid.UUID, cursorStr string) (FollowPage, error)
}

// fakeFollowService is a test double for followSvc.
type fakeFollowService struct {
	followFn        func(ctx context.Context, callerID, targetID uuid.UUID) error
	unfollowFn      func(ctx context.Context, callerID, targetID uuid.UUID) error
	listFollowingFn func(ctx context.Context, callerID, targetID uuid.UUID, cursorStr string) (FollowPage, error)
	listFollowersFn func(ctx context.Context, callerID, targetID uuid.UUID, cursorStr string) (FollowPage, error)
}

func (f *fakeFollowService) Follow(ctx context.Context, callerID, targetID uuid.UUID) error {
	if f.followFn != nil {
		return f.followFn(ctx, callerID, targetID)
	}
	return nil
}

func (f *fakeFollowService) Unfollow(ctx context.Context, callerID, targetID uuid.UUID) error {
	if f.unfollowFn != nil {
		return f.unfollowFn(ctx, callerID, targetID)
	}
	return nil
}

func (f *fakeFollowService) ListFollowing(ctx context.Context, callerID, targetID uuid.UUID, cursorStr string) (FollowPage, error) {
	if f.listFollowingFn != nil {
		return f.listFollowingFn(ctx, callerID, targetID, cursorStr)
	}
	return FollowPage{Items: []FollowUserDTO{}, Terminated: true}, nil
}

func (f *fakeFollowService) ListFollowers(ctx context.Context, callerID, targetID uuid.UUID, cursorStr string) (FollowPage, error) {
	if f.listFollowersFn != nil {
		return f.listFollowersFn(ctx, callerID, targetID, cursorStr)
	}
	return FollowPage{Items: []FollowUserDTO{}, Terminated: true}, nil
}

// ---------------------------------------------------------------------------
// testableFollowHandler — handler wired to followSvc interface
// ---------------------------------------------------------------------------

// testableFollowHandler mirrors Handler but accepts followSvc so a fake can
// be injected for unit tests.
type testableFollowHandler struct {
	svc followSvc
	log *zap.Logger
}

func newTestableFollowHandler(svc followSvc) *testableFollowHandler {
	log, _ := zap.NewDevelopment()
	return &testableFollowHandler{svc: svc, log: log}
}

func (h *testableFollowHandler) follow(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}
	targetID, ok := parseUUIDParam(w, r, "userID")
	if !ok {
		return
	}
	if err := h.svc.Follow(r.Context(), callerID, targetID); err != nil {
		h.handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *testableFollowHandler) handleServiceError(w http.ResponseWriter, err error) {
	ae, ok := err.(*apierror.APIError)
	if !ok {
		apierror.Render(w, http.StatusInternalServerError,
			apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
		return
	}
	switch ae.Code {
	case apierror.CodeValidation:
		apierror.Render(w, http.StatusUnprocessableEntity, ae.ToResponse())
	case apierror.CodeUnauthorized:
		apierror.Render(w, http.StatusUnauthorized, ae.ToResponse())
	case apierror.CodeForbidden:
		apierror.Render(w, http.StatusForbidden, ae.ToResponse())
	case apierror.CodeNotFound:
		apierror.Render(w, http.StatusNotFound, ae.ToResponse())
	case apierror.CodeRateLimit:
		apierror.Render(w, http.StatusTooManyRequests, ae.ToResponse())
	default:
		h.log.Error("follow handler: unexpected internal error", zap.Error(err))
		apierror.Render(w, http.StatusInternalServerError,
			apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
	}
}

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

var testFollowJWTSecret = []byte("follow-handler-test-secret-at-least-32!")

// makeFollowAuthedRequest creates an HTTP request with a valid JWT Authorization header.
func makeFollowAuthedRequest(t *testing.T, method, target string, userID uuid.UUID) *http.Request {
	t.Helper()
	tokenStr, err := auth.GenerateAccessToken(userID, uuid.New(), testFollowJWTSecret)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	req := httptest.NewRequest(method, target, nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	return req
}

// errorCodeFromFollowBody parses the apierror envelope and returns the error code string.
func errorCodeFromFollowBody(t *testing.T, body []byte) string {
	t.Helper()
	var resp apierror.ErrorResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("could not unmarshal error response: %v\nbody: %s", err, body)
	}
	return resp.Error.Code
}

// newFollowChiContextWithParam attaches a chi route context with a single URL
// param so that chi.URLParam can extract it inside the handler.
func newFollowChiContextWithParam(r *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// applyFollowJWT wraps the given handler with auth.JWTMiddleware and serves
// the request, returning the recorded response.
func applyFollowJWT(h http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	auth.JWTMiddleware(testFollowJWTSecret, zap.NewNop())(h).ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// TestFollowHandler_Unauthenticated
// ---------------------------------------------------------------------------

// TestFollowHandler_Unauthenticated verifies that POST /users/{userID}/follow
// without a JWT returns 401.
func TestFollowHandler_Unauthenticated(t *testing.T) {
	h := newTestableFollowHandler(&fakeFollowService{})

	targetID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/users/"+targetID.String()+"/follow", nil)
	req = newFollowChiContextWithParam(req, "userID", targetID.String())
	rec := httptest.NewRecorder()

	h.follow(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if code := errorCodeFromFollowBody(t, rec.Body.Bytes()); code != apierror.CodeUnauthorized {
		t.Errorf("error code = %q, want %q", code, apierror.CodeUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// TestFollowHandler_RateLimitNilRedis
// ---------------------------------------------------------------------------

// TestFollowHandler_RateLimitNilRedis verifies that passing a nil Redis client
// to followRateLimitMiddleware causes the middleware to fail closed with
// HTTP 503. This is the follow package equivalent of the Phase 1
// TestRateLimitMiddleware_NilRedis_Returns503 invariant (CLAUDE.md §12).
func TestFollowHandler_RateLimitNilRedis(t *testing.T) {
	log, _ := zap.NewDevelopment()

	// Sentinel handler that must NOT be called.
	called := false
	sentinel := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	userID := uuid.New()
	// Wire: JWTMiddleware → followRateLimitMiddleware(nil redis) → sentinel.
	// events is nil — counter instrumentation is a no-op in this test.
	rl := followRateLimitMiddleware(nil, log, nil)
	chain := auth.JWTMiddleware(testFollowJWTSecret, zap.NewNop())(rl(sentinel))

	req := makeFollowAuthedRequest(t, http.MethodPost, "/users/"+uuid.New().String()+"/follow", userID)
	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, req)

	if called {
		t.Fatal("sentinel handler must not be called when Redis is nil (fail-closed)")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 (fail-closed on nil Redis)", rec.Code)
	}
	if code := errorCodeFromFollowBody(t, rec.Body.Bytes()); code != apierror.CodeServiceUnavailable {
		t.Errorf("error code = %q, want %q", code, apierror.CodeServiceUnavailable)
	}
}

// ---------------------------------------------------------------------------
// TestFollowHandler_SelfFollow
// ---------------------------------------------------------------------------

// TestFollowHandler_SelfFollow verifies that POST /users/{userID}/follow where
// the JWT userID equals the path {userID} returns a 422 (validation error).
// The self-follow rule is enforced by the service.
func TestFollowHandler_SelfFollow(t *testing.T) {
	userID := uuid.New()

	fake := &fakeFollowService{
		followFn: func(_ context.Context, callerID, targetID uuid.UUID) error {
			if callerID == targetID {
				return apierror.NewAPIError(apierror.CodeValidation, "cannot follow yourself")
			}
			return nil
		},
	}
	h := newTestableFollowHandler(fake)

	req := makeFollowAuthedRequest(t, http.MethodPost, "/users/"+userID.String()+"/follow", userID)
	req = newFollowChiContextWithParam(req, "userID", userID.String())
	rec := applyFollowJWT(h.follow, req)

	// CodeValidation maps to 422 in the follow handler.
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 for self-follow", rec.Code)
	}
	if code := errorCodeFromFollowBody(t, rec.Body.Bytes()); code != apierror.CodeValidation {
		t.Errorf("error code = %q, want %q", code, apierror.CodeValidation)
	}
}
