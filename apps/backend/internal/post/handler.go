package post

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	rdb "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/auth"
)

// Handler exposes the post HTTP endpoints.
type Handler struct {
	svc             *Service
	log             *zap.Logger
	reactionChecker ReactionChecker
}

// NewHandler constructs a post Handler.
func NewHandler(svc *Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// SetReactionChecker injects a ReactionChecker dependency. Must be called after
// NewHandler and before the first request is served. Concurrency-safe if called
// during the single-threaded startup phase before the HTTP server starts.
func (h *Handler) SetReactionChecker(rc ReactionChecker) {
	h.reactionChecker = rc
}

// RegisterRoutes mounts all post routes on the provided chi.Router.
//
// Routes:
//
//	POST   /posts                      — authenticated, rate-limited (30/15min per user)
//	GET    /posts/{postID}             — public
//	DELETE /posts/{postID}             — authenticated, owner only
//	GET    /posts/{postID}/thread      — public, paginated
//	GET    /users/{userID}/posts       — public, paginated
//	GET    /hashtags/{tag}/posts       — public, auth optional, paginated
//
// The write rate limit uses key rl:post:create:{user_id} and fails closed
// (HTTP 503) when Redis is unavailable — this is an abuse-sensitive operation.
func (h *Handler) RegisterRoutes(r chi.Router, redisClient *rdb.Client, jwtSecret []byte) {
	postWriteRL := postRateLimitMiddleware(redisClient, h.log)

	// POST /posts — authenticated + rate-limited.
	r.With(auth.JWTMiddleware(jwtSecret), postWriteRL).Post("/posts", h.createPost)

	// GET /posts/{postID} — public.
	r.Get("/posts/{postID}", h.getPost)

	// DELETE /posts/{postID} — authenticated.
	r.With(auth.JWTMiddleware(jwtSecret)).Delete("/posts/{postID}", h.deletePost)

	// GET /posts/{postID}/thread — public, paginated.
	r.Get("/posts/{postID}/thread", h.getThread)

	// GET /users/{userID}/posts — public, paginated.
	r.Get("/users/{userID}/posts", h.listUserPosts)

	// GET /hashtags/{tag}/posts — public, auth optional, cursor-paginated.
	// Authenticated callers have block-filtered results.
	r.Get("/hashtags/{tag}/posts", h.listHashtagPosts(jwtSecret))
}

// createPost handles POST /api/v1/posts.
// Requires JWT authentication. Rate-limited at 30 requests per 15 minutes per user.
func (h *Handler) createPost(w http.ResponseWriter, r *http.Request) {
	callerID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		apierror.Render(w, http.StatusUnauthorized,
			apierror.New(apierror.CodeUnauthorized, "Not authenticated."))
		return
	}

	var req CreatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Render(w, http.StatusBadRequest,
			apierror.New(apierror.CodeValidation, "Invalid JSON body."))
		return
	}

	// Reject oversized content early before hitting the service.
	if req.PostType != PostTypeRepost {
		count := runeCount(req.Content)
		if count > maxPostRunes {
			apierror.Render(w, http.StatusBadRequest,
				apierror.New(apierror.CodeValidation, fmt.Sprintf("content exceeds %d code points", maxPostRunes)))
			return
		}
	}

	dto, err := h.svc.CreatePost(r.Context(), callerID, req)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"post": dto})
}

// getPost handles GET /api/v1/posts/{postID}.
// The route is public. When the caller is authenticated, viewer_has_reacted
// is included in the response. Unauthenticated callers receive a response with
// the viewer_has_reacted field fully absent (not false, not null) per CLAUDE.md §2.3.
func (h *Handler) getPost(w http.ResponseWriter, r *http.Request) {
	postID, ok := parseUUIDParam(w, r, "postID")
	if !ok {
		return
	}

	dto, err := h.svc.GetPost(r.Context(), postID)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	resp := PostDetailResponse{PostDTO: dto}

	// Enrich with viewer_has_reacted only for authenticated callers.
	// The route does not require auth, so we check context without mandating it.
	if callerID, authed := auth.UserIDFromContext(r.Context()); authed && h.reactionChecker != nil {
		reacted, reactionErr := h.reactionChecker.HasReacted(r.Context(), callerID, postID)
		if reactionErr != nil {
			// Non-fatal: log and omit the field rather than failing the request.
			h.log.Warn("post: get post reaction check failed",
				zap.String("post_id", postID.String()),
				zap.String("caller_id", callerID.String()),
				zap.Error(reactionErr),
			)
		} else {
			resp.ViewerHasReacted = &reacted
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"post": resp})
}

