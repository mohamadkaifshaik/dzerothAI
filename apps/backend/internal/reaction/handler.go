package reaction

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/auth"
)

// Handler exposes the reaction HTTP endpoints.
type Handler struct {
	svc        *Service
	postLookup PostAuthorLookup
	log        *zap.Logger
}

// NewHandler constructs a reaction Handler.
// postLookup is used to resolve post author IDs for notification fan-out.
func NewHandler(svc *Service, postLookup PostAuthorLookup, log *zap.Logger) *Handler {
	return &Handler{svc: svc, postLookup: postLookup, log: log}
}

// RegisterRoutes mounts all reaction routes on the provided chi.Router.
//
// Routes:
//
//	POST   /posts/{postID}/react  — authenticated, 204 No Content (idempotent)
//	DELETE /posts/{postID}/react  — authenticated, 204 No Content (idempotent)
//
// Rate limiting is handled inside Service.React / Service.Unreact (fail-open
// per OPEN-P4-3): nil rdb or Redis errors allow the request through; only an
// explicit limit-exceeded result returns 429.
func (h *Handler) RegisterRoutes(r chi.Router, jwtSecret []byte) {
	r.With(auth.JWTMiddleware(jwtSecret)).Post("/posts/{postID}/react", h.react)
	r.With(auth.JWTMiddleware(jwtSecret)).Delete("/posts/{postID}/react", h.unreact)
}

// react handles POST /api/v1/posts/{postID}/react.
// Requires JWT authentication. Idempotent: double-react returns 204.
func (h *Handler) react(w http.ResponseWriter, r *http.Request) {
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

	// Resolve the post author for notification fan-out.
	postAuthorID, err := h.postLookup.GetPostAuthorID(r.Context(), postID)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	if err := h.svc.React(r.Context(), callerID, postID, postAuthorID); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// unreact handles DELETE /api/v1/posts/{postID}/react.
// Requires JWT authentication. Idempotent: unreacting a non-existent reaction returns 204.
func (h *Handler) unreact(w http.ResponseWriter, r *http.Request) {
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
		default:
			h.log.Error("reaction: unexpected internal error", zap.Error(err))
			apierror.Render(w, http.StatusInternalServerError,
				apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
		}
		return
	}

	h.log.Error("reaction: unexpected internal error", zap.Error(err))
	apierror.Render(w, http.StatusInternalServerError,
		apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
}

// parseReactionUUIDParam extracts and parses a UUID path parameter from the request.
// Writes a 400 response and returns false if the param is missing or invalid.
func parseReactionUUIDParam(w http.ResponseWriter, r *http.Request, param string) (uuid.UUID, bool) {
	raw := chi.URLParam(r, param)
	id, err := uuid.Parse(raw)
	if err != nil {
		apierror.Render(w, http.StatusBadRequest,
			apierror.New(apierror.CodeValidation, fmt.Sprintf("invalid %s format", param)))
		return uuid.UUID{}, false
	}
	return id, true
}
