// Package post — handler tests.
//
// HTTP handler tests use net/http/httptest and a testableHandler that accepts
// a postSvc interface, allowing a fakeService to be injected without a real
// database or Redis connection.
//
// CRITICAL INVARIANT: the nil-Redis → 503 test for postRateLimitMiddleware
// must pass. This mirrors the Phase 1 TestRateLimitMiddleware_NilRedis_Returns503
// contract from auth/middleware_test.go (CLAUDE.md §2.2, §12).
package post

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/auth"
)

// ---------------------------------------------------------------------------
// postSvc interface and fakeService
// ---------------------------------------------------------------------------

// postSvc is the minimal interface of Service methods called by the handler.
type postSvc interface {
	CreatePost(ctx context.Context, authorID uuid.UUID, req CreatePostRequest) (PostDTO, error)
	GetPost(ctx context.Context, postID uuid.UUID) (PostDTO, error)
	DeletePost(ctx context.Context, callerID uuid.UUID, postID uuid.UUID) error
	ListPostsByAuthor(ctx context.Context, callerID *uuid.UUID, authorID uuid.UUID, cursorStr string) (PostPage, error)
	ListThreadReplies(ctx context.Context, threadRootID uuid.UUID, cursorStr string) (PostPage, error)
}

// fakeService is a test double for postSvc.
type fakeService struct {
	createPostFn   func(ctx context.Context, authorID uuid.UUID, req CreatePostRequest) (PostDTO, error)
	getPostFn      func(ctx context.Context, postID uuid.UUID) (PostDTO, error)
	deletePostFn   func(ctx context.Context, callerID uuid.UUID, postID uuid.UUID) error
	listByAuthorFn func(ctx context.Context, callerID *uuid.UUID, authorID uuid.UUID, cursorStr string) (PostPage, error)
	listThreadFn   func(ctx context.Context, threadRootID uuid.UUID, cursorStr string) (PostPage, error)
}

func (f *fakeService) CreatePost(ctx context.Context, authorID uuid.UUID, req CreatePostRequest) (PostDTO, error) {
	if f.createPostFn != nil {
		return f.createPostFn(ctx, authorID, req)
	}
	return PostDTO{}, nil
}

func (f *fakeService) GetPost(ctx context.Context, postID uuid.UUID) (PostDTO, error) {
	if f.getPostFn != nil {
		return f.getPostFn(ctx, postID)
	}
	return PostDTO{}, apierror.NewAPIError(apierror.CodeNotFound, "post not found")
}

func (f *fakeService) DeletePost(ctx context.Context, callerID uuid.UUID, postID uuid.UUID) error {
	if f.deletePostFn != nil {
		return f.deletePostFn(ctx, callerID, postID)
	}
	return nil
}

func (f *fakeService) ListPostsByAuthor(ctx context.Context, callerID *uuid.UUID, authorID uuid.UUID, cursorStr string) (PostPage, error) {
	if f.listByAuthorFn != nil {
		return f.listByAuthorFn(ctx, callerID, authorID, cursorStr)
	}
	return PostPage{Items: []PostDTO{}, Terminated: true}, nil
}

func (f *fakeService) ListThreadReplies(ctx context.Context, threadRootID uuid.UUID, cursorStr string) (PostPage, error) {
	if f.listThreadFn != nil {
		return f.listThreadFn(ctx, threadRootID, cursorStr)
	}
	return PostPage{Items: []PostDTO{}, Terminated: true}, nil
}

// ---------------------------------------------------------------------------
// testableHandler — handler wired to a postSvc interface
// ---------------------------------------------------------------------------

// fakeReactionChecker is a test double for ReactionChecker.
type fakeReactionChecker struct {
	hasReactedFn func(ctx context.Context, userID, postID uuid.UUID) (bool, error)
}

func (f *fakeReactionChecker) HasReacted(ctx context.Context, userID, postID uuid.UUID) (bool, error) {
	if f.hasReactedFn != nil {
		return f.hasReactedFn(ctx, userID, postID)
	}
	return false, nil
}

// testableHandler mirrors Handler but accepts the postSvc interface so a
// fakeService can be injected for unit tests.
type testableHandler struct {
	svc             postSvc
	log             *zap.Logger
	reactionChecker ReactionChecker
}

