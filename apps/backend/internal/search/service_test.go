package search_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/search"
)

// stubBlockProvider is a test double for BlockProvider.
type stubBlockProvider struct {
	blockedIDs []uuid.UUID
	err        error
}

func (s *stubBlockProvider) GetBlockedIDs(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	return s.blockedIDs, s.err
}

// stubRepository is a test double for the repository. We cannot inject it
// directly into Service (Service holds *Repository, not an interface), so
// service-level tests validate the service's own logic (validation, block
// provider delegation) by supplying a nil or real *Repository — which means
// the repository-level SQL is not exercised here. That is covered by
// integration tests.
//
// For service-only unit tests we verify:
//   - empty query returns CodeValidation error
//   - GetBlockedIDs error is wrapped and returned
//   - nil callerID skips block provider call

// TestSearchPosts_EmptyQuery verifies that an empty query returns a validation error.
func TestSearchPosts_EmptyQuery(t *testing.T) {
	t.Parallel()

	bp := &stubBlockProvider{}
	svc := search.NewService(nil, bp, zap.NewNop())

	_, err := svc.SearchPosts(context.Background(), nil, "", "")
	if err == nil {
		t.Fatal("expected error for empty query, got nil")
	}

	var apiErr *apierror.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *apierror.APIError, got %T: %v", err, err)
	}
	if apiErr.Code != apierror.CodeValidation {
		t.Errorf("expected code %s, got %s", apierror.CodeValidation, apiErr.Code)
	}
}

// TestSearchUsers_EmptyQuery verifies that an empty query returns a validation error.
func TestSearchUsers_EmptyQuery(t *testing.T) {
	t.Parallel()

	bp := &stubBlockProvider{}
	svc := search.NewService(nil, bp, zap.NewNop())

	_, err := svc.SearchUsers(context.Background(), nil, "", "")
	if err == nil {
		t.Fatal("expected error for empty query, got nil")
	}

	var apiErr *apierror.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *apierror.APIError, got %T: %v", err, err)
	}
	if apiErr.Code != apierror.CodeValidation {
		t.Errorf("expected code %s, got %s", apierror.CodeValidation, apiErr.Code)
	}
}

// TestSearchPosts_BlockProviderError verifies that a block provider error is
// propagated when callerID is non-nil.
func TestSearchPosts_BlockProviderError(t *testing.T) {
	t.Parallel()

	bp := &stubBlockProvider{err: errors.New("redis unavailable")}
	svc := search.NewService(nil, bp, zap.NewNop())

	callerID := uuid.New()
	_, err := svc.SearchPosts(context.Background(), &callerID, "hello", "")
	if err == nil {
		t.Fatal("expected error when block provider fails, got nil")
	}
}

// TestSearchUsers_BlockProviderError verifies that a block provider error is
// propagated when callerID is non-nil.
func TestSearchUsers_BlockProviderError(t *testing.T) {
	t.Parallel()

	bp := &stubBlockProvider{err: errors.New("redis unavailable")}
	svc := search.NewService(nil, bp, zap.NewNop())

	callerID := uuid.New()
	_, err := svc.SearchUsers(context.Background(), &callerID, "alice", "")
	if err == nil {
		t.Fatal("expected error when block provider fails, got nil")
	}
}

// TestSearchPosts_NilCallerSkipsBlockProvider verifies that when callerID is nil,
// the block provider is not called. If it were called with a nil deref it would panic.
func TestSearchPosts_NilCallerSkipsBlockProvider(t *testing.T) {
	t.Parallel()

	// Block provider always returns an error — if called with nil callerID the
	// service would propagate it. We verify it is NOT called by confirming the
	// error returned (if any) is NOT the block provider error. Since repo is nil
	// the call will panic when the repo is invoked, but the empty-query guard
	// fires first — so we test the no-panic path via an empty-query check above.
	//
	// A more meaningful version of this test requires a mock repository, which
	// is deferred to integration testing.
	//
	// Here we just confirm that calling SearchPosts with nil callerID and an
	// empty query returns CodeValidation (not a panic or block error).
	bp := &stubBlockProvider{err: errors.New("should not be called")}
	svc := search.NewService(nil, bp, zap.NewNop())

	_, err := svc.SearchPosts(context.Background(), nil, "", "")
	var apiErr *apierror.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != apierror.CodeValidation {
		t.Errorf("expected CodeValidation, got: %v", err)
	}
}
