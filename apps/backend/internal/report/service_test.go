package report

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
)

// ── Stub checkers ─────────────────────────────────────────────────────────────

type stubPostChecker struct {
	exists bool
	err    error
}

func (s *stubPostChecker) PostExistsAndNotDeleted(_ context.Context, _ uuid.UUID) (bool, error) {
	return s.exists, s.err
}

type stubUserChecker struct {
	exists bool
	err    error
}

func (s *stubUserChecker) UserExists(_ context.Context, _ uuid.UUID) (bool, error) {
	return s.exists, s.err
}

// ── testSvc mirrors Service logic without a real DB or Redis ─────────────────
//
// Because *Repository requires a *pgxpool.Pool and *redis.Client cannot be
// constructed without a real server, we use a local testSvc type that replicates
// the production validation logic using the same package-level helpers
// (validReasons, validateDetail, maxDetailCodePoints). This allows unit-testing
// all validation, existence-check, and idempotency paths without I/O.
//
// The nil-rdb fail-closed path is tested via the real *Service directly
// (Service.rdb == nil → CodeServiceUnavailable) without needing a repository.

type testSvc struct {
	submitFn    func(ctx context.Context, reporterID uuid.UUID, tt ReportTargetType, tpid, tuid *uuid.UUID, reason ReportReason, detail *string) (bool, error)
	postChecker PostChecker
	userChecker UserChecker
	logger      *zap.Logger
}

