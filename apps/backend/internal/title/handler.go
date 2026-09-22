package title

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/auth"
)

// Handler exposes the title HTTP endpoints.
type Handler struct {
	svc       *Service
	log       *zap.Logger
	jwtSecret []byte
}

// NewHandler constructs a title Handler.
func NewHandler(svc *Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// RegisterRoutes mounts all title routes on the provided chi.Router.
//
// Routes:
//
//	GET    /titles/catalog              — no auth required
//	GET    /titles/me                   — JWT required
//	GET    /titles/me/primary           — JWT required
//	PUT    /titles/me/primary           — JWT required
//	DELETE /titles/me/primary           — JWT required
//	GET    /titles/{userID}/primary     — optional auth: the handler parses the
//	                                      Bearer token directly when present so
//	                                      the owner always sees their own title;
//	                                      unauthenticated requests are not rejected.
func (h *Handler) RegisterRoutes(r chi.Router, jwtSecret []byte) {
	// Store the secret so getUserPrimary can perform optional token parsing.
	h.jwtSecret = jwtSecret

	// Public — no auth required.
	r.Get("/titles/catalog", h.getCatalog)

	// Owner-only — JWT required.
	r.With(auth.JWTMiddleware(jwtSecret, h.log)).Get("/titles/me", h.getMyTitles)
	r.With(auth.JWTMiddleware(jwtSecret, h.log)).Get("/titles/me/primary", h.getMyPrimary)
	r.With(auth.JWTMiddleware(jwtSecret, h.log)).Put("/titles/me/primary", h.setMyPrimary)
	r.With(auth.JWTMiddleware(jwtSecret, h.log)).Delete("/titles/me/primary", h.clearMyPrimary)

	// Optional auth — no middleware; the handler parses the Bearer token directly
	// when present without blocking absent auth. This allows the owner to be
	// identified and always see their own primary title even on private accounts.
	r.Get("/titles/{userID}/primary", h.getUserPrimary)
}

// getCatalog handles GET /api/v1/titles/catalog.
func (h *Handler) getCatalog(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.GetCatalog(r.Context())
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// getMyTitles handles GET /api/v1/titles/me.
func (h *Handler) getMyTitles(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	resp, err := h.svc.GetMyTitles(r.Context(), callerID)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// getMyPrimary handles GET /api/v1/titles/me/primary.
func (h *Handler) getMyPrimary(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	resp, err := h.svc.GetMyPrimaryTitle(r.Context(), callerID)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// setMyPrimary handles PUT /api/v1/titles/me/primary.
func (h *Handler) setMyPrimary(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	var req SetPrimaryTitleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Render(w, http.StatusBadRequest,
			apierror.New(apierror.CodeValidation, "invalid request body"))
		return
	}

	userTitleID, err := uuid.Parse(req.UserTitleID)
	if err != nil {
		apierror.Render(w, http.StatusBadRequest,
			apierror.New(apierror.CodeValidation, "user_title_id must be a valid UUID"))
		return
	}

	resp, svcErr := h.svc.SetMyPrimaryTitle(r.Context(), callerID, userTitleID)
	if svcErr != nil {
		h.handleServiceError(w, svcErr)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// clearMyPrimary handles DELETE /api/v1/titles/me/primary.
func (h *Handler) clearMyPrimary(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	if err := h.svc.ClearMyPrimaryTitle(r.Context(), callerID); err != nil {
		h.handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getUserPrimary handles GET /api/v1/titles/{userID}/primary.
// Auth is optional: when a valid Bearer token is present the caller UUID is
// extracted directly (no middleware ran on this route) so the owner always sees
// their own primary title. Unauthenticated requests receive a nil title for
// private accounts.
func (h *Handler) getUserPrimary(w http.ResponseWriter, r *http.Request) {
	targetID, ok := parseUUIDParam(w, r, "userID")
	if !ok {
		return
	}

	// Parse the Bearer token directly — no JWTMiddleware runs on this route.
	// extractCallerFromBearer returns nil when the token is absent or invalid,
	// which the service treats as an unauthenticated request.
	callerPtr := h.extractCallerFromBearer(r)

	resp, err := h.svc.GetUserPrimaryTitle(r.Context(), callerPtr, targetID)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// extractCallerFromBearer attempts to parse a Bearer JWT from the Authorization
// header without blocking the request when auth is absent or invalid.
// Returns nil when the Authorization header is missing, not a Bearer token,
// the token is invalid or expired, or the subject is not a valid UUID.
// This mirrors the pattern used by search.extractOptionalCallerID.
func (h *Handler) extractCallerFromBearer(r *http.Request) *uuid.UUID {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return nil
	}
	tokenStr := strings.TrimPrefix(authHeader, prefix)
	claims, err := auth.ValidateAccessToken(tokenStr, h.jwtSecret)
	if err != nil {
		return nil
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil
	}
	return &id
}

// handleServiceError maps *apierror.APIError and other service errors to HTTP responses.
func (h *Handler) handleServiceError(w http.ResponseWriter, err error) {
	var apiErr *apierror.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case apierror.CodeValidation:
			apierror.Render(w, http.StatusUnprocessableEntity, apiErr.ToResponse())
		case apierror.CodeUnauthorized:
			apierror.Render(w, http.StatusUnauthorized, apiErr.ToResponse())
		case apierror.CodeForbidden:
			apierror.Render(w, http.StatusForbidden, apiErr.ToResponse())
		case apierror.CodeNotFound:
			apierror.Render(w, http.StatusNotFound, apiErr.ToResponse())
		default:
			h.log.Error("title: unexpected internal error", zap.Error(err))
			apierror.Render(w, http.StatusInternalServerError,
				apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
		}
		return
	}

	h.log.Error("title: unexpected internal error", zap.Error(err))
	apierror.Render(w, http.StatusInternalServerError,
		apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
}

// parseUUIDParam extracts and parses a UUID path parameter from the request.
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

// writeJSON serializes v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, statusCode int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(v)
}
