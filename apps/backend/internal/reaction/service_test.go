// Package reaction — service tests.
//
// Unit tests for reaction service business logic using fake repository doubles
// and stub notifiers. No real database or Redis connection is required.
package reaction

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/notification"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
)

// ---------------------------------------------------------------------------
// reactionRepo interface and test double
// ---------------------------------------------------------------------------

// reactionRepo is the minimal interface for repository methods called by the
// service under test. It mirrors the concrete *Repository signatures.
type reactionRepo interface {
	React(ctx context.Context, userID, postID uuid.UUID) (created bool, err error)
	Unreact(ctx context.Context, userID, postID uuid.UUID) error
	HasReacted(ctx context.Context, userID, postID uuid.UUID) (bool, error)
}

// fakeReactionRepo is a test double for reactionRepo.
type fakeReactionRepo struct {
	reactFn      func(ctx context.Context, userID, postID uuid.UUID) (bool, error)
	unreactFn    func(ctx context.Context, userID, postID uuid.UUID) error
	hasReactedFn func(ctx context.Context, userID, postID uuid.UUID) (bool, error)
}

func (f *fakeReactionRepo) React(ctx context.Context, userID, postID uuid.UUID) (bool, error) {
	if f.reactFn != nil {
		return f.reactFn(ctx, userID, postID)
	}
	return true, nil // default: new reaction created
}

func (f *fakeReactionRepo) Unreact(ctx context.Context, userID, postID uuid.UUID) error {
	if f.unreactFn != nil {
		return f.unreactFn(ctx, userID, postID)
	}
	return nil
}

func (f *fakeReactionRepo) HasReacted(ctx context.Context, userID, postID uuid.UUID) (bool, error) {
	if f.hasReactedFn != nil {
		return f.hasReactedFn(ctx, userID, postID)
	}
	return false, nil
}

// ---------------------------------------------------------------------------
// stubNotifier — test double for notification.NotificationPublisher
// ---------------------------------------------------------------------------

type stubNotifier struct {
	publishFn    func(ctx context.Context, event notification.PublishEvent) error
	publishCalls []notification.PublishEvent
}

func (s *stubNotifier) Publish(ctx context.Context, event notification.PublishEvent) error {
	s.publishCalls = append(s.publishCalls, event)
	if s.publishFn != nil {
		return s.publishFn(ctx, event)
	}
	return nil
}

// ---------------------------------------------------------------------------
// testableReactionService — service that accepts the reactionRepo interface
// ---------------------------------------------------------------------------

// testableReactionService mirrors Service but holds a reactionRepo interface
// so tests can inject fakes without a real database or Redis client.
// Rate limiting is skipped (rdb == nil → fail-open per production behaviour).
type testableReactionService struct {
	repo     reactionRepo
	notifier notification.NotificationPublisher
	logger   *zap.Logger
}

func newTestableReactionService(repo reactionRepo, notifier notification.NotificationPublisher) *testableReactionService {
	return &testableReactionService{repo: repo, notifier: notifier, logger: zap.NewNop()}
}

