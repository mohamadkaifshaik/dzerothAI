//go:build !integration

package title

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/notification"
)

// ---------------------------------------------------------------------------
// Fake implementations
// ---------------------------------------------------------------------------

// fakeTitleNotificationRepo implements titleNotificationRepo for unit tests.
type fakeTitleNotificationRepo struct {
	mu sync.Mutex

	unlockPending []PendingTitleNotification
	gracePending  []PendingTitleNotification

	unlockSentIDs []uuid.UUID
	graceSentIDs  []uuid.UUID

	unlockQueryErr error
	graceQueryErr  error
	markUnlockErr  error
	markGraceErr   error
}

func (f *fakeTitleNotificationRepo) GetPendingUnlockNotifications(_ context.Context, limit int) ([]PendingTitleNotification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.unlockQueryErr != nil {
		return nil, f.unlockQueryErr
	}
	result := f.unlockPending
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (f *fakeTitleNotificationRepo) GetPendingGraceNotifications(_ context.Context, limit int) ([]PendingTitleNotification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.graceQueryErr != nil {
		return nil, f.graceQueryErr
	}
	result := f.gracePending
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (f *fakeTitleNotificationRepo) MarkUnlockNotificationSent(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.markUnlockErr != nil {
		return f.markUnlockErr
	}
	f.unlockSentIDs = append(f.unlockSentIDs, id)
	return nil
}

func (f *fakeTitleNotificationRepo) MarkGraceNotificationSent(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.markGraceErr != nil {
		return f.markGraceErr
	}
	f.graceSentIDs = append(f.graceSentIDs, id)
	return nil
}

// fakeTitleNotificationPublisher implements titleNotificationPublisher for unit tests.
type fakeTitleNotificationPublisher struct {
	mu sync.Mutex

	published  []notification.PublishEvent
	publishErr error
}

func (f *fakeTitleNotificationPublisher) Publish(_ context.Context, event notification.PublishEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.publishErr != nil {
		return f.publishErr
	}
	f.published = append(f.published, event)
	return nil
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func newTestNotificationWorker(repo titleNotificationRepo, pub titleNotificationPublisher) *TitleNotificationWorker {
	cfg := NotificationWorkerConfig{Interval: time.Minute, BatchSize: 100}
	return NewTitleNotificationWorker(repo, pub, cfg, zap.NewNop())
}

func newPending(slug string) PendingTitleNotification {
	return PendingTitleNotification{
		UserTitleID: uuid.New(),
		UserID:      uuid.New(),
		Slug:        slug,
		DisplayName: slug + "_display",
	}
}

// ---------------------------------------------------------------------------
// Test 1: Pending unlock notification is fetched
// ---------------------------------------------------------------------------

func TestNotificationWorker_UnlockPendingFetched(t *testing.T) {
	p := newPending("centurion")
	repo := &fakeTitleNotificationRepo{unlockPending: []PendingTitleNotification{p}}
	pub := &fakeTitleNotificationPublisher{}

	w := newTestNotificationWorker(repo, pub)
	w.RunOnce(context.Background())

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.published) == 0 {
		t.Fatal("expected at least one published event for pending unlock notification")
	}
}

// ---------------------------------------------------------------------------
// Test 2: Pending unlock notification is published with correct event type
// ---------------------------------------------------------------------------

func TestNotificationWorker_UnlockPublishedWithCorrectEvent(t *testing.T) {
	p := newPending("centurion")
	repo := &fakeTitleNotificationRepo{unlockPending: []PendingTitleNotification{p}}
	pub := &fakeTitleNotificationPublisher{}

	w := newTestNotificationWorker(repo, pub)
	w.RunOnce(context.Background())

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(pub.published))
	}
	if pub.published[0].Event != notification.EventTitleUnlocked {
		t.Errorf("event = %v, want %v", pub.published[0].Event, notification.EventTitleUnlocked)
	}
	if pub.published[0].RecipientID != p.UserID {
		t.Errorf("recipient = %v, want %v", pub.published[0].RecipientID, p.UserID)
	}
}

// ---------------------------------------------------------------------------
// Test 3: Successful unlock publication marks the flag sent
// ---------------------------------------------------------------------------

func TestNotificationWorker_SuccessfulUnlockMarksFlag(t *testing.T) {
	p := newPending("centurion")
	repo := &fakeTitleNotificationRepo{unlockPending: []PendingTitleNotification{p}}
	pub := &fakeTitleNotificationPublisher{}

	w := newTestNotificationWorker(repo, pub)
	w.RunOnce(context.Background())

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.unlockSentIDs) != 1 {
		t.Fatalf("expected MarkUnlockNotificationSent called once, got %d", len(repo.unlockSentIDs))
	}
	if repo.unlockSentIDs[0] != p.UserTitleID {
		t.Errorf("marked ID = %v, want %v", repo.unlockSentIDs[0], p.UserTitleID)
	}
}

// ---------------------------------------------------------------------------
// Test 4: Pending grace notification is fetched
// ---------------------------------------------------------------------------

func TestNotificationWorker_GracePendingFetched(t *testing.T) {
	p := newPending("trendsetter")
	repo := &fakeTitleNotificationRepo{gracePending: []PendingTitleNotification{p}}
	pub := &fakeTitleNotificationPublisher{}

	w := newTestNotificationWorker(repo, pub)
	w.RunOnce(context.Background())

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.published) == 0 {
		t.Fatal("expected at least one published event for pending grace notification")
	}
}

// ---------------------------------------------------------------------------
// Test 5: Pending grace notification is published with correct event type
// ---------------------------------------------------------------------------