// deletePost handles DELETE /api/v1/posts/{postID}.
// The caller must be the post author; ownership is checked in the service layer.
func (h *Handler) deletePost(w http.ResponseWriter, r *http.Request) {
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

	if err := h.svc.DeletePost(r.Context(), callerID, postID); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// getThread handles GET /api/v1/posts/{postID}/thread.
// Public, paginated. Uses {postID} as the thread_root_id.
func (h *Handler) getThread(w http.ResponseWriter, r *http.Request) {
	postID, ok := parseUUIDParam(w, r, "postID")
	if !ok {
		return
	}

	cursor := r.URL.Query().Get("cursor")

	page, err := h.svc.ListThreadReplies(r.Context(), postID, cursor)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, page)
}

// listUserPosts handles GET /api/v1/users/{userID}/posts.
// Public, paginated. Respects the author's private account setting.
func (h *Handler) listUserPosts(w http.ResponseWriter, r *http.Request) {
	userID, ok := parseUUIDParam(w, r, "userID")
	if !ok {
		return
	}

	cursor := r.URL.Query().Get("cursor")

	// Caller may or may not be authenticated. We extract the caller ID if present
	// but do not require authentication for public profiles.
	var callerID *uuid.UUID
	if id, authed := auth.UserIDFromContext(r.Context()); authed {
		callerID = &id
	}

	page, err := h.svc.ListPostsByAuthor(r.Context(), callerID, userID, cursor)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, page)
}

// listHashtagPosts handles GET /api/v1/hashtags/{tag}/posts.
// Auth optional. Authenticated callers have block-filtered results.
// Terminated at 200 items (CLAUDE.md §2.1, no infinite scrolling).
func (h *Handler) listHashtagPosts(jwtSecret []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tag := chi.URLParam(r, "tag")
		cursor := r.URL.Query().Get("cursor")
		callerID := extractOptionalCallerID(r, jwtSecret)

		page, err := h.svc.PostsByHashtag(r.Context(), callerID, tag, cursor)
		if err != nil {
			h.handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, page)
	}
}

// extractOptionalCallerID attempts to parse a Bearer JWT from the Authorization
// header without blocking the request if authentication is absent or invalid.
// Returns nil when no valid JWT is present.
//
// NOTE: This function is intentionally local to each package that needs optional-auth
// (post, search). Factor into a shared helper in Phase 7 tech-debt cleanup.
func extractOptionalCallerID(r *http.Request, jwtSecret []byte) *uuid.UUID {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return nil
	}
	tokenStr := strings.TrimPrefix(authHeader, prefix)
	claims, err := auth.ValidateAccessToken(tokenStr, jwtSecret)
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
			h.log.Error("post: unexpected internal error", zap.Error(err))
			apierror.Render(w, http.StatusInternalServerError,
				apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
		}
		return
	}

	h.log.Error("post: unexpected internal error", zap.Error(err))
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
// NOTE: writeJSON is intentionally kept local to each package (post, user, auth).
// Factor into a shared helper in Phase 7 tech-debt cleanup.
func writeJSON(w http.ResponseWriter, statusCode int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(v)
}

// runeCount returns the number of Unicode code points in s.
func runeCount(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// postRateLimitMiddleware enforces 30 requests per 15 minutes per authenticated user.
//
// Redis key: rl:post:create:{user_id}
//
// This middleware must run AFTER JWTMiddleware (caller must be authenticated).
// Fails closed (HTTP 503) when Redis is unavailable — this is an abuse-sensitive operation.
func postRateLimitMiddleware(redisClient *rdb.Client, log *zap.Logger) func(http.Handler) http.Handler {
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

			key := fmt.Sprintf("rl:post:create:%s", userID.String())
			ctx := r.Context()

			// Fail closed when Redis is nil.
			if redisClient == nil {
				log.Error("post rate limit: Redis client is nil — failing closed",
					zap.String("user_id", userID.String()),
				)
				apierror.Render(w, http.StatusServiceUnavailable,
					apierror.New(apierror.CodeServiceUnavailable, "Service temporarily unavailable."))
				return
			}

			// Initialize key with TTL only if it does not already exist, then increment.
			if err := redisClient.SetNX(ctx, key, 0, window).Err(); err != nil {
				log.Error("post rate limit: Redis SET NX failed — failing closed",
					zap.String("user_id", userID.String()),
					zap.Error(err),
				)
				apierror.Render(w, http.StatusServiceUnavailable,
					apierror.New(apierror.CodeServiceUnavailable, "Service temporarily unavailable."))
				return
			}

			count, err := redisClient.Incr(ctx, key).Result()
			if err != nil {
				log.Error("post rate limit: Redis INCR failed — failing closed",
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
					log.Error("post rate limit: Redis EXPIRE failed after count==1",
						zap.String("key", key),
						zap.Error(expErr),
					)
				}
			}

			if count > maxAttempts {
				w.Header().Set("Retry-After", "900")
				apierror.Render(w, http.StatusTooManyRequests,
					apierror.New(apierror.CodeRateLimit, "Too many posts. Please try again later."))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
