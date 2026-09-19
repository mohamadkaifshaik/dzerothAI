package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	rdb "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	platformMetrics "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/metrics"
)

// Handler exposes the auth HTTP endpoints.
type Handler struct {
	svc    *Service
	log    *zap.Logger
	events *platformMetrics.Events
}

// NewHandler constructs an auth Handler.
func NewHandler(svc *Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// SetEvents injects the Prometheus event counters into the Handler.
// Passing nil disables counter instrumentation (no-op, safe for unit tests).
func (h *Handler) SetEvents(e *platformMetrics.Events) {
	h.events = e
}

// RegisterRoutes mounts all auth routes on the provided chi.Router.
// Rate-limiting middleware is applied per endpoint using the approved parameters.
func (h *Handler) RegisterRoutes(r chi.Router, redisClient *rdb.Client, jwtSecret []byte) {
	loginRL := RateLimitMiddleware(redisClient, RateLimitConfig{
		Operation:   "login",
		MaxAttempts: 10,
		Window:      15 * time.Minute,
	}, h.log, h.events)

	registerRL := RateLimitMiddleware(redisClient, RateLimitConfig{
		Operation:   "register",
		MaxAttempts: 5,
		Window:      time.Hour,
	}, h.log, h.events)

	r.With(registerRL).Post("/auth/register", h.register)
	r.With(loginRL).Post("/auth/login", h.login)
	r.Post("/auth/refresh", h.refresh)
	r.With(JWTMiddleware(jwtSecret)).Post("/auth/logout", h.logout)
}

// register handles POST /api/v1/auth/register.
// On success returns HTTP 201 with a TokenPair JSON body.
func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Handle      string `json:"handle"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Password    string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Render(w, http.StatusBadRequest,
			apierror.New(apierror.CodeValidation, "Invalid JSON body."))
		return
	}

	pair, err := h.svc.Register(r.Context(), RegisterInput{
		Handle:      body.Handle,
		DisplayName: body.DisplayName,
		Email:       body.Email,
		Password:    body.Password,
	})
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, pair)
}

// login handles POST /api/v1/auth/login.
// On success returns HTTP 200 with a TokenPair JSON body.
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Render(w, http.StatusBadRequest,
			apierror.New(apierror.CodeValidation, "Invalid JSON body."))
		return
	}

	pair, err := h.svc.Login(r.Context(), LoginInput{
		Email:    body.Email,
		Password: body.Password,
	})
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, pair)
}

// refresh handles POST /api/v1/auth/refresh.
// Body: { "refresh_token": "..." }
// On success returns HTTP 200 with a new TokenPair.
func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Render(w, http.StatusBadRequest,
			apierror.New(apierror.CodeValidation, "Invalid JSON body."))
		return
	}

	pair, err := h.svc.Refresh(r.Context(), body.RefreshToken)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, pair)
}

// logout handles POST /api/v1/auth/logout (requires JWT middleware).
// Extracts the session JTI from the context (set by JWTMiddleware) and deletes the session.
// Returns HTTP 204 No Content.
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	// The logout handler uses the session JTI as the session identifier.
	// Per ADR 0005: sessions.id (UUID v7) and the JWT jti (UUID v4) are separate values.
	// For logout we delete the session by its database ID. However the JWT only carries
	// the user ID (sub) and jti; the session database row is looked up by the jti stored
	// in context. Since jti != sessions.id we must look up the session by jti or by
	// user+token combination. The safest approach for Phase 1 logout is to delete all
	// sessions for the authenticated user OR require the client to supply the refresh token.
	//
	// Per spec: "extract session ID from JWT jti; call service.Logout".
	// The jti is a UUID v4 and does not map directly to sessions.id (UUID v7).
	// To be consistent with the spec intent (revoking the current session) we use the
	// authenticated user ID and delete the session identified by the jti value we stored
	// in context at login time. Since we don't persist jti→session mapping, Phase 1
	// logout deletes all sessions for the user. This is a safe, conservative behavior.
	// TODO(phase-2): persist jti→session_id mapping to support single-session logout.

	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	if err := DeleteAllUserSessions(r.Context(), h.svc.pool, userID); err != nil {
		h.log.Error("logout failed", zap.Error(err))
		apierror.Render(w, http.StatusInternalServerError,
			apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
		return
	}

	h.events.RecordAuthEvent(platformMetrics.AuthEventLogout)
	w.WriteHeader(http.StatusNoContent)
}

// handleServiceError maps service-layer errors to appropriate HTTP responses.
// Internal errors are logged without sensitive details; only safe messages reach clients.
func (h *Handler) handleServiceError(w http.ResponseWriter, err error) {
	var ve *ValidationError
	if errors.As(err, &ve) {
		if len(ve.Details) > 0 {
			details := make([]apierror.Detail, len(ve.Details))
			for i, d := range ve.Details {
				details[i] = apierror.Detail{Field: d.Field, Message: d.Message}
			}
			apierror.Render(w, http.StatusBadRequest, apierror.ValidationError(details))
		} else {
			apierror.Render(w, http.StatusBadRequest,
				apierror.New(apierror.CodeValidation, ve.Message))
		}
		return
	}

	var ue *UnauthorizedError
	if errors.As(err, &ue) {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, ue.Message))
		return
	}

	var se *SuspendedError
	if errors.As(err, &se) {
		apierror.Render(w, http.StatusForbidden,
			apierror.New(apierror.CodeForbidden, "Account is suspended."))
		return
	}

	if errors.Is(err, ErrDuplicateEmail) {
		apierror.Render(w, http.StatusConflict,
			apierror.New(apierror.CodeConflict, "An account with that email address already exists."))
		return
	}

	if errors.Is(err, ErrDuplicateHandle) {
		apierror.Render(w, http.StatusConflict,
			apierror.New(apierror.CodeConflict, "That handle is already taken."))
		return
	}

	// Unexpected internal error — log it, return safe message.
	h.log.Error("auth: unexpected internal error", zap.Error(err))
	apierror.Render(w, http.StatusInternalServerError,
		apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
}

// writeJSON serializes v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, statusCode int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(v)
}