func TestNotificationWorker_GracePublishedWithCorrectEvent(t *testing.T) {
	p := newPending("trendsetter")
	repo := &fakeTitleNotificationRepo{gracePending: []PendingTitleNotification{p}}
	pub := &fakeTitleNotificationPublisher{}

	w := newTestNotificationWorker(repo, pub)
	w.RunOnce(context.Background())

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(pub.published))
	}
	if pub.published[0].Event != notification.EventTitleGracePeriod {
		t.Errorf("event = %v, want %v", pub.published[0].Event, notification.EventTitleGracePeriod)
	}
	if pub.published[0].RecipientID != p.UserID {
		t.Errorf("recipient = %v, want %v", pub.published[0].RecipientID, p.UserID)
	}
}

// ---------------------------------------------------------------------------
// Test 6: Successful grace publication marks the flag sent
// ---------------------------------------------------------------------------

func TestNotificationWorker_SuccessfulGraceMarksFlag(t *testing.T) {
	p := newPending("trendsetter")
	repo := &fakeTitleNotificationRepo{gracePending: []PendingTitleNotification{p}}
	pub := &fakeTitleNotificationPublisher{}

	w := newTestNotificationWorker(repo, pub)
	w.RunOnce(context.Background())

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.graceSentIDs) != 1 {
		t.Fatalf("expected MarkGraceNotificationSent called once, got %d", len(repo.graceSentIDs))
	}
	if repo.graceSentIDs[0] != p.UserTitleID {
		t.Errorf("marked ID = %v, want %v", repo.graceSentIDs[0], p.UserTitleID)
	}
}

// ---------------------------------------------------------------------------
// Test 7: Publish failure does NOT mark the flag sent
// ---------------------------------------------------------------------------

func TestNotificationWorker_PublishFailureDoesNotMarkFlag(t *testing.T) {
	pUnlock := newPending("centurion")
	pGrace := newPending("trendsetter")

	repo := &fakeTitleNotificationRepo{
		unlockPending: []PendingTitleNotification{pUnlock},
		gracePending:  []PendingTitleNotification{pGrace},
	}
	pub := &fakeTitleNotificationPublisher{
		publishErr: errors.New("publish failed"),
	}

	w := newTestNotificationWorker(repo, pub)
	w.RunOnce(context.Background())

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.unlockSentIDs) != 0 {
		t.Errorf("MarkUnlockNotificationSent must not be called on publish failure, got %d calls", len(repo.unlockSentIDs))
	}
	if len(repo.graceSentIDs) != 0 {
		t.Errorf("MarkGraceNotificationSent must not be called on publish failure, got %d calls", len(repo.graceSentIDs))
	}
}

// ---------------------------------------------------------------------------
// Test 8: Empty result terminates batch processing without error
// ---------------------------------------------------------------------------

func TestNotificationWorker_EmptyResultNoError(t *testing.T) {
	repo := &fakeTitleNotificationRepo{} // no pending rows
	pub := &fakeTitleNotificationPublisher{}

	w := newTestNotificationWorker(repo, pub)
	// RunOnce must complete without panic or blocking.
	done := make(chan struct{})
	go func() {
		w.RunOnce(context.Background())
		close(done)
	}()
	select {
	case <-done:
		// pass
	case <-time.After(2 * time.Second):
		t.Fatal("RunOnce did not return on empty result set")
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.published) != 0 {
		t.Errorf("expected 0 published events for empty result, got %d", len(pub.published))
	}
}

// ---------------------------------------------------------------------------
// Test 9: Multiple records are processed in batches
// ---------------------------------------------------------------------------

func TestNotificationWorker_MultipleRecordsProcessed(t *testing.T) {
	const count = 5
	unlockPending := make([]PendingTitleNotification, count)
	for i := range unlockPending {
		unlockPending[i] = newPending("centurion")
	}
	gracePending := make([]PendingTitleNotification, count)
	for i := range gracePending {
		gracePending[i] = newPending("trendsetter")
	}

	repo := &fakeTitleNotificationRepo{
		unlockPending: unlockPending,
		gracePending:  gracePending,
	}
	pub := &fakeTitleNotificationPublisher{}

	w := newTestNotificationWorker(repo, pub)
	w.RunOnce(context.Background())

	pub.mu.Lock()
	publishedCount := len(pub.published)
	pub.mu.Unlock()
	if publishedCount != 2*count {
		t.Errorf("expected %d published events, got %d", 2*count, publishedCount)
	}

	repo.mu.Lock()
	unlockSent := len(repo.unlockSentIDs)
	graceSent := len(repo.graceSentIDs)
	repo.mu.Unlock()
	if unlockSent != count {
		t.Errorf("expected %d unlock flags marked, got %d", count, unlockSent)
	}
	if graceSent != count {
		t.Errorf("expected %d grace flags marked, got %d", count, graceSent)
	}
}

// ---------------------------------------------------------------------------
// Test 10: Context cancellation stops the worker cleanly (Run returns)
// ---------------------------------------------------------------------------

func TestNotificationWorker_ContextCancellationStopsWorker(t *testing.T) {
	repo := &fakeTitleNotificationRepo{}
	pub := &fakeTitleNotificationPublisher{}

	// Use a very long interval so the ticker never fires — cancellation is the
	// only way Run returns.
	cfg := NotificationWorkerConfig{Interval: 24 * time.Hour, BatchSize: 100}
	w := NewTitleNotificationWorker(repo, pub, cfg, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	// Give the goroutine time to enter the select, then cancel.
	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// pass — worker exited cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}