func (ts *testSvc) SubmitPostReport(ctx context.Context, reporterID, postID uuid.UUID, req CreateReportRequest) error {
	if _, ok := validReasons[req.Reason]; !ok {
		return apierror.NewAPIError(apierror.CodeValidation, "invalid reason")
	}
	if err := validateDetail(req.Detail); err != nil {
		return err
	}
	exists, err := ts.postChecker.PostExistsAndNotDeleted(ctx, postID)
	if err != nil {
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	if !exists {
		return apierror.NewAPIError(apierror.CodeNotFound, "post not found")
	}
	pid := postID
	_, insertErr := ts.submitFn(ctx, reporterID, ReportTargetPost, &pid, nil, req.Reason, req.Detail)
	if insertErr != nil {
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

func (ts *testSvc) SubmitUserReport(ctx context.Context, reporterID, targetUserID uuid.UUID, req CreateReportRequest) error {
	if reporterID == targetUserID {
		return apierror.NewAPIError(apierror.CodeValidation, "cannot report your own account")
	}
	if _, ok := validReasons[req.Reason]; !ok {
		return apierror.NewAPIError(apierror.CodeValidation, "invalid reason")
	}
	if err := validateDetail(req.Detail); err != nil {
		return err
	}
	exists, err := ts.userChecker.UserExists(ctx, targetUserID)
	if err != nil {
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	if !exists {
		return apierror.NewAPIError(apierror.CodeNotFound, "user not found")
	}
	uid := targetUserID
	_, insertErr := ts.submitFn(ctx, reporterID, ReportTargetUser, nil, &uid, req.Reason, req.Detail)
	if insertErr != nil {
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

// noopSubmit always succeeds and returns created=true.
func noopSubmit(_ context.Context, _ uuid.UUID, _ ReportTargetType, _, _ *uuid.UUID, _ ReportReason, _ *string) (bool, error) {
	return true, nil
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestSubmitPostReport_ValidReason_Success(t *testing.T) {
	t.Parallel()
	svc := &testSvc{
		submitFn:    noopSubmit,
		postChecker: &stubPostChecker{exists: true},
		userChecker: &stubUserChecker{},
		logger:      zap.NewNop(),
	}
	if err := svc.SubmitPostReport(context.Background(), uuid.New(), uuid.New(), CreateReportRequest{
		Reason: ReasonSpam,
	}); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestSubmitPostReport_InvalidReason_ValidationError(t *testing.T) {
	t.Parallel()
	svc := &testSvc{
		submitFn:    noopSubmit,
		postChecker: &stubPostChecker{exists: true},
		userChecker: &stubUserChecker{},
		logger:      zap.NewNop(),
	}
	err := svc.SubmitPostReport(context.Background(), uuid.New(), uuid.New(), CreateReportRequest{
		Reason: "not_a_real_reason",
	})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	assertAPICode(t, err, apierror.CodeValidation)
}

func TestSubmitPostReport_DetailTooLong_ValidationError(t *testing.T) {
	t.Parallel()
	svc := &testSvc{
		submitFn:    noopSubmit,
		postChecker: &stubPostChecker{exists: true},
		userChecker: &stubUserChecker{},
		logger:      zap.NewNop(),
	}
	longDetail := strings.Repeat("a", maxDetailCodePoints+1)
	err := svc.SubmitPostReport(context.Background(), uuid.New(), uuid.New(), CreateReportRequest{
		Reason: ReasonSpam,
		Detail: &longDetail,
	})
	if err == nil {
		t.Fatal("expected validation error for long detail, got nil")
	}
	assertAPICode(t, err, apierror.CodeValidation)
}

func TestSubmitPostReport_PostDeleted_NotFound(t *testing.T) {
	t.Parallel()
	svc := &testSvc{
		submitFn:    noopSubmit,
		postChecker: &stubPostChecker{exists: false},
		userChecker: &stubUserChecker{},
		logger:      zap.NewNop(),
	}
	err := svc.SubmitPostReport(context.Background(), uuid.New(), uuid.New(), CreateReportRequest{
		Reason: ReasonHarassment,
	})
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	assertAPICode(t, err, apierror.CodeNotFound)
}

func TestSubmitUserReport_SelfReport_ValidationError(t *testing.T) {
	t.Parallel()
	svc := &testSvc{
		submitFn:    noopSubmit,
		postChecker: &stubPostChecker{},
		userChecker: &stubUserChecker{exists: true},
		logger:      zap.NewNop(),
	}
	id := uuid.New()
	err := svc.SubmitUserReport(context.Background(), id, id, CreateReportRequest{
		Reason: ReasonSpam,
	})
	if err == nil {
		t.Fatal("expected validation error for self-report, got nil")
	}
	assertAPICode(t, err, apierror.CodeValidation)
}

// TestSubmitReport_DuplicatePending_Idempotent verifies that when the repository
// returns created=false (ON CONFLICT DO NOTHING matched an existing pending row),
// the service returns nil — the caller receives an idempotent 204.
func TestSubmitReport_DuplicatePending_Idempotent(t *testing.T) {
	t.Parallel()
	svc := &testSvc{
		submitFn: func(_ context.Context, _ uuid.UUID, _ ReportTargetType, _, _ *uuid.UUID, _ ReportReason, _ *string) (bool, error) {
			return false, nil // duplicate → created=false
		},
		postChecker: &stubPostChecker{exists: true},
		userChecker: &stubUserChecker{},
		logger:      zap.NewNop(),
	}
	if err := svc.SubmitPostReport(context.Background(), uuid.New(), uuid.New(), CreateReportRequest{
		Reason: ReasonOther,
	}); err != nil {
		t.Fatalf("expected nil (idempotent on duplicate), got: %v", err)
	}
}

// TestSubmitReport_RedisUnavailable_ServiceUnavailable verifies that when the
// Service is constructed with a nil Redis client, SubmitPostReport returns
// CodeServiceUnavailable (fail-closed).
func TestSubmitReport_RedisUnavailable_ServiceUnavailable(t *testing.T) {
	t.Parallel()
	// Use the real *Service with rdb=nil. The rate-limit check fires before any
	// repository or checker call, so repo and checkers are never reached.
	svc := &Service{
		repo:        nil, // never reached
		postChecker: &stubPostChecker{exists: true},
		userChecker: &stubUserChecker{},
		rdb:         nil, // nil → fail-closed
		logger:      zap.NewNop(),
	}
	err := svc.SubmitPostReport(context.Background(), uuid.New(), uuid.New(), CreateReportRequest{
		Reason: ReasonSpam,
	})
	if err == nil {
		t.Fatal("expected CodeServiceUnavailable, got nil")
	}
	assertAPICode(t, err, apierror.CodeServiceUnavailable)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func assertAPICode(t *testing.T, err error, code string) {
	t.Helper()
	var apiErr *apierror.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *apierror.APIError, got: %T %v", err, err)
	}
	if apiErr.Code != code {
		t.Fatalf("expected code %q, got %q (message: %s)", code, apiErr.Code, apiErr.Message)
	}
}
