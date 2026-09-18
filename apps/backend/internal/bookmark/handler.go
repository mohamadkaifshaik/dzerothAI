package bookmark

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

// Handler exposes the bookmark HTTP endpoints.
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler constructs a bookmark Handler.
func NewHandler(svc *Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// RegisterRoutes mounts all bookmark routes on the provided chi.Router.
//
// Routes:
//
//	POST   /posts/{postID}/bookmark  — authenticated, rate-limited (120/15min per user)
//	DELETE /posts/{postID}/bookmark  — authenticated
//	GET    /me/bookmarks             — authenticated (owner-only, always uses JWT callerID)
//
// The POST rate limit uses key rl:bookmark:{user_id} and fails closed (HTTP 503)
// when Redis is unavailable — this is an abuse-sensitive operation.
func (h *Handler) RegisterRoutes(r chi.Router, redisClient *rdb.Client, jwtSecret []byte) {
	bookmarkRL := bookmarkRateLimitMiddleware(redisClient, h.log)

	// POST /posts/{postID}/bookmark — authenticated + rate-limited.
	r.With(auth.JWTMiddleware(jwtSecret), bookmarkRL).Post("/posts/{postID}/bookmark", h.addBookmark)

	// DELETE /posts/{postID}/bookmark — authenticated.
	r.With(auth.JWTMiddleware(jwtSecret)).Delete("/posts/{postID}/bookmark", h.removeBookmark)

	// GET /me/bookmarks — authenticated, owner-only (uses JWT callerID, no path param).
	r.With(auth.JWTMiddleware(jwtSecret)).Get("/me/bookmarks", h.listBookmarks)
}

// addBookmark handles POST /api/v1/posts/{postID}/bookmark.
// Requires JWT authentication. Rate-limited at 120 requests per 15 minutes per user.
func (h *Handler) addBookmark(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	postID, ok := parseUUIDParam(w, r, "postID")
	if !ok {
		return
	}

	if err := h.svc.Bookmark(r.Context(), callerID, postID); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// removeBookmark handles DELETE /api/v1/posts/{postID}/bookmark.
func (h *Handler) removeBookmark(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	postID, ok := parseUUIDParam(w, r, "postID")
	if !ok {
		return
	}

	if err := h.svc.Unbookmark(r.Context(), callerID, postID); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// listBookmarks handles GET /api/v1/me/bookmarks.
// Owner-only: always uses JWT callerID, never a path-param userID.
func (h *Handler) listBookmarks(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	cursor := r.URL.Query().Get("cursor")

	page, err := h.svc.ListBookmarks(r.Context(), callerID, cursor)
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
			apierror.Render(w, http.StatusBadRequest, apiErr.ToResponse())
		case apierror.CodeUnauthorized:
			apierror.Render(w, http.StatusUnauthorized, apiErr.ToResponse())
		case apierror.CodeForbidden:
			apierror.Render(w, http.StatusForbidden, apiErr.ToResponse())
		case apierror.CodeNotFound:
			apierror.Render(w, http.StatusNotFound, apiErr.ToResponse())
		case apierror.CodeRateLimit:
			apierror.Render(w, http.StatusTooManyRequests, apiErr.ToResponse())
		default:
			h.log.Error("bookmark: unexpected internal error", zap.Error(err))
			apierror.Render(w, http.StatusInternalServerError,
				apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
		}
		return
	}

	h.log.Error("bookmark: unexpected internal error", zap.Error(err))
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

// bookmarkRateLimitMiddleware enforces 120 requests per 15 minutes per authenticated user.
//
// Redis key: rl:bookmark:{user_id}
//
// This middleware must run AFTER JWTMiddleware (caller must be authenticated).
// Fails closed (HTTP 503) when Redis is unavailable — this is an abuse-sensitive operation.
func bookmarkRateLimitMiddleware(redisClient *rdb.Client, log *zap.Logger) func(http.Handler) http.Handler {
	const (
		maxAttempts int64         = 120
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

			key := fmt.Sprintf("rl:bookmark:%s", userID.String())
			ctx := r.Context()

			// Fail closed when Redis is nil.
			if redisClient == nil {
				log.Error("bookmark rate limit: Redis client is nil — failing closed",
					zap.String("user_id", userID.String()),
				)
				apierror.Render(w, http.StatusServiceUnavailable,
					apierror.New(apierror.CodeServiceUnavailable, "Service temporarily unavailable."))
				return
			}

			// Initialize key with TTL only if it does not already exist, then increment.
			if err := redisClient.SetNX(ctx, key, 0, window).Err(); err != nil {
				log.Error("bookmark rate limit: Redis SET NX failed — failing closed",
					zap.String("user_id", userID.String()),
					zap.Error(err),
				)
				apierror.Render(w, http.StatusServiceUnavailable,
					apierror.New(apierror.CodeServiceUnavailable, "Service temporarily unavailable."))
				return
			}

			count, err := redisClient.Incr(ctx, key).Result()
			if err != nil {
				log.Error("bookmark rate limit: Redis INCR failed — failing closed",
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
					log.Error("bookmark rate limit: Redis EXPIRE failed after count==1",
						zap.String("key", key),
						zap.Error(expErr),
					)
				}
			}

			if count > maxAttempts {
				w.Header().Set("Retry-After", "900")
				apierror.Render(w, http.StatusTooManyRequests,
					apierror.New(apierror.CodeRateLimit, "Too many bookmark requests. Please try again later."))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
