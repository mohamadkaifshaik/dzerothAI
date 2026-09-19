package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/auth"
)

// Handler exposes the report HTTP endpoints.
type Handler struct {
	svc    *Service
	logger *zap.Logger
}

// NewHandler constructs a report Handler.
func NewHandler(svc *Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// RegisterRoutes mounts all report routes on the provided chi.Router.
//
// Routes (registered under the /api/v1 sub-router passed from main.go):
//
//	POST /posts/{postID}/report  — authenticated (JWT)
//	POST /users/{userID}/report  — authenticated (JWT)
//
// Both routes return 204 No Content on success. The response body is always
// empty — reporter identity is never returned.
func (h *Handler) RegisterRoutes(r chi.Router, jwtSecret []byte) {
	r.With(auth.JWTMiddleware(jwtSecret)).Post("/posts/{postID}/report", h.reportPost)
	r.With(auth.JWTMiddleware(jwtSecret)).Post("/users/{userID}/report", h.reportUser)
}

// reportPost handles POST /api/v1/posts/{postID}/report.
func (h *Handler) reportPost(w http.ResponseWriter, r *http.Request) {
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
		apierror.Render(w, http.StatusBadRequest,
			apierror.New(apierror.CodeValidation, "Invalid JSON body."))
		return
	}

	if err := h.svc.SubmitPostReport(r.Context(), reporterID, postID, req); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// reportUser handles POST /api/v1/users/{userID}/report.
func (h *Handler) reportUser(w http.ResponseWriter, r *http.Request) {
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
		apierror.Render(w, http.StatusBadRequest,
			apierror.New(apierror.CodeValidation, "Invalid JSON body."))
		return
	}

	if err := h.svc.SubmitUserReport(r.Context(), reporterID, targetUserID, req); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleServiceError maps *apierror.APIError and other service errors to HTTP responses.
func (h *Handler) handleServiceError(w http.ResponseWriter, err error) {
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
		case apierror.CodeServiceUnavailable:
			apierror.Render(w, http.StatusServiceUnavailable, apiErr.ToResponse())
		default:
			h.logger.Error("report: unexpected internal error", zap.Error(err))
			apierror.Render(w, http.StatusInternalServerError,
				apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
		}
		return
	}

	h.logger.Error("report: unexpected internal error", zap.Error(err))
	apierror.Render(w, http.StatusInternalServerError,
		apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
}

// parseUUIDParam extracts and parses a UUID path parameter.
// Writes a 400 response and returns false if the param is missing or invalid.
func parseUUIDParam(w http.ResponseWriter, r *http.Request, param string) (uuid.UUID, bool) {
	raw := chi.URLParam(r, param)
	id, err := uuid.Parse(raw)
	if err != nil {
		apierror.Render(w, http.StatusBadRequest,
			apierror.New(apierror.CodeValidation, fmt.Sprintf("invalid %s format", param)))
		return uuid.UUID{}, false
	}
	return id, true
}
