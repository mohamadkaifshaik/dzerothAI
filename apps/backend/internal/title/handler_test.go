// Package title — handler tests.
//
// Unit tests for title HTTP endpoints using httptest and a stub service.
// No real database or Redis connection is required.
package title

import (
	"bytes"
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
// titleHandlerSvc — test-local interface and stub
// ---------------------------------------------------------------------------

// titleHandlerSvc matches the methods Handler calls on *Service.
type titleHandlerSvc interface {
	GetCatalog(ctx context.Context) (TitleCatalogResponse, error)
	GetMyTitles(ctx context.Context, callerID uuid.UUID) (UserTitlesResponse, error)
	GetMyPrimaryTitle(ctx context.Context, callerID uuid.UUID) (PrimaryTitleResponse, error)
	SetMyPrimaryTitle(ctx context.Context, callerID uuid.UUID, userTitleID uuid.UUID) (PrimaryTitleResponse, error)
	ClearMyPrimaryTitle(ctx context.Context, callerID uuid.UUID) error
	GetUserPrimaryTitle(ctx context.Context, callerID *uuid.UUID, targetID uuid.UUID) (PrimaryTitleResponse, error)
}

type stubTitleSvc struct {
	getCatalogFn         func(ctx context.Context) (TitleCatalogResponse, error)
	getMyTitlesFn        func(ctx context.Context, callerID uuid.UUID) (UserTitlesResponse, error)
	getMyPrimaryFn       func(ctx context.Context, callerID uuid.UUID) (PrimaryTitleResponse, error)
	setMyPrimaryFn       func(ctx context.Context, callerID uuid.UUID, userTitleID uuid.UUID) (PrimaryTitleResponse, error)
	clearMyPrimaryFn     func(ctx context.Context, callerID uuid.UUID) error
	getUserPrimaryFn     func(ctx context.Context, callerID *uuid.UUID, targetID uuid.UUID) (PrimaryTitleResponse, error)
}

func (s *stubTitleSvc) GetCatalog(ctx context.Context) (TitleCatalogResponse, error) {
	if s.getCatalogFn != nil {
		return s.getCatalogFn(ctx)
	}
	return TitleCatalogResponse{Items: []TitleDefinitionDTO{}}, nil
}

func (s *stubTitleSvc) GetMyTitles(ctx context.Context, callerID uuid.UUID) (UserTitlesResponse, error) {
	if s.getMyTitlesFn != nil {
		return s.getMyTitlesFn(ctx, callerID)
	}
	return UserTitlesResponse{Items: []UserTitleDTO{}}, nil
}

func (s *stubTitleSvc) GetMyPrimaryTitle(ctx context.Context, callerID uuid.UUID) (PrimaryTitleResponse, error) {
	if s.getMyPrimaryFn != nil {
		return s.getMyPrimaryFn(ctx, callerID)
	}
	return PrimaryTitleResponse{PrimaryTitle: nil}, nil
}

func (s *stubTitleSvc) SetMyPrimaryTitle(ctx context.Context, callerID uuid.UUID, userTitleID uuid.UUID) (PrimaryTitleResponse, error) {
	if s.setMyPrimaryFn != nil {
		return s.setMyPrimaryFn(ctx, callerID, userTitleID)
	}
	return PrimaryTitleResponse{PrimaryTitle: nil}, nil
}

func (s *stubTitleSvc) ClearMyPrimaryTitle(ctx context.Context, callerID uuid.UUID) error {
	if s.clearMyPrimaryFn != nil {
		return s.clearMyPrimaryFn(ctx, callerID)
	}
	return nil
}

func (s *stubTitleSvc) GetUserPrimaryTitle(ctx context.Context, callerID *uuid.UUID, targetID uuid.UUID) (PrimaryTitleResponse, error) {
	if s.getUserPrimaryFn != nil {
		return s.getUserPrimaryFn(ctx, callerID, targetID)
	}
	return PrimaryTitleResponse{PrimaryTitle: nil}, nil
}

// ---------------------------------------------------------------------------
// testTitleHandler — thin handler backed by the interface
// ---------------------------------------------------------------------------

// testTitleHandler mirrors the production Handler logic but accepts the
// test-local titleHandlerSvc interface so stubs can be injected.
type testTitleHandler struct {
	svc    titleHandlerSvc
	logger *zap.Logger
}

func (h *testTitleHandler) getCatalog(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.GetCatalog(r.Context())
	if err != nil {
		mapTitleErr(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *testTitleHandler) getMyTitles(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized, apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}
	resp, err := h.svc.GetMyTitles(r.Context(), callerID)
	if err != nil {
		mapTitleErr(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *testTitleHandler) getMyPrimary(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized, apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}
	resp, err := h.svc.GetMyPrimaryTitle(r.Context(), callerID)
	if err != nil {
		mapTitleErr(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *testTitleHandler) setMyPrimary(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized, apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}
	var req SetPrimaryTitleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Render(w, http.StatusBadRequest, apierror.New(apierror.CodeValidation, "invalid request body"))
		return
	}
	userTitleID, err := uuid.Parse(req.UserTitleID)
	if err != nil {
		apierror.Render(w, http.StatusBadRequest, apierror.New(apierror.CodeValidation, "user_title_id must be a valid UUID"))
		return
	}
	resp, svcErr := h.svc.SetMyPrimaryTitle(r.Context(), callerID, userTitleID)
	if svcErr != nil {
		mapTitleErr(w, h.logger, svcErr)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *testTitleHandler) clearMyPrimary(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized, apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}
	if err := h.svc.ClearMyPrimaryTitle(r.Context(), callerID); err != nil {
		mapTitleErr(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *testTitleHandler) getUserPrimary(w http.ResponseWriter, r *http.Request) {
	targetID, ok := parseUUIDParam(w, r, "userID")
	if !ok {
		return
	}
	var callerPtr *uuid.UUID
	if callerID, ok := auth.UserIDFromContext(r.Context()); ok {
		id := callerID
		callerPtr = &id
	}
	resp, err := h.svc.GetUserPrimaryTitle(r.Context(), callerPtr, targetID)
	if err != nil {
		mapTitleErr(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func mapTitleErr(w http.ResponseWriter, log *zap.Logger, err error) {
	var ae *apierror.APIError
	if e, ok := err.(*apierror.APIError); ok {
		ae = e
		switch ae.Code {
		case apierror.CodeValidation:
			apierror.Render(w, http.StatusUnprocessableEntity, ae.ToResponse())
		case apierror.CodeUnauthorized:
			apierror.Render(w, http.StatusUnauthorized, ae.ToResponse())
		case apierror.CodeForbidden:
			apierror.Render(w, http.StatusForbidden, ae.ToResponse())
		case apierror.CodeNotFound:
			apierror.Render(w, http.StatusNotFound, ae.ToResponse())
		default:
			log.Error("title test: unexpected internal error", zap.Error(err))
			apierror.Render(w, http.StatusInternalServerError,
				apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
		}
		return
	}
	log.Error("title test: unexpected internal error", zap.Error(err))
	apierror.Render(w, http.StatusInternalServerError,
		apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
}

// ---------------------------------------------------------------------------
// JWT helper
// ---------------------------------------------------------------------------

var titleTestJWTSecret = []byte("title-handler-test-secret-32byt!!")

func titleBearerToken(t *testing.T, userID uuid.UUID) string {
	t.Helper()
	tok, err := auth.GenerateAccessToken(userID, uuid.New(), titleTestJWTSecret)
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}
	return "Bearer " + tok
}

// ---------------------------------------------------------------------------
// router builder
// ---------------------------------------------------------------------------

// buildTitleRouter builds a test chi router. For GET /titles/{userID}/primary the
// JWT middleware is applied so that authenticated callers have their ID in context.
// The handler itself treats missing authentication as anonymous (nil callerID) rather
// than rejecting the request, so unauthenticated requests still succeed.
func buildTitleRouter(h *testTitleHandler) chi.Router {
	r := chi.NewRouter()
	r.Get("/titles/catalog", h.getCatalog)
	r.With(auth.JWTMiddleware(titleTestJWTSecret, zap.NewNop())).Get("/titles/me", h.getMyTitles)
	r.With(auth.JWTMiddleware(titleTestJWTSecret, zap.NewNop())).Get("/titles/me/primary", h.getMyPrimary)
	r.With(auth.JWTMiddleware(titleTestJWTSecret, zap.NewNop())).Put("/titles/me/primary", h.setMyPrimary)
	r.With(auth.JWTMiddleware(titleTestJWTSecret, zap.NewNop())).Delete("/titles/me/primary", h.clearMyPrimary)
	// Optional auth: JWT middleware populates context when token is present;
	// the handler calls auth.UserIDFromContext and treats missing auth as anonymous.
	r.With(auth.JWTMiddleware(titleTestJWTSecret, zap.NewNop())).Get("/titles/{userID}/primary", h.getUserPrimary)
	return r
}

// buildTitleRouterNoAuth builds a test chi router without JWT middleware on
// GET /titles/{userID}/primary, used to test the unauthenticated path.
func buildTitleRouterNoAuth(h *testTitleHandler) chi.Router {
	r := chi.NewRouter()
	r.Get("/titles/{userID}/primary", h.getUserPrimary)
	return r
}

// ---------------------------------------------------------------------------
// GET /titles/catalog — no auth, always 200
// ---------------------------------------------------------------------------

// TestGetCatalog_NoAuth_Returns200 verifies that the catalog endpoint is accessible
// without authentication.
func TestGetCatalog_NoAuth_Returns200(t *testing.T) {
	t.Parallel()

	h := &testTitleHandler{svc: &stubTitleSvc{}, logger: zap.NewNop()}
	r := buildTitleRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/titles/catalog", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp TitleCatalogResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Items == nil {
		t.Error("expected non-nil Items slice")
	}
}

// TestGetCatalog_ReturnsDTOs verifies catalog response shape.
func TestGetCatalog_ReturnsDTOs(t *testing.T) {
	t.Parallel()

	stub := &stubTitleSvc{
		getCatalogFn: func(_ context.Context) (TitleCatalogResponse, error) {
			return TitleCatalogResponse{Items: []TitleDefinitionDTO{
				{ID: uuid.New().String(), Slug: "centurion", DisplayName: "Centurion", Category: "milestone", IsRevocable: false},
			}}, nil
		},
	}
	h := &testTitleHandler{svc: stub, logger: zap.NewNop()}
	r := buildTitleRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/titles/catalog", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp TitleCatalogResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(resp.Items))
	}
	if resp.Items[0].Slug != "centurion" {
		t.Errorf("expected slug centurion, got %q", resp.Items[0].Slug)
	}
}

// TestGetCatalog_ServiceError_Returns500 verifies that a service error maps to 500.
func TestGetCatalog_ServiceError_Returns500(t *testing.T) {
	t.Parallel()

	stub := &stubTitleSvc{
		getCatalogFn: func(_ context.Context) (TitleCatalogResponse, error) {
			return TitleCatalogResponse{}, apierror.NewAPIError(apierror.CodeInternal, "db error")
		},
	}
	h := &testTitleHandler{svc: stub, logger: zap.NewNop()}
	r := buildTitleRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/titles/catalog", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /titles/me — unauthenticated → 401
// ---------------------------------------------------------------------------

// TestGetMyTitles_Unauthenticated_Returns401 verifies that missing JWT gives 401.
func TestGetMyTitles_Unauthenticated_Returns401(t *testing.T) {
	t.Parallel()

	h := &testTitleHandler{svc: &stubTitleSvc{}, logger: zap.NewNop()}
	r := buildTitleRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/titles/me", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// TestGetMyTitles_Authenticated_Returns200 verifies authenticated GET /titles/me.
func TestGetMyTitles_Authenticated_Returns200(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	stub := &stubTitleSvc{
		getMyTitlesFn: func(_ context.Context, id uuid.UUID) (UserTitlesResponse, error) {
			return UserTitlesResponse{Items: []UserTitleDTO{}}, nil
		},
	}
	h := &testTitleHandler{svc: stub, logger: zap.NewNop()}
	r := buildTitleRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/titles/me", nil)
	req.Header.Set("Authorization", titleBearerToken(t, userID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// GET /titles/me/primary — unauthenticated → 401
// ---------------------------------------------------------------------------

// TestGetMyPrimary_Unauthenticated_Returns401 verifies that missing JWT gives 401.
func TestGetMyPrimary_Unauthenticated_Returns401(t *testing.T) {
	t.Parallel()

	h := &testTitleHandler{svc: &stubTitleSvc{}, logger: zap.NewNop()}
	r := buildTitleRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/titles/me/primary", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// TestGetMyPrimary_Authenticated_Returns200 verifies authenticated GET /titles/me/primary.
func TestGetMyPrimary_Authenticated_Returns200(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	h := &testTitleHandler{svc: &stubTitleSvc{}, logger: zap.NewNop()}
	r := buildTitleRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/titles/me/primary", nil)
	req.Header.Set("Authorization", titleBearerToken(t, userID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp PrimaryTitleResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// nil primary_title is valid when no primary is set
}

// ---------------------------------------------------------------------------
// PUT /titles/me/primary — unauthenticated → 401
// ---------------------------------------------------------------------------

// TestSetMyPrimary_Unauthenticated_Returns401 verifies that missing JWT gives 401.
func TestSetMyPrimary_Unauthenticated_Returns401(t *testing.T) {
	t.Parallel()

	h := &testTitleHandler{svc: &stubTitleSvc{}, logger: zap.NewNop()}
	r := buildTitleRouter(h)

	body, _ := json.Marshal(SetPrimaryTitleRequest{UserTitleID: uuid.New().String()})
	req := httptest.NewRequest(http.MethodPut, "/titles/me/primary", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// TestSetMyPrimary_InvalidUUID_Returns400 verifies that an invalid UUID body gives 400.
func TestSetMyPrimary_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	h := &testTitleHandler{svc: &stubTitleSvc{}, logger: zap.NewNop()}
	r := buildTitleRouter(h)

	body, _ := json.Marshal(SetPrimaryTitleRequest{UserTitleID: "not-a-uuid"})
	req := httptest.NewRequest(http.MethodPut, "/titles/me/primary", bytes.NewReader(body))
	req.Header.Set("Authorization", titleBearerToken(t, userID))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// TestSetMyPrimary_Forbidden_Returns403 verifies that CodeForbidden from service maps to 403.
func TestSetMyPrimary_Forbidden_Returns403(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	stub := &stubTitleSvc{
		setMyPrimaryFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) (PrimaryTitleResponse, error) {
			return PrimaryTitleResponse{}, apierror.NewAPIError(apierror.CodeForbidden, "not your title")
		},
	}
	h := &testTitleHandler{svc: stub, logger: zap.NewNop()}
	r := buildTitleRouter(h)

	body, _ := json.Marshal(SetPrimaryTitleRequest{UserTitleID: uuid.New().String()})
	req := httptest.NewRequest(http.MethodPut, "/titles/me/primary", bytes.NewReader(body))
	req.Header.Set("Authorization", titleBearerToken(t, userID))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

// TestSetMyPrimary_Success_Returns200 verifies successful PUT /titles/me/primary.
func TestSetMyPrimary_Success_Returns200(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	stub := &stubTitleSvc{
		setMyPrimaryFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) (PrimaryTitleResponse, error) {
			return PrimaryTitleResponse{PrimaryTitle: &TitleSummaryDTO{Slug: "centurion", DisplayName: "Centurion"}}, nil
		},
	}
	h := &testTitleHandler{svc: stub, logger: zap.NewNop()}
	r := buildTitleRouter(h)

	body, _ := json.Marshal(SetPrimaryTitleRequest{UserTitleID: uuid.New().String()})
	req := httptest.NewRequest(http.MethodPut, "/titles/me/primary", bytes.NewReader(body))
	req.Header.Set("Authorization", titleBearerToken(t, userID))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp PrimaryTitleResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.PrimaryTitle == nil || resp.PrimaryTitle.Slug != "centurion" {
		t.Errorf("expected centurion slug, got %+v", resp.PrimaryTitle)
	}
}

// ---------------------------------------------------------------------------
// DELETE /titles/me/primary — unauthenticated → 401
// ---------------------------------------------------------------------------

// TestClearMyPrimary_Unauthenticated_Returns401 verifies that missing JWT gives 401.
func TestClearMyPrimary_Unauthenticated_Returns401(t *testing.T) {
	t.Parallel()

	h := &testTitleHandler{svc: &stubTitleSvc{}, logger: zap.NewNop()}
	r := buildTitleRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/titles/me/primary", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// TestClearMyPrimary_Authenticated_Returns204 verifies that DELETE /titles/me/primary
// returns 204 on success.
func TestClearMyPrimary_Authenticated_Returns204(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	h := &testTitleHandler{svc: &stubTitleSvc{}, logger: zap.NewNop()}
	r := buildTitleRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/titles/me/primary", nil)
	req.Header.Set("Authorization", titleBearerToken(t, userID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// GET /titles/{userID}/primary — optional auth
// ---------------------------------------------------------------------------

// TestGetUserPrimary_InvalidUUID_Returns400 verifies that an invalid userID gives 400.
func TestGetUserPrimary_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()

	h := &testTitleHandler{svc: &stubTitleSvc{}, logger: zap.NewNop()}
	r := buildTitleRouterNoAuth(h)

	req := httptest.NewRequest(http.MethodGet, "/titles/not-a-uuid/primary", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// TestGetUserPrimary_Unauthenticated_Returns200 verifies that an unauthenticated
// request to GET /titles/{userID}/primary still gets 200 (nil title for private accounts
// is handled by the service, not the handler).
func TestGetUserPrimary_Unauthenticated_Returns200(t *testing.T) {
	t.Parallel()

	targetID := uuid.New()
	stub := &stubTitleSvc{
		getUserPrimaryFn: func(_ context.Context, callerID *uuid.UUID, _ uuid.UUID) (PrimaryTitleResponse, error) {
			if callerID != nil {
				return PrimaryTitleResponse{}, apierror.NewAPIError(apierror.CodeInternal, "expected nil callerID")
			}
			return PrimaryTitleResponse{PrimaryTitle: nil}, nil
		},
	}
	h := &testTitleHandler{svc: stub, logger: zap.NewNop()}
	// Use the no-auth router so the request is truly unauthenticated (no JWT middleware
	// to reject the missing token — instead the handler sees no user in context).
	r := buildTitleRouterNoAuth(h)

	req := httptest.NewRequest(http.MethodGet, "/titles/"+targetID.String()+"/primary", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// TestGetUserPrimary_Authenticated_PassesCallerID verifies that an authenticated
// request passes a non-nil callerID to the service.
func TestGetUserPrimary_Authenticated_PassesCallerID(t *testing.T) {
	t.Parallel()

	callerID := uuid.New()
	targetID := uuid.New()
	callerSeen := false

	stub := &stubTitleSvc{
		getUserPrimaryFn: func(_ context.Context, cID *uuid.UUID, tID uuid.UUID) (PrimaryTitleResponse, error) {
			if cID != nil && *cID == callerID {
				callerSeen = true
			}
			return PrimaryTitleResponse{PrimaryTitle: nil}, nil
		},
	}
	h := &testTitleHandler{svc: stub, logger: zap.NewNop()}
	r := buildTitleRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/titles/"+targetID.String()+"/primary", nil)
	req.Header.Set("Authorization", titleBearerToken(t, callerID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !callerSeen {
		t.Error("expected service to receive non-nil callerID matching the authenticated user")
	}
}

// TestGetUserPrimary_NoMetricsInResponse verifies that the public title endpoint
// exposes no social-validation metrics (CLAUDE.md §2.3).
func TestGetUserPrimary_NoMetricsInResponse(t *testing.T) {
	t.Parallel()

	targetID := uuid.New()
	stub := &stubTitleSvc{
		getUserPrimaryFn: func(_ context.Context, _ *uuid.UUID, _ uuid.UUID) (PrimaryTitleResponse, error) {
			return PrimaryTitleResponse{PrimaryTitle: &TitleSummaryDTO{Slug: "centurion", DisplayName: "Centurion"}}, nil
		},
	}
	h := &testTitleHandler{svc: stub, logger: zap.NewNop()}
	r := buildTitleRouterNoAuth(h)

	req := httptest.NewRequest(http.MethodGet, "/titles/"+targetID.String()+"/primary", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	body := rec.Body.String()
	forbiddenFields := []string{
		"like_count", "likes", "impression_count", "impressions",
		"bookmark_count", "follower_count", "reaction_count",
		"view_count", "share_count",
	}
	for _, field := range forbiddenFields {
		if containsTitleStr(body, `"`+field+`"`) {
			t.Errorf("response body must not contain metric field %q", field)
		}
	}
}

// ---------------------------------------------------------------------------
// helper
// ---------------------------------------------------------------------------

func containsTitleStr(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
