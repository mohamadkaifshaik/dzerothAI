package studio

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/auth"
)

// Handler exposes the private Creator Studio HTTP endpoints.
// All routes require JWT authentication. The callerID is extracted from the JWT
// context only — no path parameter is accepted.
type Handler struct {
	svc    *Service
	logger *zap.Logger
}

// NewHandler constructs a studio Handler.
func NewHandler(svc *Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// RegisterRoutes mounts the studio analytics route behind JWTMiddleware.
//
//	GET /me/studio/analytics — returns StudioPage (owner-only, JWT-scoped)
func (h *Handler) RegisterRoutes(r chi.Router, jwtSecret []byte) {
	r.With(auth.JWTMiddleware(jwtSecret)).Get("/me/studio/analytics", h.getAnalytics)
}

// getAnalytics handles GET /me/studio/analytics.
//
// Query params:
//
//	cursor (optional, base64url) — opaque pagination cursor
//
// Returns 200 with StudioPage JSON on success.
// Returns 401 when unauthenticated.
// Returns 429 when rate-limited.
// Returns 500 on internal error.
func (h *Handler) getAnalytics(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	cursor := r.URL.Query().Get("cursor")

	page, err := h.svc.GetStudioAnalytics(r.Context(), callerID, cursor)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, page)
}

// handleServiceError maps service-layer errors to appropriate HTTP responses.
func (h *Handler) handleServiceError(w http.ResponseWriter, err error) {
	if ae, ok := err.(*apierror.APIError); ok {
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
			apierror.Render(w, http.StatusInternalServerError,
				apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
		}
		return
	}

	h.logger.Error("studio: unexpected internal error", zap.Error(err))
	apierror.Render(w, http.StatusInternalServerError,
		apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
}

// writeJSON serializes v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, statusCode int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(v)
}