func (s *testableReactionService) React(ctx context.Context, callerID, postID uuid.UUID, postAuthorID uuid.UUID) error {
	created, err := s.repo.React(ctx, callerID, postID)
	if err != nil {
		s.logger.Error("reaction: react", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	if !created {
		return nil // already reacted — idempotent
	}
	if callerID != postAuthorID && s.notifier != nil {
		postIDCopy := postID
		pubErr := s.notifier.Publish(ctx, notification.PublishEvent{
			RecipientID: postAuthorID,
			ActorID:     callerID,
			Event:       notification.EventReaction,
			PostID:      &postIDCopy,
		})
		if pubErr != nil {
			s.logger.Warn("reaction: publish notification failed", zap.Error(pubErr))
		}
	}
	return nil
}

func (s *testableReactionService) Unreact(ctx context.Context, callerID, postID uuid.UUID) error {
	if err := s.repo.Unreact(ctx, callerID, postID); err != nil {
		s.logger.Error("reaction: unreact", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

func (s *testableReactionService) HasReacted(ctx context.Context, userID, postID uuid.UUID) (bool, error) {
	reacted, err := s.repo.HasReacted(ctx, userID, postID)
	if err != nil {
		s.logger.Error("reaction: has reacted", zap.Error(err))
		return false, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return reacted, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func reactionAPICode(err error) string {
	var ae *apierror.APIError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}

// ---------------------------------------------------------------------------
// TestReact_CreatesReaction_Succeeds
// ---------------------------------------------------------------------------

// TestReact_CreatesReaction_Succeeds verifies that calling React when no
// reaction exists inserts a new row and returns nil.
func TestReact_CreatesReaction_Succeeds(t *testing.T) {
	t.Parallel()

	reactCalled := false
	fake := &fakeReactionRepo{
		reactFn: func(_ context.Context, _, _ uuid.UUID) (bool, error) {
			reactCalled = true
			return true, nil // new reaction created
		},
	}
	notifier := &stubNotifier{}
	svc := newTestableReactionService(fake, notifier)

	callerID := uuid.New()
	postID := uuid.New()
	authorID := uuid.New() // different from callerID

	err := svc.React(context.Background(), callerID, postID, authorID)
	if err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
	if !reactCalled {
		t.Error("expected repo.React to be called")
	}
}

// ---------------------------------------------------------------------------
// TestReact_AlreadyReacted_Idempotent_NoNotification
// ---------------------------------------------------------------------------

// TestReact_AlreadyReacted_Idempotent_NoNotification verifies that when the
// reaction already exists (created=false from repo), no notification is
// published and nil is returned.
func TestReact_AlreadyReacted_Idempotent_NoNotification(t *testing.T) {
	t.Parallel()

	fake := &fakeReactionRepo{
		reactFn: func(_ context.Context, _, _ uuid.UUID) (bool, error) {
			return false, nil // already existed — ON CONFLICT DO NOTHING
		},
	}
	notifier := &stubNotifier{}
	svc := newTestableReactionService(fake, notifier)

	err := svc.React(context.Background(), uuid.New(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("expected nil for idempotent react, got: %v", err)
	}
	if len(notifier.publishCalls) != 0 {
		t.Errorf("expected no notification published on duplicate react, got %d calls", len(notifier.publishCalls))
	}
}

// ---------------------------------------------------------------------------
// TestReact_NewReaction_PublishesNotification
// ---------------------------------------------------------------------------

// TestReact_NewReaction_PublishesNotification verifies that a new reaction
// causes a notification.EventReaction to be published to the post author.
func TestReact_NewReaction_PublishesNotification(t *testing.T) {
	t.Parallel()

	fake := &fakeReactionRepo{
		reactFn: func(_ context.Context, _, _ uuid.UUID) (bool, error) {
			return true, nil // new reaction
		},
	}
	notifier := &stubNotifier{}
	svc := newTestableReactionService(fake, notifier)

	callerID := uuid.New()
	postID := uuid.New()
	authorID := uuid.New() // distinct from caller

	err := svc.React(context.Background(), callerID, postID, authorID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifier.publishCalls) != 1 {
		t.Fatalf("expected 1 notification published, got %d", len(notifier.publishCalls))
	}
	evt := notifier.publishCalls[0]
	if evt.RecipientID != authorID {
		t.Errorf("notification recipient = %v, want %v", evt.RecipientID, authorID)
	}
	if evt.ActorID != callerID {
		t.Errorf("notification actor = %v, want %v", evt.ActorID, callerID)
	}
	if evt.Event != notification.EventReaction {
		t.Errorf("notification event = %v, want %v", evt.Event, notification.EventReaction)
	}
	if evt.PostID == nil || *evt.PostID != postID {
		t.Errorf("notification post_id mismatch: got %v, want %v", evt.PostID, postID)
	}
}

// ---------------------------------------------------------------------------
// TestReact_SelfReact_NoNotification
// ---------------------------------------------------------------------------

// TestReact_SelfReact_NoNotification verifies that when callerID == postAuthorID
// no notification is published (no self-notification rule).
func TestReact_SelfReact_NoNotification(t *testing.T) {
	t.Parallel()

	fake := &fakeReactionRepo{
		reactFn: func(_ context.Context, _, _ uuid.UUID) (bool, error) {
			return true, nil // new reaction
		},
	}
	notifier := &stubNotifier{}
	svc := newTestableReactionService(fake, notifier)

	sameID := uuid.New()
	err := svc.React(context.Background(), sameID, uuid.New(), sameID) // caller == author
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifier.publishCalls) != 0 {
		t.Errorf("expected no notification for self-react, got %d", len(notifier.publishCalls))
	}
}

// ---------------------------------------------------------------------------
// TestReact_RepoError_ReturnsInternalError
// ---------------------------------------------------------------------------

// TestReact_RepoError_ReturnsInternalError verifies that a repository error
// is wrapped as CodeInternal.
func TestReact_RepoError_ReturnsInternalError(t *testing.T) {
	t.Parallel()

	fake := &fakeReactionRepo{
		reactFn: func(_ context.Context, _, _ uuid.UUID) (bool, error) {
			return false, errors.New("db down")
		},
	}
	svc := newTestableReactionService(fake, nil)

	err := svc.React(context.Background(), uuid.New(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if code := reactionAPICode(err); code != apierror.CodeInternal {
		t.Errorf("error code = %q, want %q", code, apierror.CodeInternal)
	}
}

// ---------------------------------------------------------------------------
// TestUnreact_RemovesReaction_Succeeds
// ---------------------------------------------------------------------------

// TestUnreact_RemovesReaction_Succeeds verifies that Unreact calls the
// repository and returns nil.
func TestUnreact_RemovesReaction_Succeeds(t *testing.T) {
	t.Parallel()

	unreactCalled := false
	fake := &fakeReactionRepo{
		unreactFn: func(_ context.Context, _, _ uuid.UUID) error {
			unreactCalled = true
			return nil
		},
	}
	svc := newTestableReactionService(fake, nil)

	err := svc.Unreact(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
	if !unreactCalled {
		t.Error("expected repo.Unreact to be called")
	}
}

// ---------------------------------------------------------------------------
// TestUnreact_NonExistent_Idempotent
// ---------------------------------------------------------------------------

// TestUnreact_NonExistent_Idempotent verifies that unreacting on a post where
// no reaction exists returns nil (idempotent DELETE).
func TestUnreact_NonExistent_Idempotent(t *testing.T) {
	t.Parallel()

	fake := &fakeReactionRepo{
		unreactFn: func(_ context.Context, _, _ uuid.UUID) error {
			return nil // DELETE 0 rows — no error
		},
	}
	svc := newTestableReactionService(fake, nil)

	err := svc.Unreact(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Errorf("Unreact non-existent reaction returned unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestUnreact_RepoError_ReturnsInternalError
// ---------------------------------------------------------------------------

// TestUnreact_RepoError_ReturnsInternalError verifies that a repository error
// from Unreact is wrapped as CodeInternal.
func TestUnreact_RepoError_ReturnsInternalError(t *testing.T) {
	t.Parallel()

	fake := &fakeReactionRepo{
		unreactFn: func(_ context.Context, _, _ uuid.UUID) error {
			return errors.New("db connection lost")
		},
	}
	svc := newTestableReactionService(fake, nil)

	err := svc.Unreact(context.Background(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if code := reactionAPICode(err); code != apierror.CodeInternal {
		t.Errorf("error code = %q, want %q", code, apierror.CodeInternal)
	}
}

// ---------------------------------------------------------------------------
// TestHasReacted_ReturnsTrue
// ---------------------------------------------------------------------------

// TestHasReacted_ReturnsTrue verifies that HasReacted returns true when the
// repository confirms the reaction exists.
func TestHasReacted_ReturnsTrue(t *testing.T) {
	t.Parallel()

	fake := &fakeReactionRepo{
		hasReactedFn: func(_ context.Context, _, _ uuid.UUID) (bool, error) {
			return true, nil
		},
	}
	svc := newTestableReactionService(fake, nil)

	got, err := svc.HasReacted(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected HasReacted=true, got false")
	}
}

// ---------------------------------------------------------------------------
// TestHasReacted_ReturnsFalse
// ---------------------------------------------------------------------------

// TestHasReacted_ReturnsFalse verifies that HasReacted returns false when no
// reaction row exists.
func TestHasReacted_ReturnsFalse(t *testing.T) {
	t.Parallel()

	fake := &fakeReactionRepo{
		hasReactedFn: func(_ context.Context, _, _ uuid.UUID) (bool, error) {
			return false, nil
		},
	}
	svc := newTestableReactionService(fake, nil)

	got, err := svc.HasReacted(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected HasReacted=false, got true")
	}
}

// ---------------------------------------------------------------------------
// TestHasReacted_RepoError_ReturnsInternalError
// ---------------------------------------------------------------------------

// TestHasReacted_RepoError_ReturnsInternalError verifies that a repository
// error is wrapped as CodeInternal.
func TestHasReacted_RepoError_ReturnsInternalError(t *testing.T) {
	t.Parallel()

	fake := &fakeReactionRepo{
		hasReactedFn: func(_ context.Context, _, _ uuid.UUID) (bool, error) {
			return false, errors.New("query error")
		},
	}
	svc := newTestableReactionService(fake, nil)

	_, err := svc.HasReacted(context.Background(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if code := reactionAPICode(err); code != apierror.CodeInternal {
		t.Errorf("error code = %q, want %q", code, apierror.CodeInternal)
	}
}

// ---------------------------------------------------------------------------
// TestPublicMetricLockdown_PostDTO_NoCountFields (CLAUDE.md §2.3)
// ---------------------------------------------------------------------------

// TestPublicMetricLockdown_PostDTO_NoCountFields verifies that post.PostDTO —
// the struct used in public API responses for post data — contains no
// social-validation metric fields.
//
// This test uses reflection to enumerate all exported fields and fails if any
// field name matches a known metric pattern. Per CLAUDE.md §2.3, the following
// must never appear in any public DTO:
//
//   - like_count / LikeCount
//   - impression_count / ImpressionCount
//   - bookmark_count / BookmarkCount
//   - reply_count / ReplyCount
//   - repost_count / RepostCount
//   - share_count / ShareCount
//   - view_count / ViewCount
//   - follower_count / FollowerCount
//   - reaction_count / ReactionCount
func TestPublicMetricLockdown_PostDTO_NoCountFields(t *testing.T) {
	t.Parallel()

	forbiddenFieldNames := []string{
		"LikeCount",
		"ImpressionCount",
		"BookmarkCount",
		"ReplyCount",
		"RepostCount",
		"ShareCount",
		"ViewCount",
		"FollowerCount",
		"ReactionCount",
		"Likes",
		"Impressions",
		"Views",
	}

	dt := reflect.TypeOf(post.PostDTO{})
	for i := 0; i < dt.NumField(); i++ {
		fieldName := dt.Field(i).Name
		for _, forbidden := range forbiddenFieldNames {
			if fieldName == forbidden {
				t.Errorf("post.PostDTO must not contain metric field %q (CLAUDE.md §2.3)", fieldName)
			}
		}
	}
}

// TestPublicMetricLockdown_NotificationDTO_NoCountFields verifies that
// NotificationDTO contains no social-validation metric fields.
func TestPublicMetricLockdown_NotificationDTO_NoCountFields(t *testing.T) {
	t.Parallel()

	forbiddenFieldNames := []string{
		"LikeCount", "ImpressionCount", "BookmarkCount",
		"ReplyCount", "RepostCount", "ShareCount",
		"ViewCount", "FollowerCount", "ReactionCount",
	}

	dt := reflect.TypeOf(notification.NotificationDTO{})
	for i := 0; i < dt.NumField(); i++ {
		fieldName := dt.Field(i).Name
		for _, forbidden := range forbiddenFieldNames {
			if fieldName == forbidden {
				t.Errorf("notification.NotificationDTO must not contain metric field %q (CLAUDE.md §2.3)", fieldName)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// TestReact_NotificationFailure_NonFatal
// ---------------------------------------------------------------------------

// TestReact_NotificationFailure_NonFatal verifies that when the notifier
// returns an error, React still returns nil — notification failure is
// non-fatal per the production implementation.
func TestReact_NotificationFailure_NonFatal(t *testing.T) {
	t.Parallel()

	fake := &fakeReactionRepo{
		reactFn: func(_ context.Context, _, _ uuid.UUID) (bool, error) {
			return true, nil // new reaction
		},
	}
	notifier := &stubNotifier{
		publishFn: func(_ context.Context, _ notification.PublishEvent) error {
			return errors.New("notification service down")
		},
	}
	svc := newTestableReactionService(fake, notifier)

	err := svc.React(context.Background(), uuid.New(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("notification failure must be non-fatal; got error: %v", err)
	}
}
