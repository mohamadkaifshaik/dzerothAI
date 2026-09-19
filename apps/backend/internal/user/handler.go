package user

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/auth"
)

// BlockChecker allows user.Handler to check bidirectional block state without
// importing the block package directly, avoiding a circular dependency.
// block.Service satisfies this interface via its IsBlockedBidirectional method.
type BlockChecker interface {
	IsBlockedBidirectional(ctx context.Context, userA, userB uuid.UUID) (bool, error)
}

// Handler exposes the user profile HTTP endpoints.
type Handler struct {
	svc          *Service
	log          *zap.Logger
	blockChecker BlockChecker
}

// NewHandler constructs a user Handler.
func NewHandler(svc *Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// SetBlockChecker injects a BlockChecker dependency. Must be called after
// NewHandler and before the first request is served.
func (h *Handler) SetBlockChecker(bc BlockChecker) {
	h.blockChecker = bc
}

// RegisterRoutes mounts all user/profile routes on the provided chi.Router.
// All routes require JWT authentication; the JWTMiddleware must be applied
// by the caller or via a sub-router.
func (h *Handler) RegisterRoutes(r chi.Router, jwtSecret []byte) {
	r.With(auth.JWTMiddleware(jwtSecret)).Group(func(r chi.Router) {
		r.Get("/me", h.getMe)
		r.Put("/me", h.updateMe)
		r.Get("/me/settings", h.getSettings)
		r.Put("/me/settings", h.updateSettings)
		r.Delete("/me/account", h.deleteAccount)
		r.Get("/users/{id}", h.getUserByID)
	})
}

// getMe handles GET /api/v1/me.
// Returns the authenticated user's OwnProfile.
func (h *Handler) getMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	u, err := h.svc.GetProfile(r.Context(), userID)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": ToOwnProfile(u)})
}

// updateMe handles PUT /api/v1/me.
// Parses UpdateProfileInput from the request body and applies it to the caller's profile.
func (h *Handler) updateMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	var input UpdateProfileInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		apierror.Render(w, http.StatusBadRequest,
			apierror.New(apierror.CodeValidation, "Invalid JSON body."))
		return
	}

	u, err := h.svc.UpdateProfile(r.Context(), userID, input)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": ToOwnProfile(u)})
}

// getSettings handles GET /api/v1/me/settings.
// Returns the caller's current privacy settings.
func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	u, err := h.svc.GetProfile(r.Context(), userID)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"is_private": u.IsPrivate,
		},
	})
}

// updateSettings handles PUT /api/v1/me/settings.
// Body: { "is_private": bool }
func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	var body struct {
		IsPrivate bool `json:"is_private"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Render(w, http.StatusBadRequest,
			apierror.New(apierror.CodeValidation, "Invalid JSON body."))
		return
	}

	u, err := h.svc.UpdateSettings(r.Context(), userID, body.IsPrivate)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"is_private": u.IsPrivate,
		},
	})
}

// deleteAccount handles DELETE /api/v1/me/account.
// Suspends the authenticated caller's own account and revokes all their sessions.
// Returns HTTP 204 No Content on success. Idempotent: an already-suspended
// account still returns 204.
func (h *Handler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	if err := h.svc.SuspendSelf(r.Context(), userID); err != nil {
		h.log.Error("user: delete account failed", zap.Error(err))
		apierror.Render(w, http.StatusInternalServerError,
			apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// getUserByID handles GET /api/v1/users/{id}.
// Returns the PublicProfile for any user by UUID.
// The response never includes social-validation metrics (CLAUDE.md §2.3).
//
// Privacy enforcement (CLAUDE.md §12): when a bidirectional block exists
// between the authenticated caller and the target user, this handler returns
// HTTP 404 "user not found" — never HTTP 403. This ensures neither party can
// infer that a block relationship exists by observing the response status code.
func (h *Handler) getUserByID(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	idStr := chi.URLParam(r, "id")
	targetID, err := uuid.Parse(idStr)
	if err != nil {
		apierror.Render(w, http.StatusBadRequest,
			apierror.New(apierror.CodeValidation, "Invalid user ID format."))
		return
	}

	// Block-based 404: if either party has blocked the other, return 404 to
	// avoid revealing the block state (CLAUDE.md §12). Skip the check when
	// the caller is looking up their own profile, or when no block checker is
	// configured (safe degradation — profile remains visible).
	if h.blockChecker != nil && callerID != targetID {
		blocked, err := h.blockChecker.IsBlockedBidirectional(r.Context(), callerID, targetID)
		if err != nil {
			h.log.Error("user: block check for profile lookup", zap.Error(err))
			apierror.Render(w, http.StatusInternalServerError,
				apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
			return
		}
		if blocked {
			// Return 404 — do not reveal that a block relationship exists.
			apierror.Render(w, http.StatusNotFound,
				apierror.New(apierror.CodeNotFound, "User not found."))
			return
		}
	}

	u, err := h.svc.GetProfile(r.Context(), targetID)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": ToPublicProfile(u)})
}

// handleServiceError maps service-layer errors to HTTP responses.
func (h *Handler) handleServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		apierror.Render(w, http.StatusNotFound,
			apierror.New(apierror.CodeNotFound, "User not found."))
		return
	}

	var ve *ProfileValidationError
	if errors.As(err, &ve) {
		details := make([]apierror.Detail, len(ve.Details))
		for i, d := range ve.Details {
			details[i] = apierror.Detail{Field: d.Field, Message: d.Message}
		}
		apierror.Render(w, http.StatusBadRequest, apierror.ValidationError(details))
		return
	}

	h.log.Error("user: unexpected internal error", zap.Error(err))
	apierror.Render(w, http.StatusInternalServerError,
		apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
}

// writeJSON serializes v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, statusCode int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(v)
}