func newTestableHandler(svc postSvc) *testableHandler {
	log, _ := zap.NewDevelopment()
	return &testableHandler{svc: svc, log: log}
}

func (h *testableHandler) createPost(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	var req CreatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Render(w, http.StatusBadRequest,
			apierror.New(apierror.CodeValidation, "Invalid JSON body."))
		return
	}

	if req.PostType != PostTypeRepost {
		count := runeCount(req.Content)
		if count > maxPostRunes {
			apierror.Render(w, http.StatusBadRequest,
				apierror.New(apierror.CodeValidation, "content exceeds 500 code points"))
			return
		}
	}

	dto, err := h.svc.CreatePost(r.Context(), callerID, req)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"post": dto})
}

func (h *testableHandler) getPost(w http.ResponseWriter, r *http.Request) {
	postID, ok := parseUUIDParam(w, r, "postID")
	if !ok {
		return
	}

	dto, err := h.svc.GetPost(r.Context(), postID)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	resp := PostDetailResponse{PostDTO: dto}

	if callerID, authed := auth.UserIDFromContext(r.Context()); authed && h.reactionChecker != nil {
		reacted, reactionErr := h.reactionChecker.HasReacted(r.Context(), callerID, postID)
		if reactionErr == nil {
			resp.ViewerHasReacted = &reacted
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"post": resp})
}

func (h *testableHandler) deletePost(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	postID, ok := parseUUIDParam(w, r, "postID")
	if !ok {
		return
	}

	if err := h.svc.DeletePost(r.Context(), callerID, postID); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *testableHandler) listUserPosts(w http.ResponseWriter, r *http.Request) {
	userID, ok := parseUUIDParam(w, r, "userID")
	if !ok {
		return
	}

	cursor := r.URL.Query().Get("cursor")

	var callerID *uuid.UUID
	if id, authed := auth.UserIDFromContext(r.Context()); authed {
		callerID = &id
	}

	page, err := h.svc.ListPostsByAuthor(r.Context(), callerID, userID, cursor)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, page)
}

func (h *testableHandler) handleServiceError(w http.ResponseWriter, err error) {
	ae, ok := err.(*apierror.APIError)
	if !ok {
		apierror.Render(w, http.StatusInternalServerError,
			apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
		return
	}
	switch ae.Code {
	case apierror.CodeValidation:
		apierror.Render(w, http.StatusBadRequest, ae.ToResponse())
	case apierror.CodeUnauthorized:
		apierror.Render(w, http.StatusUnauthorized, ae.ToResponse())
	case apierror.CodeForbidden:
		apierror.Render(w, http.StatusForbidden, ae.ToResponse())
	case apierror.CodeNotFound:
		apierror.Render(w, http.StatusNotFound, ae.ToResponse())
	case apierror.CodeRateLimit:
		apierror.Render(w, http.StatusTooManyRequests, ae.ToResponse())
	default:
		h.log.Error("post handler: unexpected internal error", zap.Error(err))
		apierror.Render(w, http.StatusInternalServerError,
			apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
	}
}

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

var testJWTSecret = []byte("handler-test-secret-at-least-32-bytes!")

// makeAuthedRequest creates an HTTP request with a valid JWT Authorization header.
func makeAuthedRequest(t *testing.T, method, target string, body []byte, userID uuid.UUID) *http.Request {
	t.Helper()
	tokenStr, err := auth.GenerateAccessToken(userID, uuid.New(), testJWTSecret)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, target, bytes.NewReader(body))
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	return req
}

// errorCodeFromBody parses the apierror envelope and returns the error code string.
func errorCodeFromBody(t *testing.T, body []byte) string {
	t.Helper()
	var resp apierror.ErrorResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("could not unmarshal error response: %v\nbody: %s", err, body)
	}
	return resp.Error.Code
}

