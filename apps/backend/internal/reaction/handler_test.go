// Package reaction — handler tests.
//
// Unit tests for reaction HTTP endpoints using httptest and stub services.
// No real database or Redis connection is required.
package reaction

import (
	"context"
	"errors"
	"fmt"
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
// reactionHandlerSvc — test-local interface and stub
// ---------------------------------------------------------------------------

// reactionHandlerSvc matches the methods Handler calls on its *Service.
type reactionHandlerSvc interface {
	React(ctx context.Context, callerID, postID uuid.UUID, postAuthorID uuid.UUID) error
	Unreact(ctx context.Context, callerID, postID uuid.UUID) error
}

type stubReactionSvc struct {
	reactFn   func(ctx context.Context, callerID, postID uuid.UUID, postAuthorID uuid.UUID) error
	unreactFn func(ctx context.Context, callerID, postID uuid.UUID) error
}

func (s *stubReactionSvc) React(ctx context.Context, callerID, postID uuid.UUID, postAuthorID uuid.UUID) error {
	if s.reactFn != nil {
		return s.reactFn(ctx, callerID, postID, postAuthorID)
	}
	return nil
}

func (s *stubReactionSvc) Unreact(ctx context.Context, callerID, postID uuid.UUID) error {
	if s.unreactFn != nil {
		return s.unreactFn(ctx, callerID, postID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// stubPostAuthorLookup — test double for PostAuthorLookup
// ---------------------------------------------------------------------------

type stubPostAuthorLookup struct {
	authorID uuid.UUID
	err      error
}

func (s *stubPostAuthorLookup) GetPostAuthorID(_ context.Context, _ uuid.UUID) (uuid.UUID, error) {
	return s.authorID, s.err
}

// ---------------------------------------------------------------------------
// testReactionHandler — thin handler backed by the interface
// ---------------------------------------------------------------------------

// testReactionHandler mirrors the production Handler logic but accepts the
// test-local reactionHandlerSvc interface so stubs can be injected.
type testReactionHandler struct {
	svc        reactionHandlerSvc
	postLookup PostAuthorLookup
	logger     *zap.Logger
}

func (h *testReactionHandler) react(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}
	postID, ok := parseReactionUUIDParam(w, r, "postID")
	if !ok {
		return
	}
	postAuthorID, err := h.postLookup.GetPostAuthorID(r.Context(), postID)
	if err != nil {
		h.mapErr(w, err)
		return
	}
	if err := h.svc.React(r.Context(), callerID, postID, postAuthorID); err != nil {
		h.mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *testReactionHandler) unreact(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}
	postID, ok := parseReactionUUIDParam(w, r, "postID")
	if !ok {
		return
	}
	if err := h.svc.Unreact(r.Context(), callerID, postID); err != nil {
		h.mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *testReactionHandler) mapErr(w http.ResponseWriter, err error) {
	var apiErr *apierror.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case apierror.CodeValidation:
			apierror.Render(w, http.StatusBadRequest, apiErr.ToResponse())
		case apierror.CodeUnauthorized:
			apierror.Render(w, http.StatusUnauthorized, apiErr.ToResponse())
		case apierror.CodeForbidden:
			apierror.Render(w, http.StatusForbidden, apiErr.ToResponse())
		case apierror.CodeNotFound:
			apierror.Render(w, http.StatusNotFound, apiErr.ToResponse())
		case apierror.CodeRateLimit:
			w.Header().Set("Retry-After", "900")
			apierror.Render(w, http.StatusTooManyRequests, apiErr.ToResponse())
		default:
			apierror.Render(w, http.StatusInternalServerError,
				apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
		}
		return
	}
	apierror.Render(w, http.StatusInternalServerError,
		apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
}

// ---------------------------------------------------------------------------
// JWT helper (mirrors report/handler_test.go pattern)
// ---------------------------------------------------------------------------

var reactionTestJWTSecret = []byte("reaction-handler-test-secret-32b!!")

func reactionBearerToken(t *testing.T, userID uuid.UUID) string {
	t.Helper()
	tok, err := auth.GenerateAccessToken(userID, uuid.New(), reactionTestJWTSecret)
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}
	return "Bearer " + tok
}

// ---------------------------------------------------------------------------
// router builder
// ---------------------------------------------------------------------------

func buildReactionRouter(h *testReactionHandler) chi.Router {
	r := chi.NewRouter()
	r.With(auth.JWTMiddleware(reactionTestJWTSecret)).Post("/posts/{postID}/react", h.react)
	r.With(auth.JWTMiddleware(reactionTestJWTSecret)).Delete("/posts/{postID}/react", h.unreact)
	return r
}

// ---------------------------------------------------------------------------
// POST /posts/{postID}/react — unauthenticated → 401
// ---------------------------------------------------------------------------

// TestReactHandler_Unauthenticated_Returns401 verifies that a request without
// a valid JWT receives 401.
func TestReactHandler_Unauthenticated_Returns401(t *testing.T) {
	t.Parallel()

	lookup := &stubPostAuthorLookup{authorID: uuid.New()}
	h := &testReactionHandler{
		svc:        &stubReactionSvc{},
		postLookup: lookup,
		logger:     zap.NewNop(),
	}
	r := buildReactionRouter(h)

	postID := uuid.New()
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/posts/%s/react", postID), nil)
	// No Authorization header.

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// POST /posts/{postID}/react — authenticated → 204
// ---------------------------------------------------------------------------

// TestReactHandler_Authenticated_Returns204 verifies that an authenticated
// request returns 204 No Content.
func TestReactHandler_Authenticated_Returns204(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	postID := uuid.New()

	lookup := &stubPostAuthorLookup{authorID: uuid.New()}
	h := &testReactionHandler{
		svc:        &stubReactionSvc{},
		postLookup: lookup,
		logger:     zap.NewNop(),
	}
	r := buildReactionRouter(h)

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/posts/%s/react", postID), nil)
	req.Header.Set("Authorization", reactionBearerToken(t, userID))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("expected empty body on 204, got: %s", rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// POST /posts/{postID}/react — invalid postID → 400
// ---------------------------------------------------------------------------

// TestReactHandler_InvalidPostID_Returns400 verifies that a non-UUID path
// parameter returns HTTP 400.
func TestReactHandler_InvalidPostID_Returns400(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	lookup := &stubPostAuthorLookup{authorID: uuid.New()}
	h := &testReactionHandler{
		svc:        &stubReactionSvc{},
		postLookup: lookup,
		logger:     zap.NewNop(),
	}
	r := buildReactionRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/posts/not-a-uuid/react", nil)
	req.Header.Set("Authorization", reactionBearerToken(t, userID))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// POST /posts/{postID}/react — post not found → 404
// ---------------------------------------------------------------------------

// TestReactHandler_PostNotFound_Returns404 verifies that when the post lookup
// returns CodeNotFound, the handler maps it to HTTP 404.
func TestReactHandler_PostNotFound_Returns404(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	postID := uuid.New()

	lookup := &stubPostAuthorLookup{
		err: apierror.NewAPIError(apierror.CodeNotFound, "post not found"),
	}
	h := &testReactionHandler{
		svc:        &stubReactionSvc{},
		postLookup: lookup,
		logger:     zap.NewNop(),
	}
	r := buildReactionRouter(h)

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/posts/%s/react", postID), nil)
	req.Header.Set("Authorization", reactionBearerToken(t, userID))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// POST /posts/{postID}/react — rate limited → 429
// ---------------------------------------------------------------------------

// TestReactHandler_RateLimited_Returns429 verifies that a CodeRateLimit
// service error is mapped to HTTP 429 with a Retry-After header.
func TestReactHandler_RateLimited_Returns429(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	postID := uuid.New()

	lookup := &stubPostAuthorLookup{authorID: uuid.New()}
	h := &testReactionHandler{
		svc: &stubReactionSvc{
			reactFn: func(_ context.Context, _, _ uuid.UUID, _ uuid.UUID) error {
				return apierror.NewAPIError(apierror.CodeRateLimit, "Too many reaction requests.")
			},
		},
		postLookup: lookup,
		logger:     zap.NewNop(),
	}
	r := buildReactionRouter(h)

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/posts/%s/react", postID), nil)
	req.Header.Set("Authorization", reactionBearerToken(t, userID))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429 response")
	}
}

// ---------------------------------------------------------------------------
// POST /posts/{postID}/react — service error → 500
// ---------------------------------------------------------------------------

// TestReactHandler_ServiceError_Returns500 verifies that a CodeInternal
// service error is mapped to HTTP 500.
func TestReactHandler_ServiceError_Returns500(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	postID := uuid.New()

	lookup := &stubPostAuthorLookup{authorID: uuid.New()}
	h := &testReactionHandler{
		svc: &stubReactionSvc{
			reactFn: func(_ context.Context, _, _ uuid.UUID, _ uuid.UUID) error {
				return apierror.NewAPIError(apierror.CodeInternal, "db error")
			},
		},
		postLookup: lookup,
		logger:     zap.NewNop(),
	}
	r := buildReactionRouter(h)

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/posts/%s/react", postID), nil)
	req.Header.Set("Authorization", reactionBearerToken(t, userID))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// DELETE /posts/{postID}/react — unauthenticated → 401
// ---------------------------------------------------------------------------

// TestUnreactHandler_Unauthenticated_Returns401 verifies that a request
// without a valid JWT receives 401.
func TestUnreactHandler_Unauthenticated_Returns401(t *testing.T) {
	t.Parallel()

	lookup := &stubPostAuthorLookup{authorID: uuid.New()}
	h := &testReactionHandler{
		svc:        &stubReactionSvc{},
		postLookup: lookup,
		logger:     zap.NewNop(),
	}
	r := buildReactionRouter(h)

	postID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/posts/%s/react", postID), nil)
	// No Authorization header.

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// DELETE /posts/{postID}/react — authenticated → 204
// ---------------------------------------------------------------------------

// TestUnreactHandler_Authenticated_Returns204 verifies that an authenticated
// request returns 204 No Content with no response body.
func TestUnreactHandler_Authenticated_Returns204(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	postID := uuid.New()

	lookup := &stubPostAuthorLookup{authorID: uuid.New()}
	h := &testReactionHandler{
		svc:        &stubReactionSvc{},
		postLookup: lookup,
		logger:     zap.NewNop(),
	}
	r := buildReactionRouter(h)

	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/posts/%s/react", postID), nil)
	req.Header.Set("Authorization", reactionBearerToken(t, userID))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("expected empty body on 204, got: %s", rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// DELETE /posts/{postID}/react — invalid postID → 400
// ---------------------------------------------------------------------------

// TestUnreactHandler_InvalidPostID_Returns400 verifies that a non-UUID path
// parameter returns HTTP 400.
func TestUnreactHandler_InvalidPostID_Returns400(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	lookup := &stubPostAuthorLookup{authorID: uuid.New()}
	h := &testReactionHandler{
		svc:        &stubReactionSvc{},
		postLookup: lookup,
		logger:     zap.NewNop(),
	}
	r := buildReactionRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/posts/invalid-uuid/react", nil)
	req.Header.Set("Authorization", reactionBearerToken(t, userID))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// DELETE /posts/{postID}/react — no count in response body
// ---------------------------------------------------------------------------

// TestReactHandler_NoCountInResponse verifies that neither the react nor
// unreact endpoint includes any reaction count or like count in the response.
// Both endpoints return 204 with no body — this test confirms the empty body
// invariant (CLAUDE.md §2.3).
func TestReactHandler_NoCountInResponse(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	postID := uuid.New()

	lookup := &stubPostAuthorLookup{authorID: uuid.New()}
	h := &testReactionHandler{
		svc:        &stubReactionSvc{},
		postLookup: lookup,
		logger:     zap.NewNop(),
	}
	r := buildReactionRouter(h)

	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		req := httptest.NewRequest(method,
			fmt.Sprintf("/posts/%s/react", postID), nil)
		req.Header.Set("Authorization", reactionBearerToken(t, userID))

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("%s react: expected 204, got %d; body: %s", method, rec.Code, rec.Body.String())
			continue
		}
		body := rec.Body.String()
		if body != "" {
			t.Errorf("%s react: expected empty body (no metrics), got: %s", method, body)
		}
	}
}

// ---------------------------------------------------------------------------
// DELETE /posts/{postID}/react — service error → 500
// ---------------------------------------------------------------------------

// TestUnreactHandler_ServiceError_Returns500 verifies that a CodeInternal
// service error from Unreact is mapped to HTTP 500.
func TestUnreactHandler_ServiceError_Returns500(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	postID := uuid.New()

	lookup := &stubPostAuthorLookup{authorID: uuid.New()}
	h := &testReactionHandler{
		svc: &stubReactionSvc{
			unreactFn: func(_ context.Context, _, _ uuid.UUID) error {
				return apierror.NewAPIError(apierror.CodeInternal, "db error")
			},
		},
		postLookup: lookup,
		logger:     zap.NewNop(),
	}
	r := buildReactionRouter(h)

	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/posts/%s/react", postID), nil)
	req.Header.Set("Authorization", reactionBearerToken(t, userID))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Unregistered error type → 500
// ---------------------------------------------------------------------------

// TestReactHandler_UnknownError_Returns500 verifies that a plain (non-APIError)
// error is mapped to HTTP 500.
func TestReactHandler_UnknownError_Returns500(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	postID := uuid.New()

	lookup := &stubPostAuthorLookup{authorID: uuid.New()}
	h := &testReactionHandler{
		svc: &stubReactionSvc{
			reactFn: func(_ context.Context, _, _ uuid.UUID, _ uuid.UUID) error {
				return errors.New("totally unexpected panic-like error")
			},
		},
		postLookup: lookup,
		logger:     zap.NewNop(),
	}
	r := buildReactionRouter(h)

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/posts/%s/react", postID), nil)
	req.Header.Set("Authorization", reactionBearerToken(t, userID))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d; body: %s", rec.Code, rec.Body.String())
	}
}
