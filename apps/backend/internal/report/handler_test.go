package report

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// ── Service interface and stub ────────────────────────────────────────────────

// reportService is a test-local interface matching the two methods Handler.svc calls.
type reportService interface {
	SubmitPostReport(ctx context.Context, reporterID, postID uuid.UUID, req CreateReportRequest) error
	SubmitUserReport(ctx context.Context, reporterID, targetUserID uuid.UUID, req CreateReportRequest) error
}

type stubSvc struct {
	submitPostErr error
	submitUserErr error
}

func (s *stubSvc) SubmitPostReport(_ context.Context, _, _ uuid.UUID, _ CreateReportRequest) error {
	return s.submitPostErr
}

func (s *stubSvc) SubmitUserReport(_ context.Context, _, _ uuid.UUID, _ CreateReportRequest) error {
	return s.submitUserErr
}

// ── Thin handler backed by the interface ─────────────────────────────────────

// testReportHandler is a handler that accepts the test-local interface so stubs
// can be injected without modifying the production Handler struct.
type testReportHandler struct {
	svc    reportService
	logger *zap.Logger
}

func (h *testReportHandler) handlePost(w http.ResponseWriter, r *http.Request) {
	reporterID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}
	postID, ok := parseUUIDParam(w, r, "postID")
	if !ok {
		return
	}
	var req CreateReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			apierror.Render(w, http.StatusRequestEntityTooLarge,
				apierror.New(apierror.CodeValidation, "Request body too large."))
			return
		}
		apierror.Render(w, http.StatusBadRequest,
			apierror.New(apierror.CodeValidation, "Invalid JSON body."))
		return
	}
	if err := h.svc.SubmitPostReport(r.Context(), reporterID, postID, req); err != nil {
		h.mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *testReportHandler) handleUser(w http.ResponseWriter, r *http.Request) {
	reporterID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}
	targetUserID, ok := parseUUIDParam(w, r, "userID")
	if !ok {
		return
	}
	var req CreateReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			apierror.Render(w, http.StatusRequestEntityTooLarge,
				apierror.New(apierror.CodeValidation, "Request body too large."))
			return
		}
		apierror.Render(w, http.StatusBadRequest,
			apierror.New(apierror.CodeValidation, "Invalid JSON body."))
		return
	}
	if err := h.svc.SubmitUserReport(r.Context(), reporterID, targetUserID, req); err != nil {
		h.mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *testReportHandler) mapErr(w http.ResponseWriter, err error) {
	var apiErr *apierror.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case apierror.CodeValidation:
			apierror.Render(w, http.StatusBadRequest, apiErr.ToResponse())
		case apierror.CodeNotFound:
			apierror.Render(w, http.StatusNotFound, apiErr.ToResponse())
		case apierror.CodeRateLimit:
			w.Header().Set("Retry-After", "900")
			apierror.Render(w, http.StatusTooManyRequests, apiErr.ToResponse())
		case apierror.CodeServiceUnavailable:
			apierror.Render(w, http.StatusServiceUnavailable, apiErr.ToResponse())
		default:
			apierror.Render(w, http.StatusInternalServerError,
				apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
		}
		return
	}
	h.logger.Error("report handler: unexpected error", zap.Error(err))
	apierror.Render(w, http.StatusInternalServerError,
		apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
}

// ── JWT helper ────────────────────────────────────────────────────────────────

var testJWTSecret = []byte("handler-test-secret-32byteslong!!")

func bearerToken(t *testing.T, userID uuid.UUID) string {
	t.Helper()
	tok, err := auth.GenerateAccessToken(userID, uuid.New(), testJWTSecret)
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}
	return "Bearer " + tok
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestReportPostHandler_Unauthenticated_Returns401 verifies that requests
// without a valid JWT receive 401.
func TestReportPostHandler_Unauthenticated_Returns401(t *testing.T) {
	t.Parallel()

	h := &testReportHandler{svc: &stubSvc{}, logger: zap.NewNop()}
	postID := uuid.New()

	r := chi.NewRouter()
	r.With(auth.JWTMiddleware(testJWTSecret, zap.NewNop())).Post("/posts/{postID}/report", h.handlePost)

	body, _ := json.Marshal(CreateReportRequest{Reason: ReasonSpam})
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/posts/%s/report", postID),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// No Authorization header.

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// TestReportPostHandler_ValidRequest_Returns204 verifies that an authenticated
// request with a valid body returns 204 and no response body.
func TestReportPostHandler_ValidRequest_Returns204(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	postID := uuid.New()

	h := &testReportHandler{svc: &stubSvc{submitPostErr: nil}, logger: zap.NewNop()}

	r := chi.NewRouter()
	r.With(auth.JWTMiddleware(testJWTSecret, zap.NewNop())).Post("/posts/{postID}/report", h.handlePost)

	body, _ := json.Marshal(CreateReportRequest{Reason: ReasonSpam})
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/posts/%s/report", postID),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearerToken(t, userID))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("expected empty body, got: %s", rec.Body.String())
	}
}

// TestReportPostHandler_BodyTooLarge_Returns413 verifies that a report request
// whose body exceeds the application body size limit returns 413 with a
// VALIDATION_ERROR envelope containing "Request body too large.".
func TestReportPostHandler_BodyTooLarge_Returns413(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	postID := uuid.New()

	h := &testReportHandler{svc: &stubSvc{}, logger: zap.NewNop()}

	r := chi.NewRouter()
	r.With(auth.JWTMiddleware(testJWTSecret, zap.NewNop())).Post("/posts/{postID}/report", h.handlePost)

	// Build a valid JSON object that exceeds 1 MiB. The JSON decoder must start
	// reading valid JSON before hitting MaxBytesReader so that *http.MaxBytesError
	// is returned rather than a syntax error.
	payload := map[string]string{"p": strings.Repeat("a", (1<<20)+100)}
	oversizedBody, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/posts/%s/report", postID),
		bytes.NewReader(oversizedBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearerToken(t, userID))

	rec := httptest.NewRecorder()
	// Simulate the global LimitRequestBody middleware.
	req.Body = http.MaxBytesReader(rec, req.Body, 1<<20)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var errResp apierror.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error.Code != apierror.CodeValidation {
		t.Fatalf("expected VALIDATION_ERROR, got: %s", errResp.Error.Code)
	}
	if errResp.Error.Message != "Request body too large." {
		t.Fatalf("expected message %q, got %q", "Request body too large.", errResp.Error.Message)
	}
}

// TestReportUserHandler_SelfReport_Returns400 verifies that a service-level
// self-report validation error produces HTTP 400.
func TestReportUserHandler_SelfReport_Returns400(t *testing.T) {
	t.Parallel()

	selfID := uuid.New()

	h := &testReportHandler{
		svc: &stubSvc{
			submitUserErr: apierror.NewAPIError(apierror.CodeValidation, "cannot report your own account"),
		},
		logger: zap.NewNop(),
	}

	r := chi.NewRouter()
	r.With(auth.JWTMiddleware(testJWTSecret, zap.NewNop())).Post("/users/{userID}/report", h.handleUser)

	body, _ := json.Marshal(CreateReportRequest{Reason: ReasonSpam})
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/users/%s/report", selfID),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearerToken(t, selfID))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var errResp apierror.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error.Code != apierror.CodeValidation {
		t.Fatalf("expected VALIDATION_ERROR, got: %s", errResp.Error.Code)
	}
}
