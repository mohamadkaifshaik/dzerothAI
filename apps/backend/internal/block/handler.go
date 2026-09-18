package block

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	rdb "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/auth"
)

// Handler exposes the block and mute HTTP endpoints.
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler constructs a block Handler.
func NewHandler(svc *Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// RegisterRoutes mounts all block/mute routes on the provided chi.Router.
//
// Routes:
//
//	POST   /users/{userID}/block   — authenticated, rate-limited (30/15min per user)
//	DELETE /users/{userID}/block   — authenticated
//	POST   /users/{userID}/mute    — authenticated, rate-limited (30/15min per user)
//	DELETE /users/{userID}/mute    — authenticated
//
// The POST rate limit uses key rl:block:{user_id} and fails closed (HTTP 503)
// when Redis is unavailable — this is an abuse-sensitive operation.
func (h *Handler) RegisterRoutes(r chi.Router, redisClient *rdb.Client, jwtSecret []byte) {
	blockWriteRL := blockRateLimitMiddleware(redisClient, h.log)

	// POST /users/{userID}/block — authenticated + rate-limited.
	r.With(auth.JWTMiddleware(jwtSecret), blockWriteRL).Post("/users/{userID}/block", h.block)

	// DELETE /users/{userID}/block — authenticated.
	r.With(auth.JWTMiddleware(jwtSecret)).Delete("/users/{userID}/block", h.unblock)

	// POST /users/{userID}/mute — authenticated + rate-limited.
	r.With(auth.JWTMiddleware(jwtSecret), blockWriteRL).Post("/users/{userID}/mute", h.mute)

	// DELETE /users/{userID}/mute — authenticated.
	r.With(auth.JWTMiddleware(jwtSecret)).Delete("/users/{userID}/mute", h.unmute)
}

// block handles POST /api/v1/users/{userID}/block.
// Requires JWT authentication. Rate-limited at 30 requests per 15 minutes per user.
func (h *Handler) block(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	targetID, ok := parseUUIDParam(w, r, "userID")
	if !ok {
		return
	}

	if err := h.svc.Block(r.Context(), callerID, targetID); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// unblock handles DELETE /api/v1/users/{userID}/block.
func (h *Handler) unblock(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	targetID, ok := parseUUIDParam(w, r, "userID")
	if !ok {
		return
	}

	if err := h.svc.Unblock(r.Context(), callerID, targetID); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// mute handles POST /api/v1/users/{userID}/mute.
// Requires JWT authentication. Rate-limited at 30 requests per 15 minutes per user.
func (h *Handler) mute(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	targetID, ok := parseUUIDParam(w, r, "userID")
	if !ok {
		return
	}

	if err := h.svc.Mute(r.Context(), callerID, targetID); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// unmute handles DELETE /api/v1/users/{userID}/mute.
func (h *Handler) unmute(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	targetID, ok := parseUUIDParam(w, r, "userID")
	if !ok {
		return
	}

	if err := h.svc.Unmute(r.Context(), callerID, targetID); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleServiceError maps *apierror.APIError and other service errors to HTTP responses.
// CodeValidation → 422 (Unprocessable Entity) to match the follow handler convention.
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
		case apierror.CodeRateLimit:
			apierror.Render(w, http.StatusTooManyRequests, apiErr.ToResponse())
		default:
			h.log.Error("block: unexpected internal error", zap.Error(err))
			apierror.Render(w, http.StatusInternalServerError,
				apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
		}
		return
	}

	h.log.Error("block: unexpected internal error", zap.Error(err))
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
// NOTE: writeJSON is intentionally kept local to each package.
// Factor into a shared helper in Phase 7 tech-debt cleanup.
func writeJSON(w http.ResponseWriter, statusCode int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(v)
}

// blockRateLimitMiddleware enforces 30 requests per 15 minutes per authenticated user.
//
// Redis key: rl:block:{user_id}
//
// This middleware must run AFTER JWTMiddleware (caller must be authenticated).
// Fails closed (HTTP 503) when Redis is unavailable — this is an abuse-sensitive operation.
func blockRateLimitMiddleware(redisClient *rdb.Client, log *zap.Logger) func(http.Handler) http.Handler {
	const (
		maxAttempts int64         = 30
		window      time.Duration = 15 * time.Minute
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := auth.UserIDFromContext(r.Context())
			if !ok {
				// No user in context — JWTMiddleware should have blocked this.
				apierror.Render(w, http.StatusUnauthorized,
					apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
				return
			}

			key := fmt.Sprintf("rl:block:%s", userID.String())
			ctx := r.Context()

			// Fail closed when Redis is nil.
			if redisClient == nil {
				log.Error("block rate limit: Redis client is nil — failing closed",
					zap.String("user_id", userID.String()),
				)
				apierror.Render(w, http.StatusServiceUnavailable,
					apierror.New(apierror.CodeServiceUnavailable, "Service temporarily unavailable."))
				return
			}

			// Initialize key with TTL only if it does not already exist, then increment.
			if err := redisClient.SetNX(ctx, key, 0, window).Err(); err != nil {
				log.Error("block rate limit: Redis SET NX failed — failing closed",
					zap.String("user_id", userID.String()),
					zap.Error(err),
				)
				apierror.Render(w, http.StatusServiceUnavailable,
					apierror.New(apierror.CodeServiceUnavailable, "Service temporarily unavailable."))
				return
			}

			count, err := redisClient.Incr(ctx, key).Result()
			if err != nil {
				log.Error("block rate limit: Redis INCR failed — failing closed",
					zap.String("user_id", userID.String()),
					zap.Error(err),
				)
				apierror.Render(w, http.StatusServiceUnavailable,
					apierror.New(apierror.CodeServiceUnavailable, "Service temporarily unavailable."))
				return
			}

			// Belt-and-suspenders: ensure TTL is always set.
			if count == 1 {
				if expErr := redisClient.Expire(ctx, key, window).Err(); expErr != nil {
					log.Error("block rate limit: Redis EXPIRE failed after count==1",
						zap.String("key", key),
						zap.Error(expErr),
					)
				}
			}

			if count > maxAttempts {
				w.Header().Set("Retry-After", "900")
				apierror.Render(w, http.StatusTooManyRequests,
					apierror.New(apierror.CodeRateLimit, "Too many block/mute requests. Please try again later."))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
