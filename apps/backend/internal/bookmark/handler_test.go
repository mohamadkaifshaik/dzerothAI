// Package bookmark — handler tests.
//
// HTTP handler tests use net/http/httptest and a testable handler that accepts
// a bookmarkSvc interface, allowing a fakeBookmarkService to be injected without
// a real database or Redis connection.
package bookmark

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/auth"
)

// ---------------------------------------------------------------------------
// bookmarkSvc interface and fakeBookmarkService
// ---------------------------------------------------------------------------

// bookmarkSvc is the minimal interface of Service methods called by the handler.
type bookmarkSvc interface {
	Bookmark(ctx context.Context, callerID, postID uuid.UUID) error
	Unbookmark(ctx context.Context, callerID, postID uuid.UUID) error
	ListBookmarks(ctx context.Context, callerID uuid.UUID, cursorStr string) (BookmarkPage, error)
}

// fakeBookmarkService is a test double for bookmarkSvc.
type fakeBookmarkService struct {
	bookmarkFn      func(ctx context.Context, callerID, postID uuid.UUID) error
	unbookmarkFn    func(ctx context.Context, callerID, postID uuid.UUID) error
	listBookmarksFn func(ctx context.Context, callerID uuid.UUID, cursorStr string) (BookmarkPage, error)
}

func (f *fakeBookmarkService) Bookmark(ctx context.Context, callerID, postID uuid.UUID) error {
	if f.bookmarkFn != nil {
		return f.bookmarkFn(ctx, callerID, postID)
	}
	return nil
}

func (f *fakeBookmarkService) Unbookmark(ctx context.Context, callerID, postID uuid.UUID) error {
	if f.unbookmarkFn != nil {
		return f.unbookmarkFn(ctx, callerID, postID)
	}
	return nil
}

func (f *fakeBookmarkService) ListBookmarks(ctx context.Context, callerID uuid.UUID, cursorStr string) (BookmarkPage, error) {
	if f.listBookmarksFn != nil {
		return f.listBookmarksFn(ctx, callerID, cursorStr)
	}
	return BookmarkPage{Items: []BookmarkDTO{}, Terminated: true}, nil
}

// ---------------------------------------------------------------------------
// testableBookmarkHandler — handler wired to a bookmarkSvc interface
// ---------------------------------------------------------------------------

type testableBookmarkHandler struct {
	svc bookmarkSvc
	log *zap.Logger
}

func newTestableBookmarkHandler(svc bookmarkSvc) *testableBookmarkHandler {
	log, _ := zap.NewDevelopment()
	return &testableBookmarkHandler{svc: svc, log: log}
}

