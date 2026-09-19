package ctxlog_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap/zapcore"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/ctxlog"
)

// TestRequestIDField_WithID verifies that a context containing a chi request ID
// returns a zap.Field with key "request_id" and the expected string value.
func TestRequestIDField_WithID(t *testing.T) {
	const wantID = "test-req-123"
	ctx := context.WithValue(context.Background(), chimw.RequestIDKey, wantID)

	field := ctxlog.RequestIDField(ctx)

	if field.Type == zapcore.SkipType {
		t.Fatal("RequestIDField returned zap.Skip() but expected a string field")
	}
	if field.Key != "request_id" {
		t.Errorf("field key: want %q, got %q", "request_id", field.Key)
	}
	if field.String != wantID {
		t.Errorf("field value: want %q, got %q", wantID, field.String)
	}
}

// TestRequestIDField_EmptyContext verifies that a context without a chi request ID
// returns zap.Skip() so that no empty "request_id" field is written to the log.
func TestRequestIDField_EmptyContext(t *testing.T) {
	field := ctxlog.RequestIDField(context.Background())

	if field.Type != zapcore.SkipType {
		t.Errorf("expected zap.Skip() for empty context, got field type %v (key=%q, value=%q)",
			field.Type, field.Key, field.String)
	}
}

// TestRequestIDField_SameIDAsAccessLog verifies that RequestIDField reads the same
// request ID that chimw.GetReqID reads — i.e., they both use the same context key
// and neither can produce a divergent ID.
func TestRequestIDField_SameIDAsAccessLog(t *testing.T) {
	const wantID = "shared-req-id-456"
	ctx := context.WithValue(context.Background(), chimw.RequestIDKey, wantID)

	// Simulate what the access log middleware reads:
	accessLogID := chimw.GetReqID(ctx)

	// Simulate what a service log call reads:
	field := ctxlog.RequestIDField(ctx)

	if accessLogID != wantID {
		t.Errorf("chimw.GetReqID: want %q, got %q", wantID, accessLogID)
	}
	if field.String != wantID {
		t.Errorf("RequestIDField value: want %q, got %q", wantID, field.String)
	}
	if accessLogID != field.String {
		t.Errorf("access log ID %q != service log field %q — IDs diverged", accessLogID, field.String)
	}
}

// TestRequestID_EndToEnd creates an httptest request, runs it through a chi router
// with chimw.RequestID middleware, and confirms that the request ID read by a
// handler via RequestIDField is identical to the one set by chi middleware.
func TestRequestID_EndToEnd(t *testing.T) {
	var (
		capturedFieldValue string
		capturedChiValue   string
	)

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		capturedChiValue = chimw.GetReqID(r.Context())
		field := ctxlog.RequestIDField(r.Context())
		capturedFieldValue = field.String
		w.WriteHeader(200)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if capturedChiValue == "" {
		t.Fatal("chimw.GetReqID returned empty string — RequestID middleware may not have run")
	}
	if capturedFieldValue == "" {
		t.Fatal("RequestIDField returned empty string inside a chi request context")
	}
	if capturedChiValue != capturedFieldValue {
		t.Errorf("chi ID %q != RequestIDField value %q", capturedChiValue, capturedFieldValue)
	}
}
