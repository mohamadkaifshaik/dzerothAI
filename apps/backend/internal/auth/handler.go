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
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/ctxlog"
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

	refreshRL := RateLimitMiddleware(redisClient, RateLimitConfig{
		Operation:   "refresh",
		MaxAttempts: 20,
		Window:      15 * time.Minute,
	}, h.log, h.events)

	r.With(registerRL).Post("/auth/register", h.register)
	r.With(loginRL).Post("/auth/login", h.login)
	r.With(refreshRL).Post("/auth/refresh", h.refresh)
	r.With(JWTMiddleware(jwtSecret, h.log)).Post("/auth/logout", h.logout)
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
		h.handleServiceError(w, r, err)
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
		h.handleServiceError(w, r, err)
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
		h.handleServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, pair)
}

// logout handles POST /api/v1/auth/logout (requires JWT middleware).
// Extracts the session UUID from the "sid" claim (set in context by JWTMiddleware)
// and deletes only that session row, leaving all other active sessions for the same
// user intact. Returns HTTP 204 No Content on success.
//
// Backward compatibility: tokens issued before Phase 8A-3 do not carry a "sid" claim
// (SessionID == uuid.Nil). Such tokens are rejected with 401 rather than silently
// revoking all sessions (fail-safe behavior).
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	sessionID, ok := SessionIDFromContext(r.Context())
	if !ok {
		// Token predates the sid claim — reject rather than revoking all sessions.
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Token does not carry a session identifier. Please re-authenticate."))
		return
	}

	if err := h.svc.Logout(r.Context(), sessionID, userID); err != nil {
		h.log.Error("logout failed", ctxlog.RequestIDField(r.Context()), zap.Error(err))
		apierror.Render(w, http.StatusInternalServerError,
			apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
		return
	}

	h.events.RecordAuthEvent(platformMetrics.AuthEventLogout)
	w.WriteHeader(http.StatusNoContent)
}

// handleServiceError maps service-layer errors to appropriate HTTP responses.
// Internal errors are logged without sensitive details; only safe messages reach clients.
// r is used solely to extract the chi request ID for log correlation.
func (h *Handler) handleServiceError(w http.ResponseWriter, r *http.Request, err error) {
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
	h.log.Error("auth: unexpected internal error", ctxlog.RequestIDField(r.Context()), zap.Error(err))
	apierror.Render(w, http.StatusInternalServerError,
		apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
}

// writeJSON serializes v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, statusCode int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(v)
}
