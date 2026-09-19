package notification

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/auth"
)

// Handler exposes the notification HTTP endpoints.
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler constructs a notification Handler.
func NewHandler(svc *Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// RegisterRoutes mounts all notification routes on the provided chi.Router.
//
// Routes:
//
//	GET /me/notifications      — authenticated, owner-only, cursor-paginated
//	PUT /me/notifications/read — authenticated, marks all notifications as read
func (h *Handler) RegisterRoutes(r chi.Router, jwtSecret []byte) {
	r.With(auth.JWTMiddleware(jwtSecret, h.log)).Get("/me/notifications", h.listNotifications)
	r.With(auth.JWTMiddleware(jwtSecret, h.log)).Put("/me/notifications/read", h.markAllRead)
}

// listNotifications handles GET /api/v1/me/notifications.
// Owner-only: always uses JWT callerID, never a path-param user ID.
// Optional query param: cursor (opaque pagination token).
func (h *Handler) listNotifications(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	cursor := r.URL.Query().Get("cursor")

	page, err := h.svc.ListNotifications(r.Context(), callerID, cursor)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, page)
}

// markAllRead handles PUT /api/v1/me/notifications/read.
// Sets is_read = TRUE for all unread notifications owned by the caller.
// Returns 204 No Content on success.
func (h *Handler) markAllRead(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	if err := h.svc.MarkAllRead(r.Context(), callerID); err != nil {
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
		default:
			h.log.Error("notification: unexpected internal error", zap.Error(err))
			apierror.Render(w, http.StatusInternalServerError,
				apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
		}
		return
	}

	h.log.Error("notification: unexpected internal error", zap.Error(err))
	apierror.Render(w, http.StatusInternalServerError,
		apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
}

// writeJSON serializes v as JSON and writes it to w with the given status code.
// NOTE: writeJSON is intentionally kept local to each package.
// Factor into a shared helper in Phase 7 tech-debt cleanup.
func writeJSON(w http.ResponseWriter, statusCode int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(v)
}
