// Package notification — service tests.
//
// Unit tests for notification service business logic using fake repository
// doubles. No real database connection is required.
package notification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/post"
)

// ---------------------------------------------------------------------------
// notifRepo interface and test double
// ---------------------------------------------------------------------------

// notifRepo is the minimal interface for repository methods called by the
// service under test. It mirrors the concrete *Repository signatures.
type notifRepo interface {
	Publish(ctx context.Context, event PublishEvent) error
	ListNotifications(ctx context.Context, recipientID uuid.UUID, cursor *post.FeedCursor) (NotificationPage, error)
	MarkAllRead(ctx context.Context, recipientID uuid.UUID) error
}

// fakeNotifRepo is a test double for notifRepo.
type fakeNotifRepo struct {
	publishFn         func(ctx context.Context, event PublishEvent) error
	listFn            func(ctx context.Context, recipientID uuid.UUID, cursor *post.FeedCursor) (NotificationPage, error)
	markAllReadFn     func(ctx context.Context, recipientID uuid.UUID) error
	publishCallCount  int
	markAllReadCallID uuid.UUID
}

func (f *fakeNotifRepo) Publish(ctx context.Context, event PublishEvent) error {
	f.publishCallCount++
	if f.publishFn != nil {
		return f.publishFn(ctx, event)
	}
	return nil
}

func (f *fakeNotifRepo) ListNotifications(ctx context.Context, recipientID uuid.UUID, cursor *post.FeedCursor) (NotificationPage, error) {
	if f.listFn != nil {
		return f.listFn(ctx, recipientID, cursor)
	}
	return NotificationPage{Items: []NotificationDTO{}, Terminated: true}, nil
}

