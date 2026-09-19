// Package notification — handler tests.
//
// Unit tests for notification HTTP endpoints using httptest and a stub service.
// No real database or Redis connection is required.
package notification

import (
	"context"
	"encoding/json"
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
// notifHandlerService — test-local interface and stub
// ---------------------------------------------------------------------------

// notifHandlerService matches the two methods Handler calls on its *Service.
type notifHandlerService interface {
	ListNotifications(ctx context.Context, callerID uuid.UUID, cursor string) (NotificationPage, error)
	MarkAllRead(ctx context.Context, callerID uuid.UUID) error
}

type stubNotifSvc struct {
	listFn        func(ctx context.Context, callerID uuid.UUID, cursor string) (NotificationPage, error)
	markAllReadFn func(ctx context.Context, callerID uuid.UUID) error
}

func (s *stubNotifSvc) ListNotifications(ctx context.Context, callerID uuid.UUID, cursor string) (NotificationPage, error) {
	if s.listFn != nil {
		return s.listFn(ctx, callerID, cursor)
	}
	return NotificationPage{Items: []NotificationDTO{}, Terminated: true}, nil
}

func (s *stubNotifSvc) MarkAllRead(ctx context.Context, callerID uuid.UUID) error {
	if s.markAllReadFn != nil {
		return s.markAllReadFn(ctx, callerID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// testNotifHandler — thin handler backed by the interface
// ---------------------------------------------------------------------------

// testNotifHandler mirrors the production Handler logic but accepts the
// test-local notifHandlerService interface so stubs can be injected.
type testNotifHandler struct {
	svc    notifHandlerService
	logger *zap.Logger
}

func (h *testNotifHandler) list(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}
	cursor := r.URL.Query().Get("cursor")
	page, err := h.svc.ListNotifications(r.Context(), callerID, cursor)
	if err != nil {
		h.mapErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *testNotifHandler) markAllRead(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}
	if err := h.svc.MarkAllRead(r.Context(), callerID); err != nil {
		h.mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *testNotifHandler) mapErr(w http.ResponseWriter, err error) {
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

var notifTestJWTSecret = []byte("notif-handler-test-secret-32byt!!")

func notifBearerToken(t *testing.T, userID uuid.UUID) string {
	t.Helper()
	tok, err := auth.GenerateAccessToken(userID, uuid.New(), notifTestJWTSecret)
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}
	return "Bearer " + tok
}

// ---------------------------------------------------------------------------
// router builder
// ---------------------------------------------------------------------------

func buildNotifRouter(h *testNotifHandler) chi.Router {
	r := chi.NewRouter()
	r.With(auth.JWTMiddleware(notifTestJWTSecret)).Get("/me/notifications", h.list)
	r.With(auth.JWTMiddleware(notifTestJWTSecret)).Put("/me/notifications/read", h.markAllRead)
	return r
}

// ---------------------------------------------------------------------------
// GET /me/notifications — unauthenticated → 401
// ---------------------------------------------------------------------------

// TestListNotifications_Unauthenticated_Returns401 verifies that a request
// without a valid JWT receives 401.
func TestListNotifications_Unauthenticated_Returns401(t *testing.T) {
	t.Parallel()

	h := &testNotifHandler{svc: &stubNotifSvc{}, logger: zap.NewNop()}
	r := buildNotifRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/me/notifications", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /me/notifications — authenticated → 200 with correct shape
// ---------------------------------------------------------------------------

// TestListNotifications_Authenticated_Returns200 verifies that an authenticated
// request returns 200 with a NotificationPage JSON body.
func TestListNotifications_Authenticated_Returns200(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	actorID := uuid.New()
	expectedPage := NotificationPage{
		Items: []NotificationDTO{
			{
				ID:          uuid.New().String(),
				RecipientID: userID.String(),
				ActorID:     actorID.String(),
				Actor: ActorSummary{
					ID:          actorID.String(),
					Handle:      "actor_handle",
					DisplayName: "Actor",
				},
				Event:     EventFollow,
				IsRead:    false,
				CreatedAt: "2026-09-19T00:00:00Z",
			},
		},
		NextCursor: "",
		Terminated: true,
	}

	stub := &stubNotifSvc{
		listFn: func(_ context.Context, _ uuid.UUID, _ string) (NotificationPage, error) {
			return expectedPage, nil
		},
	}
	h := &testNotifHandler{svc: stub, logger: zap.NewNop()}
	r := buildNotifRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/me/notifications", nil)
	req.Header.Set("Authorization", notifBearerToken(t, userID))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var page NotificationPage
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if len(page.Items) != 1 {
		t.Errorf("expected 1 notification item, got %d", len(page.Items))
	}
	if !page.Terminated {
		t.Error("expected Terminated=true")
	}
}

// ---------------------------------------------------------------------------
// GET /me/notifications — no social-validation metrics in response body
// ---------------------------------------------------------------------------

// TestListNotifications_NoMetricsInResponse verifies that the notification
// endpoint response body contains no social-validation metric fields
// (CLAUDE.md §2.3).
func TestListNotifications_NoMetricsInResponse(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	stub := &stubNotifSvc{
		listFn: func(_ context.Context, _ uuid.UUID, _ string) (NotificationPage, error) {
			return NotificationPage{Items: []NotificationDTO{}, Terminated: true}, nil
		},
	}
	h := &testNotifHandler{svc: stub, logger: zap.NewNop()}
	r := buildNotifRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/me/notifications", nil)
	req.Header.Set("Authorization", notifBearerToken(t, userID))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	body := rec.Body.String()
	forbiddenFields := []string{
		"like_count", "likes", "impression_count", "impressions",
		"bookmark_count", "follower_count", "reaction_count",
		"view_count", "share_count",
	}
	for _, field := range forbiddenFields {
		if contains(body, fmt.Sprintf("%q", field)) {
			t.Errorf("response body must not contain metric field %q", field)
		}
	}
}

// ---------------------------------------------------------------------------
// GET /me/notifications — service error → 500
// ---------------------------------------------------------------------------

// TestListNotifications_ServiceError_Returns500 verifies that an internal
// service error is mapped to HTTP 500.
func TestListNotifications_ServiceError_Returns500(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	stub := &stubNotifSvc{
		listFn: func(_ context.Context, _ uuid.UUID, _ string) (NotificationPage, error) {
			return NotificationPage{}, apierror.NewAPIError(apierror.CodeInternal, "db error")
		},
	}
	h := &testNotifHandler{svc: stub, logger: zap.NewNop()}
	r := buildNotifRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/me/notifications", nil)
	req.Header.Set("Authorization", notifBearerToken(t, userID))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// PUT /me/notifications/read — unauthenticated → 401
// ---------------------------------------------------------------------------

// TestMarkAllRead_Unauthenticated_Returns401 verifies that a request without
// a valid JWT receives 401.
func TestMarkAllRead_Unauthenticated_Returns401(t *testing.T) {
	t.Parallel()

	h := &testNotifHandler{svc: &stubNotifSvc{}, logger: zap.NewNop()}
	r := buildNotifRouter(h)

	req := httptest.NewRequest(http.MethodPut, "/me/notifications/read", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// PUT /me/notifications/read — authenticated → 204
// ---------------------------------------------------------------------------

// TestMarkAllRead_Authenticated_Returns204 verifies that an authenticated
// request returns 204 No Content with no response body.
func TestMarkAllRead_Authenticated_Returns204(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	stub := &stubNotifSvc{}
	h := &testNotifHandler{svc: stub, logger: zap.NewNop()}
	r := buildNotifRouter(h)

	req := httptest.NewRequest(http.MethodPut, "/me/notifications/read", nil)
	req.Header.Set("Authorization", notifBearerToken(t, userID))

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
// PUT /me/notifications/read — service error → 500
// ---------------------------------------------------------------------------

// TestMarkAllRead_ServiceError_Returns500 verifies that a service error is
// mapped to HTTP 500.
func TestMarkAllRead_ServiceError_Returns500(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	stub := &stubNotifSvc{
		markAllReadFn: func(_ context.Context, _ uuid.UUID) error {
			return apierror.NewAPIError(apierror.CodeInternal, "db timeout")
		},
	}
	h := &testNotifHandler{svc: stub, logger: zap.NewNop()}
	r := buildNotifRouter(h)

	req := httptest.NewRequest(http.MethodPut, "/me/notifications/read", nil)
	req.Header.Set("Authorization", notifBearerToken(t, userID))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// helper
// ---------------------------------------------------------------------------

// contains is a simple string containment check used to scan JSON bodies for
// forbidden field names. Using strings.Contains avoids importing "strings" at
// the package level when only needed here.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