func (h *testableBookmarkHandler) addBookmark(w http.ResponseWriter, r *http.Request) {
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
	if err := h.svc.Bookmark(r.Context(), callerID, postID); err != nil {
		h.handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *testableBookmarkHandler) removeBookmark(w http.ResponseWriter, r *http.Request) {
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
	if err := h.svc.Unbookmark(r.Context(), callerID, postID); err != nil {
		h.handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *testableBookmarkHandler) listBookmarks(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}
	cursor := r.URL.Query().Get("cursor")
	page, err := h.svc.ListBookmarks(r.Context(), callerID, cursor)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *testableBookmarkHandler) handleServiceError(w http.ResponseWriter, err error) {
	var apiErr *apierror.APIError
	if ae, ok := err.(*apierror.APIError); ok {
		apiErr = ae
	}
	if apiErr == nil {
		apierror.Render(w, http.StatusInternalServerError,
			apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
		return
	}
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
		apierror.Render(w, http.StatusTooManyRequests, apiErr.ToResponse())
	default:
		apierror.Render(w, http.StatusInternalServerError,
			apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
	}
}

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

var testJWTSecret = []byte("bookmark-handler-test-secret-32b!")

func makeAuthedRequest(t *testing.T, method, target string, userID uuid.UUID) *http.Request {
	t.Helper()
	tokenStr, err := auth.GenerateAccessToken(userID, testJWTSecret)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	req := httptest.NewRequest(method, target, nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	return req
}

func errorCodeFromBody(t *testing.T, body []byte) string {
	t.Helper()
	var resp apierror.ErrorResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("could not unmarshal error response: %v\nbody: %s", err, body)
	}
	return resp.Error.Code
}

func newChiContextWithParam(r *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func applyJWT(h http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	auth.JWTMiddleware(testJWTSecret)(h).ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// TestBookmarkHandler_Unauthenticated
// ---------------------------------------------------------------------------

// TestBookmarkHandler_Unauthenticated verifies that POST /posts/{postID}/bookmark
// without a JWT returns 401.
func TestBookmarkHandler_Unauthenticated(t *testing.T) {
	h := newTestableBookmarkHandler(&fakeBookmarkService{})

	postID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/posts/"+postID.String()+"/bookmark", nil)
	req = newChiContextWithParam(req, "postID", postID.String())
	rec := httptest.NewRecorder()

	h.addBookmark(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if code := errorCodeFromBody(t, rec.Body.Bytes()); code != apierror.CodeUnauthorized {
		t.Errorf("error code = %q, want %q", code, apierror.CodeUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// TestBookmarkHandler_RateLimitNilRedis
// ---------------------------------------------------------------------------

// TestBookmarkHandler_RateLimitNilRedis verifies that passing a nil Redis
// client to bookmarkRateLimitMiddleware causes the middleware to fail closed
// with HTTP 503. This mirrors the fail-closed invariant from auth and post packages
// (CLAUDE.md §12).
func TestBookmarkHandler_RateLimitNilRedis(t *testing.T) {
	log, _ := zap.NewDevelopment()

	called := false
	sentinel := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	userID := uuid.New()
	rl := bookmarkRateLimitMiddleware(nil, log)
	chain := auth.JWTMiddleware(testJWTSecret)(rl(sentinel))

	req := makeAuthedRequest(t, http.MethodPost, "/posts/"+uuid.New().String()+"/bookmark", userID)
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
// TestGetBookmarks_Unauthenticated
// ---------------------------------------------------------------------------

// TestGetBookmarks_Unauthenticated verifies that GET /me/bookmarks without a
// JWT returns 401.
func TestGetBookmarks_Unauthenticated(t *testing.T) {
	h := newTestableBookmarkHandler(&fakeBookmarkService{})

	req := httptest.NewRequest(http.MethodGet, "/me/bookmarks", nil)
	rec := httptest.NewRecorder()

	h.listBookmarks(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if code := errorCodeFromBody(t, rec.Body.Bytes()); code != apierror.CodeUnauthorized {
		t.Errorf("error code = %q, want %q", code, apierror.CodeUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// TestBookmarkDTO_NoMetricFields
// ---------------------------------------------------------------------------

// forbiddenBookmarkMetricSubstrings are the field substrings that must not
// appear in BookmarkDTO or its nested PostDTO (CLAUDE.md §2.3).
var forbiddenBookmarkMetricSubstrings = []string{
	"count",
	"like",
	"impression",
	"follower",
	"share",
	"repost",
	"retweet",
	"view",
	"reach",
	"engagement",
}

// bookmarkDTOFieldNames returns all field names (lowercase) and json tag values
// found on a struct type, recursing into nested structs.
func bookmarkDTOFieldNames(t reflect.Type) (names []string, tags []string) {
	for i := range t.NumField() {
		f := t.Field(i)

		ft := f.Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct {
			subNames, subTags := bookmarkDTOFieldNames(ft)
			names = append(names, subNames...)
			tags = append(tags, subTags...)
		}

		names = append(names, strings.ToLower(f.Name))

		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		if comma := strings.Index(tag, ","); comma >= 0 {
			tag = tag[:comma]
		}
		tags = append(tags, tag)
	}
	return names, tags
}

// TestBookmarkDTO_NoMetricFields verifies via reflection that BookmarkDTO
// contains no field that could expose social-validation metrics publicly
// (CLAUDE.md §2.3).
func TestBookmarkDTO_NoMetricFields(t *testing.T) {
	names, tags := bookmarkDTOFieldNames(reflect.TypeOf(BookmarkDTO{}))

	for _, forbidden := range forbiddenBookmarkMetricSubstrings {
		for _, name := range names {
			if strings.Contains(name, forbidden) {
				t.Errorf("BookmarkDTO field name %q contains forbidden substring %q — violates public metric lockdown (CLAUDE.md §2.3)", name, forbidden)
			}
		}
		for _, tag := range tags {
			if strings.Contains(tag, forbidden) {
				t.Errorf("BookmarkDTO json tag %q contains forbidden substring %q — violates public metric lockdown (CLAUDE.md §2.3)", tag, forbidden)
			}
		}
	}
}

// TestBookmarkDTO_NotNestedInPostDTO is a compile-time-enforced contract:
// post.PostDTO must NOT have a field of type BookmarkDTO or *BookmarkDTO.
// This test verifies via reflection that no such field exists.
func TestBookmarkDTO_NotNestedInPostDTO(t *testing.T) {
	// We import post indirectly through BookmarkDTO.Post (type post.PostDTO).
	// Use reflection on BookmarkDTO to reach the PostDTO type, then check
	// that PostDTO has no BookmarkDTO field.
	bdt := reflect.TypeOf(BookmarkDTO{})
	var postDTOType reflect.Type
	for i := range bdt.NumField() {
		f := bdt.Field(i)
		if f.Name == "Post" {
			postDTOType = f.Type
			break
		}
	}
	if postDTOType == nil {
		t.Fatal("BookmarkDTO does not have a Post field — struct changed unexpectedly")
	}

	bookmarkDTOType := reflect.TypeOf(BookmarkDTO{})
	for i := range postDTOType.NumField() {
		f := postDTOType.Field(i)
		ft := f.Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if ft == bookmarkDTOType {
			t.Errorf("post.PostDTO field %q has type BookmarkDTO — BookmarkDTO must NEVER be nested in PostDTO (CLAUDE.md §2.3)", f.Name)
		}
	}
}
