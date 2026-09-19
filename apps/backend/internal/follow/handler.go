package follow

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
	platformMetrics "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/metrics"
)

// Handler exposes the follow HTTP endpoints.
type Handler struct {
	svc    *Service
	log    *zap.Logger
	events *platformMetrics.Events
}

// NewHandler constructs a follow Handler.
func NewHandler(svc *Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// SetEvents injects the Prometheus event counters into the Handler.
// Passing nil disables counter instrumentation (no-op, safe for unit tests).
func (h *Handler) SetEvents(e *platformMetrics.Events) {
	h.events = e
}

// RegisterRoutes mounts all follow routes on the provided chi.Router.
//
// Routes:
//
//	POST   /users/{userID}/follow    — authenticated, rate-limited (60/15min per user)
//	DELETE /users/{userID}/follow    — authenticated
//	GET    /users/{userID}/following — authenticated, paginated
//	GET    /users/{userID}/followers — authenticated, paginated
//
// The POST rate limit uses key rl:follow:{user_id} and fails closed (HTTP 503)
// when Redis is unavailable — this is an abuse-sensitive operation.
func (h *Handler) RegisterRoutes(r chi.Router, redisClient *rdb.Client, jwtSecret []byte) {
	followWriteRL := followRateLimitMiddleware(redisClient, h.log, h.events)

	// POST /users/{userID}/follow — authenticated + rate-limited.
	r.With(auth.JWTMiddleware(jwtSecret, h.log), followWriteRL).Post("/users/{userID}/follow", h.follow)

	// DELETE /users/{userID}/follow — authenticated.
	r.With(auth.JWTMiddleware(jwtSecret, h.log)).Delete("/users/{userID}/follow", h.unfollow)

	// GET /users/{userID}/following — authenticated, paginated.
	r.With(auth.JWTMiddleware(jwtSecret, h.log)).Get("/users/{userID}/following", h.listFollowing)

	// GET /users/{userID}/followers — authenticated, paginated.
	r.With(auth.JWTMiddleware(jwtSecret, h.log)).Get("/users/{userID}/followers", h.listFollowers)
}

// follow handles POST /api/v1/users/{userID}/follow.
// Requires JWT authentication. Rate-limited at 60 requests per 15 minutes per user.
func (h *Handler) follow(w http.ResponseWriter, r *http.Request) {
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

	if err := h.svc.Follow(r.Context(), callerID, targetID); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// unfollow handles DELETE /api/v1/users/{userID}/follow.
func (h *Handler) unfollow(w http.ResponseWriter, r *http.Request) {
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

	if err := h.svc.Unfollow(r.Context(), callerID, targetID); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// listFollowing handles GET /api/v1/users/{userID}/following.
func (h *Handler) listFollowing(w http.ResponseWriter, r *http.Request) {
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

	cursor := r.URL.Query().Get("cursor")

	page, err := h.svc.ListFollowing(r.Context(), callerID, targetID, cursor)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, page)
}

// listFollowers handles GET /api/v1/users/{userID}/followers.
func (h *Handler) listFollowers(w http.ResponseWriter, r *http.Request) {
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

	cursor := r.URL.Query().Get("cursor")

	page, err := h.svc.ListFollowers(r.Context(), callerID, targetID, cursor)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, page)
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
		case apierror.CodeRateLimit:
			apierror.Render(w, http.StatusTooManyRequests, apiErr.ToResponse())
		default:
			h.log.Error("follow: unexpected internal error", zap.Error(err))
			apierror.Render(w, http.StatusInternalServerError,
				apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
		}
		return
	}

	h.log.Error("follow: unexpected internal error", zap.Error(err))
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
// NOTE: writeJSON is intentionally kept local to each package (post, follow, user, auth).
// Factor into a shared helper in Phase 7 tech-debt cleanup.
func writeJSON(w http.ResponseWriter, statusCode int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(v)
}

// followRateLimitMiddleware enforces 60 requests per 15 minutes per authenticated user.
//
// Redis key: rl:follow:{user_id}
//
// This middleware must run AFTER JWTMiddleware (caller must be authenticated).
// Fails closed (HTTP 503) when Redis is unavailable — this is an abuse-sensitive operation.
// events is optional (nil-safe).
func followRateLimitMiddleware(redisClient *rdb.Client, log *zap.Logger, events *platformMetrics.Events) func(http.Handler) http.Handler {
	const (
		maxAttempts int64         = 60
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

			key := fmt.Sprintf("rl:follow:%s", userID.String())
			ctx := r.Context()

			// Fail closed when Redis is nil.
			if redisClient == nil {
				log.Error("follow rate limit: Redis client is nil — failing closed",
					zap.String("user_id", userID.String()),
				)
				apierror.Render(w, http.StatusServiceUnavailable,
					apierror.New(apierror.CodeServiceUnavailable, "Service temporarily unavailable."))
				return
			}

			// Initialize key with TTL only if it does not already exist, then increment.
			if err := redisClient.SetNX(ctx, key, 0, window).Err(); err != nil {
				log.Error("follow rate limit: Redis SET NX failed — failing closed",
					zap.String("user_id", userID.String()),
					zap.Error(err),
				)
				apierror.Render(w, http.StatusServiceUnavailable,
					apierror.New(apierror.CodeServiceUnavailable, "Service temporarily unavailable."))
				return
			}

			count, err := redisClient.Incr(ctx, key).Result()
			if err != nil {
				log.Error("follow rate limit: Redis INCR failed — failing closed",
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
					log.Error("follow rate limit: Redis EXPIRE failed after count==1",
						zap.String("key", key),
						zap.Error(expErr),
					)
				}
			}

			if count > maxAttempts {
				events.RecordRateLimit(platformMetrics.RateLimitCategoryFollow, platformMetrics.RateLimitResultRejected)
				w.Header().Set("Retry-After", "900")
				apierror.Render(w, http.StatusTooManyRequests,
					apierror.New(apierror.CodeRateLimit, "Too many follow requests. Please try again later."))
				return
			}

			events.RecordRateLimit(platformMetrics.RateLimitCategoryFollow, platformMetrics.RateLimitResultAllowed)
			next.ServeHTTP(w, r)
		})
	}
}
