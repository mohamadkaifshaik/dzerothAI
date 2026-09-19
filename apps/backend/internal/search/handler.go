package search

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	rdb "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/auth"
)

// Handler exposes the search HTTP endpoints.
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler constructs a search Handler.
func NewHandler(svc *Service, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// RegisterRoutes mounts all search routes on the provided chi.Router.
//
// Routes:
//
//	GET /search/posts?q=<query>&cursor=<cursor>  — auth optional, rate-limited (fail-open)
//	GET /search/users?q=<query>&cursor=<cursor>  — auth optional, rate-limited (fail-open)
//
// Rate limit: rl:search:{ip_or_user}, 30 requests per 15 minutes.
// Fail-open: if Redis is unavailable (nil client), the request is allowed.
// This is approved behavior for non-security-sensitive operations.
//
// Optional auth: the JWTMiddleware is NOT applied. Callers may supply a valid
// Bearer token and block filtering will be applied if it parses correctly.
// Unauthenticated callers receive results without block filtering.
func (h *Handler) RegisterRoutes(r chi.Router, redisClient *rdb.Client, jwtSecret []byte) {
	searchRL := searchRateLimitMiddleware(redisClient, h.log)

	r.With(searchRL).Get("/search/posts", h.searchPosts(jwtSecret))
	r.With(searchRL).Get("/search/users", h.searchUsers(jwtSecret))
}

// searchPosts handles GET /api/v1/search/posts.
// Auth optional. Extracts caller ID from the JWT if present; unauthenticated callers
// receive results without block filtering.
func (h *Handler) searchPosts(jwtSecret []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			apierror.Render(w, http.StatusBadRequest,
				apierror.New(apierror.CodeValidation, "search query must not be empty"))
			return
		}

		cursor := r.URL.Query().Get("cursor")
		callerID := extractOptionalCallerID(r, jwtSecret)

		page, err := h.svc.SearchPosts(r.Context(), callerID, query, cursor)
		if err != nil {
			h.handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, page)
	}
}

// searchUsers handles GET /api/v1/search/users.
// Auth optional. Extracts caller ID from the JWT if present.
func (h *Handler) searchUsers(jwtSecret []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			apierror.Render(w, http.StatusBadRequest,
				apierror.New(apierror.CodeValidation, "search query must not be empty"))
			return
		}

		cursor := r.URL.Query().Get("cursor")
		callerID := extractOptionalCallerID(r, jwtSecret)

		page, err := h.svc.SearchUsers(r.Context(), callerID, query, cursor)
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
			h.log.Error("search: unexpected internal error", zap.Error(err))
			apierror.Render(w, http.StatusInternalServerError,
				apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
		}
		return
	}

	h.log.Error("search: unexpected internal error", zap.Error(err))
	apierror.Render(w, http.StatusInternalServerError,
		apierror.New(apierror.CodeInternal, "An unexpected error occurred."))
}

// writeJSON serializes v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, statusCode int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(v)
}

// searchRateLimitMiddleware enforces 30 requests per 15 minutes per IP (unauthenticated)
// or per user ID (authenticated, using the X-Real-IP / RemoteAddr fallback).
//
// Redis key: rl:search:{ip_or_user}
//
// Fail-open: if Redis is unavailable (nil client or Redis error), the request is
// allowed through. This is approved behavior for non-security-sensitive operations.
func searchRateLimitMiddleware(redisClient *rdb.Client, log *zap.Logger) func(http.Handler) http.Handler {
	const (
		maxAttempts int64         = 30
		window      time.Duration = 15 * time.Minute
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Fail-open: nil Redis client means allow the request.
			if redisClient == nil {
				log.Warn("search rate limit: Redis client is nil — failing open (approved behavior)")
				next.ServeHTTP(w, r)
				return
			}

			subject := realIP(r)
			if id, ok := auth.UserIDFromContext(r.Context()); ok {
				subject = id.String()
			}

			key := fmt.Sprintf("rl:search:%s", subject)
			ctx := r.Context()

			// Initialize key with TTL only if it does not already exist, then increment.
			if err := redisClient.SetNX(ctx, key, 0, window).Err(); err != nil {
				// Fail-open on Redis error for search (non-security-sensitive).
				log.Warn("search rate limit: Redis SET NX failed — failing open",
					zap.String("subject", subject),
					zap.Error(err),
				)
				next.ServeHTTP(w, r)
				return
			}

			count, err := redisClient.Incr(ctx, key).Result()
			if err != nil {
				// Fail-open on Redis error.
				log.Warn("search rate limit: Redis INCR failed — failing open",
					zap.String("subject", subject),
					zap.Error(err),
				)
				next.ServeHTTP(w, r)
				return
			}

			// Belt-and-suspenders: ensure TTL is always set.
			if count == 1 {
				if expErr := redisClient.Expire(ctx, key, window).Err(); expErr != nil {
					log.Warn("search rate limit: Redis EXPIRE failed after count==1",
						zap.String("key", key),
						zap.Error(expErr),
					)
				}
			}

			if count > maxAttempts {
				w.Header().Set("Retry-After", strconv.FormatInt(int64(window.Seconds()), 10))
				apierror.Render(w, http.StatusTooManyRequests,
					apierror.New(apierror.CodeRateLimit, "Too many search requests. Please try again later."))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// realIP extracts the client IP address from the request. It respects the
// X-Real-IP and X-Forwarded-For headers set by chi's RealIP middleware.
func realIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.SplitN(fwd, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx > 0 {
		return addr[:idx]
	}
	return addr
}