// newChiContextWithParam attaches a chi route context with a single URL param
// so that chi.URLParam can extract it inside the handler.
func newChiContextWithParam(r *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// applyJWT wraps the given handler with auth.JWTMiddleware and serves the
// request, returning the recorded response.
func applyJWT(h http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	auth.JWTMiddleware(testJWTSecret, zap.NewNop())(h).ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// TestCreatePostHandler_Unauthenticated
// ---------------------------------------------------------------------------

// TestCreatePostHandler_Unauthenticated verifies that POST /posts without a
// JWT returns 401.
func TestCreatePostHandler_Unauthenticated(t *testing.T) {
	h := newTestableHandler(&fakeService{})

	body, _ := json.Marshal(CreatePostRequest{PostType: PostTypeOriginal, Content: "hello"})
	req := httptest.NewRequest(http.MethodPost, "/posts", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.createPost(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if code := errorCodeFromBody(t, rec.Body.Bytes()); code != apierror.CodeUnauthorized {
		t.Errorf("error code = %q, want %q", code, apierror.CodeUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// TestCreatePostHandler_ContentTooLong
// ---------------------------------------------------------------------------

// TestCreatePostHandler_ContentTooLong verifies that a 501-rune content in
// POST /posts returns 400 VALIDATION_ERROR.
func TestCreatePostHandler_ContentTooLong(t *testing.T) {
	h := newTestableHandler(&fakeService{})

	longContent := strings.Repeat("a", maxPostRunes+1)
	body, _ := json.Marshal(CreatePostRequest{PostType: PostTypeOriginal, Content: longContent})

	userID := uuid.New()
	req := makeAuthedRequest(t, http.MethodPost, "/posts", body, userID)
	rec := applyJWT(h.createPost, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for 501-rune content", rec.Code)
	}
	if code := errorCodeFromBody(t, rec.Body.Bytes()); code != apierror.CodeValidation {
		t.Errorf("error code = %q, want %q", code, apierror.CodeValidation)
	}
}

// ---------------------------------------------------------------------------
// TestCreatePostHandler_RateLimitNilRedis
// ---------------------------------------------------------------------------

// TestCreatePostHandler_RateLimitNilRedis verifies that passing a nil Redis
// client to postRateLimitMiddleware causes the middleware to fail closed with
// HTTP 503. This is the post package equivalent of the Phase 1
// TestRateLimitMiddleware_NilRedis_Returns503 invariant (CLAUDE.md §12).
func TestCreatePostHandler_RateLimitNilRedis(t *testing.T) {
	log, _ := zap.NewDevelopment()

	// Sentinel handler that must NOT be called.
	called := false
	sentinel := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	userID := uuid.New()
	// Wire: JWTMiddleware → postRateLimitMiddleware(nil redis) → sentinel.
	// events is nil — counter instrumentation is a no-op in this test.
	rl := postRateLimitMiddleware(nil, log, nil)
	chain := auth.JWTMiddleware(testJWTSecret, zap.NewNop())(rl(sentinel))

	req := makeAuthedRequest(t, http.MethodPost, "/posts", nil, userID)
	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, req)

	if called {
		t.Fatal("sentinel handler must not be called when Redis is nil (fail-closed)")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 (fail-closed on nil Redis)", rec.Code)
	}
	if code := errorCodeFromBody(t, rec.Body.Bytes()); code != apierror.CodeServiceUnavailable {
		t.Errorf("error code = %q, want %q", code, apierror.CodeServiceUnavailable)
	}
}

// ---------------------------------------------------------------------------
// TestGetPostHandler_NotFound
// ---------------------------------------------------------------------------

// TestGetPostHandler_NotFound verifies that GET /posts/{postID} for an
// unknown post returns 404.
func TestGetPostHandler_NotFound(t *testing.T) {
	fake := &fakeService{
		getPostFn: func(_ context.Context, _ uuid.UUID) (PostDTO, error) {
			return PostDTO{}, apierror.NewAPIError(apierror.CodeNotFound, "post not found")
		},
	}
	h := newTestableHandler(fake)

	postID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/posts/"+postID.String(), nil)
	req = newChiContextWithParam(req, "postID", postID.String())
	rec := httptest.NewRecorder()

	h.getPost(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if code := errorCodeFromBody(t, rec.Body.Bytes()); code != apierror.CodeNotFound {
		t.Errorf("error code = %q, want %q", code, apierror.CodeNotFound)
	}
}

// ---------------------------------------------------------------------------
// TestGetPostHandler_ValidPost
// ---------------------------------------------------------------------------

// TestGetPostHandler_ValidPost verifies that GET /posts/{postID} for an
// existing post returns 200 with a PostDTO that contains no metric fields
// (CLAUDE.md §2.3).
func TestGetPostHandler_ValidPost(t *testing.T) {
	postID := uuid.New()
	authorID := uuid.New()
	content := "hello from the post"

	fake := &fakeService{
		getPostFn: func(_ context.Context, id uuid.UUID) (PostDTO, error) {
			return PostDTO{
				ID:       id.String(),
				AuthorID: authorID.String(),
				Author:   PostAuthor{ID: authorID.String(), Handle: "author", DisplayName: "Author"},
				PostType: PostTypeOriginal,
				Content:  &content,
			}, nil
		},
	}
	h := newTestableHandler(fake)

	req := httptest.NewRequest(http.MethodGet, "/posts/"+postID.String(), nil)
	req = newChiContextWithParam(req, "postID", postID.String())
	rec := httptest.NewRecorder()

	h.getPost(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	// Verify the response is valid JSON with a "post" key.
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if _, ok := resp["post"]; !ok {
		t.Error("response missing 'post' key")
	}

	// Verify no metric substrings appear in JSON keys (belt-and-suspenders).
	jsonStr := strings.ToLower(rec.Body.String())
	for _, forbidden := range forbiddenMetricSubstrings {
		// Check for the forbidden substring as the start of a JSON key.
		if strings.Contains(jsonStr, `"`+forbidden) {
			t.Errorf("response JSON contains forbidden metric key fragment %q", forbidden)
		}
	}
}

// ---------------------------------------------------------------------------
// TestDeletePostHandler_Unauthenticated
// ---------------------------------------------------------------------------

// TestDeletePostHandler_Unauthenticated verifies that DELETE /posts/{postID}
// without a JWT returns 401.
func TestDeletePostHandler_Unauthenticated(t *testing.T) {
	h := newTestableHandler(&fakeService{})

	postID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/posts/"+postID.String(), nil)
	req = newChiContextWithParam(req, "postID", postID.String())
	rec := httptest.NewRecorder()

	h.deletePost(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if code := errorCodeFromBody(t, rec.Body.Bytes()); code != apierror.CodeUnauthorized {
		t.Errorf("error code = %q, want %q", code, apierror.CodeUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// TestDeletePostHandler_Forbidden
// ---------------------------------------------------------------------------

// TestDeletePostHandler_Forbidden verifies that DELETE /posts/{postID} where
// the JWT belongs to user A but the post belongs to user B returns 403.
func TestDeletePostHandler_Forbidden(t *testing.T) {
	callerID := uuid.New()
	postID := uuid.New()

	fake := &fakeService{
		deletePostFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
			return apierror.NewAPIError(apierror.CodeForbidden, "you may not delete another user's post")
		},
	}
	h := newTestableHandler(fake)

	req := makeAuthedRequest(t, http.MethodDelete, "/posts/"+postID.String(), nil, callerID)
	req = newChiContextWithParam(req, "postID", postID.String())
	rec := applyJWT(h.deletePost, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
	if code := errorCodeFromBody(t, rec.Body.Bytes()); code != apierror.CodeForbidden {
		t.Errorf("error code = %q, want %q", code, apierror.CodeForbidden)
	}
}

// ---------------------------------------------------------------------------
// TestListUserPostsHandler_Terminated
// ---------------------------------------------------------------------------

// TestListUserPostsHandler_Terminated verifies that when the service returns
// a terminated feed, the JSON response body contains "terminated":true.
// This is the client signal to stop fetching (CLAUDE.md §2.1).
func TestListUserPostsHandler_Terminated(t *testing.T) {
	fake := &fakeService{
		listByAuthorFn: func(_ context.Context, _ *uuid.UUID, _ uuid.UUID, _ string) (PostPage, error) {
			return PostPage{Items: []PostDTO{}, NextCursor: "", Terminated: true}, nil
		},
	}
	h := newTestableHandler(fake)

	userID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/users/"+userID.String()+"/posts", nil)
	req = newChiContextWithParam(req, "userID", userID.String())
	rec := httptest.NewRecorder()

	h.listUserPosts(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var page PostPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("unmarshal PostPage: %v\nbody: %s", err, rec.Body.Bytes())
	}
	if !page.Terminated {
		t.Errorf("expected terminated=true in response, got false\nbody: %s", rec.Body.Bytes())
	}
}

// ---------------------------------------------------------------------------
// TestGetPostHandler_ViewerHasReacted_Unauthenticated
// ---------------------------------------------------------------------------

// TestGetPostHandler_ViewerHasReacted_Unauthenticated verifies that an
// unauthenticated GET /posts/{postID} returns 200 with viewer_has_reacted
// fully absent from the JSON (not false, not null). CLAUDE.md §2.3.
func TestGetPostHandler_ViewerHasReacted_Unauthenticated(t *testing.T) {
	postID := uuid.New()
	authorID := uuid.New()
	content := "hello"

	fake := &fakeService{
		getPostFn: func(_ context.Context, id uuid.UUID) (PostDTO, error) {
			return PostDTO{
				ID:       id.String(),
				AuthorID: authorID.String(),
				Author:   PostAuthor{ID: authorID.String(), Handle: "author", DisplayName: "Author"},
				PostType: PostTypeOriginal,
				Content:  &content,
			}, nil
		},
	}
	h := newTestableHandler(fake)
	h.reactionChecker = &fakeReactionChecker{
		hasReactedFn: func(_ context.Context, _, _ uuid.UUID) (bool, error) {
			return true, nil
		},
	}

	// Unauthenticated request — no JWT.
	req := httptest.NewRequest(http.MethodGet, "/posts/"+postID.String(), nil)
	req = newChiContextWithParam(req, "postID", postID.String())
	rec := httptest.NewRecorder()

	h.getPost(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	// viewer_has_reacted must be fully absent for unauthenticated requests.
	body := rec.Body.String()
	if strings.Contains(body, "viewer_has_reacted") {
		t.Errorf("unauthenticated response must not contain viewer_has_reacted, got: %s", body)
	}
}

// ---------------------------------------------------------------------------
// TestGetPostHandler_ViewerHasReacted_Authenticated
// ---------------------------------------------------------------------------

// TestGetPostHandler_ViewerHasReacted_Authenticated verifies that an
// authenticated GET /posts/{postID} returns viewer_has_reacted in the JSON.
// Both true and false must be serialized explicitly. CLAUDE.md §2.3.
func TestGetPostHandler_ViewerHasReacted_Authenticated(t *testing.T) {
	postID := uuid.New()
	authorID := uuid.New()
	callerID := uuid.New()
	content := "hello"

	for _, wantReacted := range []bool{true, false} {
		reacted := wantReacted
		fake := &fakeService{
			getPostFn: func(_ context.Context, id uuid.UUID) (PostDTO, error) {
				return PostDTO{
					ID:       id.String(),
					AuthorID: authorID.String(),
					Author:   PostAuthor{ID: authorID.String(), Handle: "author", DisplayName: "Author"},
					PostType: PostTypeOriginal,
					Content:  &content,
				}, nil
			},
		}
		h := newTestableHandler(fake)
		h.reactionChecker = &fakeReactionChecker{
			hasReactedFn: func(_ context.Context, _, _ uuid.UUID) (bool, error) {
				return reacted, nil
			},
		}

		req := makeAuthedRequest(t, http.MethodGet, "/posts/"+postID.String(), nil, callerID)
		req = newChiContextWithParam(req, "postID", postID.String())
		rec := applyJWT(h.getPost, req)

		if rec.Code != http.StatusOK {
			t.Errorf("wantReacted=%v: status = %d, want 200", wantReacted, rec.Code)
			continue
		}

		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("wantReacted=%v: unmarshal: %v", wantReacted, err)
		}
		postRaw, ok := envelope["post"]
		if !ok {
			t.Errorf("wantReacted=%v: response missing 'post' key", wantReacted)
			continue
		}
		var detail map[string]json.RawMessage
		if err := json.Unmarshal(postRaw, &detail); err != nil {
			t.Fatalf("wantReacted=%v: unmarshal post: %v", wantReacted, err)
		}
		raw, present := detail["viewer_has_reacted"]
		if !present {
			t.Errorf("wantReacted=%v: viewer_has_reacted absent from authenticated response", wantReacted)
			continue
		}
		var gotReacted bool
		if err := json.Unmarshal(raw, &gotReacted); err != nil {
			t.Fatalf("wantReacted=%v: unmarshal viewer_has_reacted: %v", wantReacted, err)
		}
		if gotReacted != wantReacted {
			t.Errorf("wantReacted=%v: got viewer_has_reacted=%v", wantReacted, gotReacted)
		}
	}
}