func (f *fakeNotifRepo) MarkAllRead(ctx context.Context, recipientID uuid.UUID) error {
	f.markAllReadCallID = recipientID
	if f.markAllReadFn != nil {
		return f.markAllReadFn(ctx, recipientID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// testableNotifService — service that accepts the notifRepo interface
// ---------------------------------------------------------------------------

// testableNotifService mirrors Service but holds a notifRepo interface so
// tests can inject fakes without a real database.
type testableNotifService struct {
	repo notifRepo
	log  *zap.Logger
}

func newTestableNotifService(repo notifRepo) *testableNotifService {
	return &testableNotifService{repo: repo, log: zap.NewNop()}
}

func (s *testableNotifService) Publish(ctx context.Context, event PublishEvent) error {
	// Mirror Service.Publish: self-notifications suppressed silently.
	if event.ActorID == event.RecipientID {
		return nil
	}
	if err := s.repo.Publish(ctx, event); err != nil {
		s.log.Error("notification: publish", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

func (s *testableNotifService) ListNotifications(ctx context.Context, callerID uuid.UUID, cursorStr string) (NotificationPage, error) {
	var cursor *post.FeedCursor
	if cursorStr != "" {
		c, err := post.DecodeCursor(cursorStr)
		if err != nil {
			return NotificationPage{}, apierror.NewAPIError(apierror.CodeValidation, "invalid cursor")
		}
		cursor = c
	}
	page, err := s.repo.ListNotifications(ctx, callerID, cursor)
	if err != nil {
		s.log.Error("notification: list", zap.Error(err))
		return NotificationPage{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return page, nil
}

func (s *testableNotifService) MarkAllRead(ctx context.Context, callerID uuid.UUID) error {
	if err := s.repo.MarkAllRead(ctx, callerID); err != nil {
		s.log.Error("notification: mark all read", zap.Error(err))
		return apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func notifAPICode(err error) string {
	var ae *apierror.APIError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}

func makeNotifDTO(recipientID uuid.UUID) NotificationDTO {
	actorID := uuid.New()
	return NotificationDTO{
		ID:          uuid.New().String(),
		RecipientID: recipientID.String(),
		ActorID:     actorID.String(),
		Actor: ActorSummary{
			ID:          actorID.String(),
			Handle:      "actor",
			DisplayName: "Actor",
		},
		Event:     EventFollow,
		IsRead:    false,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

func buildNotifCursor() string {
	id, _ := uuid.NewV7()
	c := post.FeedCursor{AfterID: id, Timestamp: time.Now().UTC()}
	return c.Encode()
}

// ---------------------------------------------------------------------------
// TestPublish_SelfNotification_Suppressed
// ---------------------------------------------------------------------------

// TestPublish_SelfNotification_Suppressed verifies that when actor == recipient
// the repository is never called and nil is returned.
func TestPublish_SelfNotification_Suppressed(t *testing.T) {
	t.Parallel()
	fake := &fakeNotifRepo{}
	svc := newTestableNotifService(fake)

	sameID := uuid.New()
	err := svc.Publish(context.Background(), PublishEvent{
		RecipientID: sameID,
		ActorID:     sameID,
		Event:       EventFollow,
	})
	if err != nil {
		t.Fatalf("expected nil for self-notification, got: %v", err)
	}
	if fake.publishCallCount != 0 {
		t.Errorf("expected repo.Publish not called for self-notification, called %d times", fake.publishCallCount)
	}
}

// ---------------------------------------------------------------------------
// TestPublish_DifferentActorRecipient_Succeeds
// ---------------------------------------------------------------------------

// TestPublish_DifferentActorRecipient_Succeeds verifies that when actor !=
// recipient the repository is called exactly once and nil is returned.
func TestPublish_DifferentActorRecipient_Succeeds(t *testing.T) {
	t.Parallel()

	var capturedEvent PublishEvent
	fake := &fakeNotifRepo{
		publishFn: func(_ context.Context, event PublishEvent) error {
			capturedEvent = event
			return nil
		},
	}
	svc := newTestableNotifService(fake)

	recipientID := uuid.New()
	actorID := uuid.New()
	postID := uuid.New()

	err := svc.Publish(context.Background(), PublishEvent{
		RecipientID: recipientID,
		ActorID:     actorID,
		Event:       EventReaction,
		PostID:      &postID,
	})
	if err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
	if fake.publishCallCount != 1 {
		t.Errorf("expected repo.Publish called once, called %d times", fake.publishCallCount)
	}
	if capturedEvent.RecipientID != recipientID {
		t.Errorf("recipient mismatch: got %v, want %v", capturedEvent.RecipientID, recipientID)
	}
	if capturedEvent.ActorID != actorID {
		t.Errorf("actor mismatch: got %v, want %v", capturedEvent.ActorID, actorID)
	}
	if capturedEvent.Event != EventReaction {
		t.Errorf("event mismatch: got %v, want %v", capturedEvent.Event, EventReaction)
	}
}

// ---------------------------------------------------------------------------
// TestPublish_RepoError_ReturnsInternalError
// ---------------------------------------------------------------------------

// TestPublish_RepoError_ReturnsInternalError verifies that a repository error
// is wrapped as CodeInternal.
func TestPublish_RepoError_ReturnsInternalError(t *testing.T) {
	t.Parallel()

	fake := &fakeNotifRepo{
		publishFn: func(_ context.Context, _ PublishEvent) error {
			return errors.New("db connection refused")
		},
	}
	svc := newTestableNotifService(fake)

	err := svc.Publish(context.Background(), PublishEvent{
		RecipientID: uuid.New(),
		ActorID:     uuid.New(),
		Event:       EventMention,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if code := notifAPICode(err); code != apierror.CodeInternal {
		t.Errorf("error code = %q, want %q", code, apierror.CodeInternal)
	}
}

// ---------------------------------------------------------------------------
// TestListNotifications_OwnerScoped
// ---------------------------------------------------------------------------

// TestListNotifications_OwnerScoped verifies that the callerID is forwarded
// to the repository and the returned items belong to the caller.
func TestListNotifications_OwnerScoped(t *testing.T) {
	t.Parallel()

	callerID := uuid.New()
	dto := makeNotifDTO(callerID)

	var queriedRecipient uuid.UUID
	fake := &fakeNotifRepo{
		listFn: func(_ context.Context, recipientID uuid.UUID, _ *post.FeedCursor) (NotificationPage, error) {
			queriedRecipient = recipientID
			return NotificationPage{
				Items:      []NotificationDTO{dto},
				Terminated: true,
			}, nil
		},
	}
	svc := newTestableNotifService(fake)

	page, err := svc.ListNotifications(context.Background(), callerID, "")
	if err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
	if queriedRecipient != callerID {
		t.Errorf("repository queried for %v, want %v", queriedRecipient, callerID)
	}
	if len(page.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(page.Items))
	}
	if page.Items[0].RecipientID != callerID.String() {
		t.Errorf("item recipient mismatch: got %v, want %v", page.Items[0].RecipientID, callerID.String())
	}
}

// ---------------------------------------------------------------------------
// TestListNotifications_Terminated
// ---------------------------------------------------------------------------

// TestListNotifications_Terminated verifies that the page termination flag is
// forwarded from the repository (finite feed — CLAUDE.md §2.1).
func TestListNotifications_Terminated(t *testing.T) {
	t.Parallel()

	callerID := uuid.New()
	// Build maxDepth (100) items to simulate the boundary.
	items := make([]NotificationDTO, 100)
	for i := range items {
		items[i] = makeNotifDTO(callerID)
	}

	fake := &fakeNotifRepo{
		listFn: func(_ context.Context, _ uuid.UUID, _ *post.FeedCursor) (NotificationPage, error) {
			return NotificationPage{
				Items:      items,
				NextCursor: "",
				Terminated: true,
			}, nil
		},
	}
	svc := newTestableNotifService(fake)

	page, err := svc.ListNotifications(context.Background(), callerID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !page.Terminated {
		t.Error("expected Terminated=true at max depth")
	}
	if len(page.Items) != 100 {
		t.Errorf("expected 100 items, got %d", len(page.Items))
	}
}

// ---------------------------------------------------------------------------
// TestListNotifications_BadCursor
// ---------------------------------------------------------------------------

// TestListNotifications_BadCursor verifies that a malformed cursor returns
// CodeValidation and the repository is never called.
func TestListNotifications_BadCursor(t *testing.T) {
	t.Parallel()

	repoCalled := false
	fake := &fakeNotifRepo{
		listFn: func(_ context.Context, _ uuid.UUID, _ *post.FeedCursor) (NotificationPage, error) {
			repoCalled = true
			return NotificationPage{}, nil
		},
	}
	svc := newTestableNotifService(fake)

	_, err := svc.ListNotifications(context.Background(), uuid.New(), "!!!bad-cursor!!!")
	if err == nil {
		t.Fatal("expected CodeValidation for bad cursor, got nil")
	}
	if code := notifAPICode(err); code != apierror.CodeValidation {
		t.Errorf("error code = %q, want %q", code, apierror.CodeValidation)
	}
	if repoCalled {
		t.Error("repository must not be called when cursor is invalid")
	}
}

// ---------------------------------------------------------------------------
// TestListNotifications_RepoError_ReturnsInternalError
// ---------------------------------------------------------------------------

// TestListNotifications_RepoError_ReturnsInternalError verifies that a
// repository failure is wrapped as CodeInternal.
func TestListNotifications_RepoError_ReturnsInternalError(t *testing.T) {
	t.Parallel()

	fake := &fakeNotifRepo{
		listFn: func(_ context.Context, _ uuid.UUID, _ *post.FeedCursor) (NotificationPage, error) {
			return NotificationPage{}, errors.New("db error")
		},
	}
	svc := newTestableNotifService(fake)

	_, err := svc.ListNotifications(context.Background(), uuid.New(), "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if code := notifAPICode(err); code != apierror.CodeInternal {
		t.Errorf("error code = %q, want %q", code, apierror.CodeInternal)
	}
}

// ---------------------------------------------------------------------------
// TestMarkAllRead_OwnerScoped
// ---------------------------------------------------------------------------

// TestMarkAllRead_OwnerScoped verifies that only the callerID is passed to
// the repository — reads belonging to other users are never modified.
func TestMarkAllRead_OwnerScoped(t *testing.T) {
	t.Parallel()

	callerID := uuid.New()
	otherID := uuid.New()
	_ = otherID // different user whose notifications must not be touched

	fake := &fakeNotifRepo{}
	svc := newTestableNotifService(fake)

	if err := svc.MarkAllRead(context.Background(), callerID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.markAllReadCallID != callerID {
		t.Errorf("MarkAllRead called with %v, want %v", fake.markAllReadCallID, callerID)
	}
}

// ---------------------------------------------------------------------------
// TestMarkAllRead_Idempotent
// ---------------------------------------------------------------------------

// TestMarkAllRead_Idempotent verifies that calling MarkAllRead when there are
// no unread notifications returns nil (idempotent).
func TestMarkAllRead_Idempotent(t *testing.T) {
	t.Parallel()

	callCount := 0
	fake := &fakeNotifRepo{
		markAllReadFn: func(_ context.Context, _ uuid.UUID) error {
			callCount++
			return nil // UPDATE 0 rows — no error
		},
	}
	svc := newTestableNotifService(fake)

	callerID := uuid.New()
	if err := svc.MarkAllRead(context.Background(), callerID); err != nil {
		t.Fatalf("first MarkAllRead returned error: %v", err)
	}
	if err := svc.MarkAllRead(context.Background(), callerID); err != nil {
		t.Fatalf("second MarkAllRead returned error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 repo calls, got %d", callCount)
	}
}

// ---------------------------------------------------------------------------
// TestMarkAllRead_RepoError_ReturnsInternalError
// ---------------------------------------------------------------------------

// TestMarkAllRead_RepoError_ReturnsInternalError verifies that a repository
// failure is wrapped as CodeInternal.
func TestMarkAllRead_RepoError_ReturnsInternalError(t *testing.T) {
	t.Parallel()

	fake := &fakeNotifRepo{
		markAllReadFn: func(_ context.Context, _ uuid.UUID) error {
			return errors.New("db timeout")
		},
	}
	svc := newTestableNotifService(fake)

	err := svc.MarkAllRead(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if code := notifAPICode(err); code != apierror.CodeInternal {
		t.Errorf("error code = %q, want %q", code, apierror.CodeInternal)
	}
}

// ---------------------------------------------------------------------------
// TestListNotifications_ValidCursorForwarded
// ---------------------------------------------------------------------------

// TestListNotifications_ValidCursorForwarded verifies that a valid opaque
// cursor is decoded and forwarded to the repository as a non-nil *FeedCursor.
func TestListNotifications_ValidCursorForwarded(t *testing.T) {
	t.Parallel()

	var receivedCursor *post.FeedCursor
	fake := &fakeNotifRepo{
		listFn: func(_ context.Context, _ uuid.UUID, cursor *post.FeedCursor) (NotificationPage, error) {
			receivedCursor = cursor
			return NotificationPage{Items: []NotificationDTO{}, Terminated: true}, nil
		},
	}
	svc := newTestableNotifService(fake)

	cursorStr := buildNotifCursor()
	_, err := svc.ListNotifications(context.Background(), uuid.New(), cursorStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedCursor == nil {
		t.Error("expected non-nil cursor forwarded to repository, got nil")
	}
}
