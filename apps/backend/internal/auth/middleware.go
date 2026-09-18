package auth

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	rdb "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
)

// contextKey is an unexported type for request-context keys to avoid collisions.
type contextKey int

const (
	ctxKeyUserID     contextKey = iota
	ctxKeySessionJTI contextKey = iota
)

// UserIDFromContext retrieves the authenticated user's UUID from the request context.
// Returns (uuid, true) if present, (zero, false) if the request was not authenticated.
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(ctxKeyUserID).(uuid.UUID)
	return v, ok
}

// SessionJTIFromContext retrieves the JWT jti claim from the request context.
// Returns (jti, true) if present, ("", false) if not set.
func SessionJTIFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(ctxKeySessionJTI).(string)
	return v, ok
}

// JWTMiddleware extracts and validates the Bearer JWT from the Authorization header.
// On success it sets the user UUID and jti in the request context and calls next.
// On failure it returns 401 with an apierror body. No database query is made.
func JWTMiddleware(jwtSecret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				apierror.Render(w, http.StatusUnauthorized,
					apierror.New(apierror.CodeUnauthorized, "Authorization header is required."))
				return
			}

			const prefix = "Bearer "
			if !strings.HasPrefix(authHeader, prefix) {
				apierror.Render(w, http.StatusUnauthorized,
					apierror.New(apierror.CodeUnauthorized, "Authorization header must use Bearer scheme."))
				return
			}

			tokenStr := strings.TrimPrefix(authHeader, prefix)
			claims, err := ValidateAccessToken(tokenStr, jwtSecret)
			if err != nil {
				apierror.Render(w, http.StatusUnauthorized,
					apierror.New(apierror.CodeUnauthorized, "Invalid or expired access token."))
				return
			}

			userID, err := uuid.Parse(claims.Subject)
			if err != nil {
				apierror.Render(w, http.StatusUnauthorized,
					apierror.New(apierror.CodeUnauthorized, "Invalid token subject."))
				return
			}

			ctx := context.WithValue(r.Context(), ctxKeyUserID, userID)
			ctx = context.WithValue(ctx, ctxKeySessionJTI, claims.ID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RateLimitConfig holds the parameters for a single rate-limit rule.
type RateLimitConfig struct {
	// Operation is used in the Redis key, e.g. "login" or "register".
	Operation string
	// MaxAttempts is the maximum number of requests allowed in the window.
	MaxAttempts int64
	// Window is the duration of the sliding window.
	Window time.Duration
}

// RateLimitMiddleware enforces a Redis-backed rate limit for authentication endpoints.
//
// Redis key pattern: rl:auth:{operation}:{ip}
//
// When Redis is unavailable the middleware FAILS CLOSED by returning HTTP 503.
// This is a security requirement per ADR 0005 and the approved Redis failure policy.
func RateLimitMiddleware(redisClient *rdb.Client, cfg RateLimitConfig, log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := realIP(r)
			key := fmt.Sprintf("rl:auth:%s:%s", cfg.Operation, ip)

			// Fail closed: if Redis is unreachable, block the request.
			if redisClient == nil {
				log.Error("rate limit: Redis client is nil — failing closed",
					zap.String("operation", cfg.Operation),
					zap.String("ip", ip),
				)
				apierror.Render(w, http.StatusServiceUnavailable,
					apierror.New(apierror.CodeServiceUnavailable, "Service temporarily unavailable."))
				return
			}

			ctx := r.Context()

			// Atomically initialize the key with a TTL only when it does not exist yet
			// (SET NX PX), then unconditionally increment the counter. This two-command
			// sequence is not a single atomic transaction, but the NX initialization
			// ensures the TTL is always set before the counter can reach 1, eliminating
			// the previous INCR+conditional-EXPIRE race where a failed EXPIRE would leave
			// a key without a TTL (keys with no TTL persist indefinitely in Redis, which
			// could permanently lock out an IP with no recovery path).
			//
			// Race analysis: between SET NX and INCR another goroutine may delete the key
			// (unlikely) or the key may already exist (most common path). In both cases the
			// subsequent INCR is safe: it either increments an existing TTL-bearing key or
			// creates a new counter=1 key without a TTL. The latter can only happen if Redis
			// loses the key between the two commands, which is operationally equivalent to a
			// clean window start — and the INCR-only path below would set a fresh EXPIRE.
			// To guard the residual edge case, always set EXPIRE after INCR when count==1.
			if err := redisClient.SetNX(ctx, key, 0, cfg.Window).Err(); err != nil {
				// Redis error on the initialization step — fail closed.
				log.Error("rate limit: Redis SET NX failed — failing closed",
					zap.String("operation", cfg.Operation),
					zap.String("ip", ip),
					zap.Error(err),
				)
				apierror.Render(w, http.StatusServiceUnavailable,
					apierror.New(apierror.CodeServiceUnavailable, "Service temporarily unavailable."))
				return
			}

			count, err := redisClient.Incr(ctx, key).Result()
			if err != nil {
				// Redis error — fail closed per security requirement.
				log.Error("rate limit: Redis INCR failed — failing closed",
					zap.String("operation", cfg.Operation),
					zap.String("ip", ip),
					zap.Error(err),
				)
				apierror.Render(w, http.StatusServiceUnavailable,
					apierror.New(apierror.CodeServiceUnavailable, "Service temporarily unavailable."))
				return
			}

			// Belt-and-suspenders: if count==1 the key was either just created by INCR
			// (meaning SET NX raced) or the key existed but had no TTL from a prior partial
			// failure. Set the TTL now to ensure the window always closes.
			if count == 1 {
				if expErr := redisClient.Expire(ctx, key, cfg.Window).Err(); expErr != nil {
					// EXPIRE failure is logged but does not block the request. The SET NX
					// above already set the TTL in the common case; this is a safety net.
					log.Error("rate limit: Redis EXPIRE failed after count==1",
						zap.String("key", key),
						zap.Error(expErr),
					)
				}
			}

			if count > cfg.MaxAttempts {
				w.Header().Set("Retry-After", strconv.FormatInt(int64(cfg.Window.Seconds()), 10))
				apierror.Render(w, http.StatusTooManyRequests,
					apierror.New(apierror.CodeRateLimit, "Too many requests. Please try again later."))
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
	// Fall back to RemoteAddr (strip port).
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx > 0 {
		return addr[:idx]
	}
	return addr
}
