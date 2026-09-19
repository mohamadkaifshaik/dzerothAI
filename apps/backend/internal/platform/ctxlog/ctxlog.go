// Package ctxlog provides helpers that bridge the chi request context and the
// zap structured logger. It contains no business logic and introduces no new
// ID-generation mechanism — it only reads the request ID that chimw.RequestID
// has already stored in the context.
package ctxlog

import (
	"context"

	chimw "github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

// RequestIDField extracts the chi request ID from ctx and returns a zap.Field
// suitable for inclusion in any zap log call.
//
// When a chi request ID is present in ctx (i.e., chimw.RequestID middleware ran
// before the current code path), the returned field has key "request_id" and the
// string value of the ID.
//
// When no request ID is present — for example in background goroutines, tests
// that do not set up chi middleware, or startup/shutdown log calls — the function
// returns zap.Skip(), which causes zap to omit the field entirely. An empty
// string is never written to the log.
//
// Usage:
//
//	s.log.Error("feed: blocked ids", ctxlog.RequestIDField(ctx), zap.Error(err))
func RequestIDField(ctx context.Context) zap.Field {
	id := chimw.GetReqID(ctx)
	if id == "" {
		return zap.Skip()
	}
	return zap.String("request_id", id)
}
