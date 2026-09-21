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
)

// ---------------------------------------------------------------------------
// Fake repository
// ---------------------------------------------------------------------------

// fakeWorkerRepo is a controllable fake that satisfies workerRepository.
type fakeWorkerRepo struct {
	mu sync.Mutex

	// userIDs is the ordered list of IDs returned by GetUserIDsBatch.
	userIDs []uuid.UUID

	// titles maps userID → []UserTitleEntry returned by GetUserTitles.
	titles map[uuid.UUID][]UserTitleEntry

	// enterGracePeriodCalls records (userTitleID, endsAt) calls.
	enterGracePeriodCalls []enterGraceCall
	enterGraceErr         error

	// restoreGraceCalls records userTitleIDs passed to RestoreGracePeriodTitle.
	restoreGraceCalls []uuid.UUID
	restoreGraceErr   error

	// revokeCalls records arguments passed to RevokeUserTitleTx.
	revokeCalls []revokeCall
	revokeErr   error

	// batchErr, if non-nil, is returned from GetUserIDsBatch.
	batchErr error

	// titlesErr, if non-nil, is returned from GetUserTitles.
	titlesErr error
}

type enterGraceCall struct {
	userTitleID uuid.UUID
	endsAt      time.Time
}

type revokeCall struct {
	userTitleID uuid.UUID
	userID      uuid.UUID
	revokedAt   time.Time
}

func (f *fakeWorkerRepo) GetUserIDsBatch(_ context.Context, afterID uuid.UUID, limit int) ([]uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.batchErr != nil {
		return nil, f.batchErr
	}
	// Return IDs > afterID, up to limit (simulate cursor pagination).
	var result []uuid.UUID
	for _, id := range f.userIDs {
		if id.String() > afterID.String() {
			result = append(result, id)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (f *fakeWorkerRepo) GetUserTitles(_ context.Context, userID uuid.UUID) ([]UserTitleEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.titlesErr != nil {
		return nil, f.titlesErr
	}
	return f.titles[userID], nil
}

func (f *fakeWorkerRepo) EnterGracePeriod(_ context.Context, userTitleID uuid.UUID, endsAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enterGracePeriodCalls = append(f.enterGracePeriodCalls, enterGraceCall{userTitleID, endsAt})
	return f.enterGraceErr
}

func (f *fakeWorkerRepo) RestoreGracePeriodTitle(_ context.Context, userTitleID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restoreGraceCalls = append(f.restoreGraceCalls, userTitleID)
	return f.restoreGraceErr
}

func (f *fakeWorkerRepo) RevokeUserTitleTx(_ context.Context, userTitleID, userID uuid.UUID, revokedAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revokeCalls = append(f.revokeCalls, revokeCall{userTitleID, userID, revokedAt})
	return f.revokeErr
}

// ---------------------------------------------------------------------------
// Fake engine
// ---------------------------------------------------------------------------

// fakeWorkerEngine is a controllable fake that satisfies workerEngine.
type fakeWorkerEngine struct {
	mu sync.Mutex

	// evaluateResults is the list returned by EvaluateAndUnlock for each call.
	evaluateResults []QualificationResult
	evaluateErr     error
	evaluateCalls   []uuid.UUID

	// qualifyResults maps slug → QualificationResult for Qualify calls.
	qualifyResults map[string]QualificationResult
	qualifyErr     map[string]error
	qualifyCalls   []qualifyCall
}

type qualifyCall struct {
	userID uuid.UUID
	slug   string
}

func (f *fakeWorkerEngine) EvaluateAndUnlock(_ context.Context, userID uuid.UUID) ([]QualificationResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.evaluateCalls = append(f.evaluateCalls, userID)
	return f.evaluateResults, f.evaluateErr
}

func (f *fakeWorkerEngine) Qualify(_ context.Context, userID uuid.UUID, slug string) (QualificationResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.qualifyCalls = append(f.qualifyCalls, qualifyCall{userID, slug})
	if f.qualifyErr != nil {
		if err, ok := f.qualifyErr[slug]; ok {
			return QualificationResult{}, err
		}
	}
	if f.qualifyResults != nil {
		if r, ok := f.qualifyResults[slug]; ok {
			return r, nil
		}
	}
	return QualificationResult{Slug: slug, Qualified: false}, nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newTestWorker(eng workerEngine, repo workerRepository) *TitleQualificationWorker {
	return newTitleQualificationWorkerFromIfaces(eng, repo, WorkerConfig{Interval: time.Minute}, zap.NewNop())
}

// makeEntry builds a UserTitleEntry for test use.
func makeEntry(userID uuid.UUID, status TitleStatus, slug string, isRevocable bool, graceEndsAt *time.Time) UserTitleEntry {
	return UserTitleEntry{
		UserTitle: UserTitle{
			ID:                uuid.New(),
			UserID:            userID,
			TitleDefinitionID: uuid.New(),
			Status:            status,
			GracePeriodEndsAt: graceEndsAt,
			UnlockedAt:        time.Now().UTC().Add(-time.Hour),
			CreatedAt:         time.Now().UTC().Add(-time.Hour),
		},
		Slug:        slug,
		DisplayName: slug,
		IsRevocable: isRevocable,
	}
}

// ---------------------------------------------------------------------------
// 1. New qualification → EvaluateAndUnlock called (active title created by engine)
// ---------------------------------------------------------------------------

func TestWorker_NewQualification_EvaluateAndUnlockCalled(t *testing.T) {
	userID := uuid.New()
	eng := &fakeWorkerEngine{
		evaluateResults: []QualificationResult{
			{Slug: "centurion", Qualified: true, UserTitle: &UserTitle{ID: uuid.New()}},
		},
	}
	repo := &fakeWorkerRepo{
		userIDs: []uuid.UUID{userID},
		titles:  map[uuid.UUID][]UserTitleEntry{userID: {}},
	}

	w := newTestWorker(eng, repo)
	w.RunOnce(context.Background())

	eng.mu.Lock()
	defer eng.mu.Unlock()
	if len(eng.evaluateCalls) != 1 || eng.evaluateCalls[0] != userID {
		t.Errorf("EvaluateAndUnlock not called for user; calls=%v", eng.evaluateCalls)
	}
}

// ---------------------------------------------------------------------------
// 2. Duplicate evaluation → idempotent (EvaluateAndUnlock called twice, no panic)
// ---------------------------------------------------------------------------

func TestWorker_DuplicateEvaluation_Idempotent(t *testing.T) {
	userID := uuid.New()
	eng := &fakeWorkerEngine{
		evaluateResults: []QualificationResult{
			{Slug: "centurion", Qualified: true, UserTitle: &UserTitle{ID: uuid.New()}},
		},
	}
	repo := &fakeWorkerRepo{
		userIDs: []uuid.UUID{userID},
		titles:  map[uuid.UUID][]UserTitleEntry{userID: {}},
	}

	w := newTestWorker(eng, repo)
	w.RunOnce(context.Background())
	w.RunOnce(context.Background())

	eng.mu.Lock()
	defer eng.mu.Unlock()
	if len(eng.evaluateCalls) != 2 {
		t.Errorf("expected 2 EvaluateAndUnlock calls (two passes), got %d", len(eng.evaluateCalls))
	}
	// No repo methods called for transitions since titles map is empty.
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.enterGracePeriodCalls) != 0 {
		t.Errorf("expected 0 EnterGracePeriod calls, got %d", len(repo.enterGracePeriodCalls))
	}
}

// ---------------------------------------------------------------------------
// 3. Permanent title (is_revocable=false) with qualification=false → NO state change
// ---------------------------------------------------------------------------

func TestWorker_PermanentTitle_NotRevocable_NoStateChange(t *testing.T) {
	userID := uuid.New()
	entry := makeEntry(userID, StatusActive, "founding_member", false, nil)

	eng := &fakeWorkerEngine{
		qualifyResults: map[string]QualificationResult{
			"founding_member": {Slug: "founding_member", Qualified: false},
		},
	}
	repo := &fakeWorkerRepo{
		userIDs: []uuid.UUID{userID},
		titles:  map[uuid.UUID][]UserTitleEntry{userID: {entry}},
	}

	w := newTestWorker(eng, repo)
	w.RunOnce(context.Background())

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.enterGracePeriodCalls) != 0 {
		t.Errorf("permanent title must not enter grace period; calls=%v", repo.enterGracePeriodCalls)
	}
	if len(repo.revokeCalls) != 0 {
		t.Errorf("permanent title must not be revoked; calls=%v", repo.revokeCalls)
	}
}

// ---------------------------------------------------------------------------
// 4. Active revocable title + unqualified → EnterGracePeriod called
// ---------------------------------------------------------------------------

func TestWorker_ActiveRevocable_Unqualified_EntersGrace(t *testing.T) {
	userID := uuid.New()
	entry := makeEntry(userID, StatusActive, "trendsetter", true, nil)

	eng := &fakeWorkerEngine{
		qualifyResults: map[string]QualificationResult{
			"trendsetter": {Slug: "trendsetter", Qualified: false},
		},
	}
	repo := &fakeWorkerRepo{
		userIDs: []uuid.UUID{userID},
		titles:  map[uuid.UUID][]UserTitleEntry{userID: {entry}},
	}

	w := newTestWorker(eng, repo)
	w.RunOnce(context.Background())

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.enterGracePeriodCalls) != 1 {
		t.Fatalf("expected 1 EnterGracePeriod call, got %d", len(repo.enterGracePeriodCalls))
	}
	if repo.enterGracePeriodCalls[0].userTitleID != entry.ID {
		t.Errorf("EnterGracePeriod called with wrong ID: %v", repo.enterGracePeriodCalls[0].userTitleID)
	}
	// Grace period should be ~48h from now.
	expectedEndsAt := time.Now().UTC().Add(gracePeriodDuration)
	delta := repo.enterGracePeriodCalls[0].endsAt.Sub(expectedEndsAt)
	if delta < -5*time.Second || delta > 5*time.Second {
		t.Errorf("EnterGracePeriod endsAt %v not within 5s of expected %v", repo.enterGracePeriodCalls[0].endsAt, expectedEndsAt)
	}
}

// ---------------------------------------------------------------------------
// 5. Grace period title + qualified → RestoreGracePeriodTitle called
// ---------------------------------------------------------------------------

func TestWorker_GracePeriod_Qualified_Restored(t *testing.T) {
	userID := uuid.New()
	endsAt := time.Now().UTC().Add(24 * time.Hour)
	entry := makeEntry(userID, StatusGracePeriod, "trendsetter", true, &endsAt)

	eng := &fakeWorkerEngine{
		qualifyResults: map[string]QualificationResult{
			"trendsetter": {Slug: "trendsetter", Qualified: true, UserTitle: &UserTitle{ID: entry.ID}},
		},
	}
	repo := &fakeWorkerRepo{
		userIDs: []uuid.UUID{userID},
		titles:  map[uuid.UUID][]UserTitleEntry{userID: {entry}},
	}

	w := newTestWorker(eng, repo)
	w.RunOnce(context.Background())

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.restoreGraceCalls) != 1 {
		t.Fatalf("expected 1 RestoreGracePeriodTitle call, got %d", len(repo.restoreGraceCalls))
	}
	if repo.restoreGraceCalls[0] != entry.ID {
		t.Errorf("RestoreGracePeriodTitle called with wrong ID: %v", repo.restoreGraceCalls[0])
	}
	if len(repo.revokeCalls) != 0 {
		t.Errorf("unexpected revoke calls: %v", repo.revokeCalls)
	}
}

// ---------------------------------------------------------------------------
// 6. Grace period title + unqualified + before expiry → no change
// ---------------------------------------------------------------------------

func TestWorker_GracePeriod_Unqualified_BeforeExpiry_NoChange(t *testing.T) {
	userID := uuid.New()
	endsAt := time.Now().UTC().Add(1 * time.Hour) // still in window
	entry := makeEntry(userID, StatusGracePeriod, "trendsetter", true, &endsAt)

	eng := &fakeWorkerEngine{
		qualifyResults: map[string]QualificationResult{
			"trendsetter": {Slug: "trendsetter", Qualified: false},
		},
	}
	repo := &fakeWorkerRepo{
		userIDs: []uuid.UUID{userID},
		titles:  map[uuid.UUID][]UserTitleEntry{userID: {entry}},
	}

	w := newTestWorker(eng, repo)
	w.RunOnce(context.Background())

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.revokeCalls) != 0 {
		t.Errorf("must not revoke within grace window; calls=%v", repo.revokeCalls)
	}
	if len(repo.restoreGraceCalls) != 0 {
		t.Errorf("must not restore when unqualified; calls=%v", repo.restoreGraceCalls)
	}
	if len(repo.enterGracePeriodCalls) != 0 {
		t.Errorf("must not re-enter grace when already in grace; calls=%v", repo.enterGracePeriodCalls)
	}
}

// ---------------------------------------------------------------------------
// 7. Grace period title + unqualified + at/after expiry → RevokeUserTitleTx called
// ---------------------------------------------------------------------------

func TestWorker_GracePeriod_Unqualified_AfterExpiry_Revoked(t *testing.T) {
	userID := uuid.New()
	endsAt := time.Now().UTC().Add(-1 * time.Second) // just expired
	entry := makeEntry(userID, StatusGracePeriod, "trendsetter", true, &endsAt)

	eng := &fakeWorkerEngine{
		qualifyResults: map[string]QualificationResult{
			"trendsetter": {Slug: "trendsetter", Qualified: false},
		},
	}
	repo := &fakeWorkerRepo{
		userIDs: []uuid.UUID{userID},
		titles:  map[uuid.UUID][]UserTitleEntry{userID: {entry}},
	}

	w := newTestWorker(eng, repo)
	w.RunOnce(context.Background())

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.revokeCalls) != 1 {
		t.Fatalf("expected 1 RevokeUserTitleTx call, got %d", len(repo.revokeCalls))
	}
	call := repo.revokeCalls[0]
	if call.userTitleID != entry.ID {
		t.Errorf("revoke userTitleID = %v, want %v", call.userTitleID, entry.ID)
	}
	if call.userID != userID {
		t.Errorf("revoke userID = %v, want %v", call.userID, userID)
	}
}

// ---------------------------------------------------------------------------
// 8. Revoked title re-earned → EvaluateAndUnlock handles it (no direct revoke)
// ---------------------------------------------------------------------------

func TestWorker_RevokedTitle_Reearned_EvaluateAndUnlockHandles(t *testing.T) {
	userID := uuid.New()
	// No active/grace_period entries — GetUserTitles returns empty (revoked are excluded).
	eng := &fakeWorkerEngine{
		evaluateResults: []QualificationResult{
			{Slug: "centurion", Qualified: true, UserTitle: &UserTitle{ID: uuid.New()}},
		},
	}
	repo := &fakeWorkerRepo{
		userIDs: []uuid.UUID{userID},
		titles:  map[uuid.UUID][]UserTitleEntry{userID: {}},
	}

	w := newTestWorker(eng, repo)
	w.RunOnce(context.Background())

	eng.mu.Lock()
	defer eng.mu.Unlock()
	if len(eng.evaluateCalls) != 1 {
		t.Errorf("EvaluateAndUnlock must be called for re-earn path; calls=%v", eng.evaluateCalls)
	}
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.revokeCalls) != 0 {
		t.Errorf("must not call RevokeUserTitleTx for re-earned title; calls=%v", repo.revokeCalls)
	}
}

// ---------------------------------------------------------------------------
// 9. Final revocation → RevokeUserTitleTx called with correct user + title IDs
// ---------------------------------------------------------------------------

func TestWorker_FinalRevocation_CorrectArgs(t *testing.T) {
	userID := uuid.New()
	expired := time.Now().UTC().Add(-2 * time.Hour)
	entry := makeEntry(userID, StatusGracePeriod, "niche_guru_tech", true, &expired)

	eng := &fakeWorkerEngine{
		qualifyResults: map[string]QualificationResult{
			"niche_guru_tech": {Slug: "niche_guru_tech", Qualified: false},
		},
	}
	repo := &fakeWorkerRepo{
		userIDs: []uuid.UUID{userID},
		titles:  map[uuid.UUID][]UserTitleEntry{userID: {entry}},
	}

	w := newTestWorker(eng, repo)
	w.RunOnce(context.Background())

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.revokeCalls) != 1 {
		t.Fatalf("expected 1 revoke call, got %d", len(repo.revokeCalls))
	}
	c := repo.revokeCalls[0]
	if c.userTitleID != entry.ID {
		t.Errorf("userTitleID = %v, want %v", c.userTitleID, entry.ID)
	}
	if c.userID != userID {
		t.Errorf("userID = %v, want %v", c.userID, userID)
	}
}

// ---------------------------------------------------------------------------
// 10. One user failing doesn't stop other users (error tolerance)
// ---------------------------------------------------------------------------

func TestWorker_OneUserError_OtherUsersProcessed(t *testing.T) {
	userA := uuid.New()
	userB := uuid.New()

	// userA has a GetUserTitles error; userB succeeds.
	callCount := 0
	repo := &fakeWorkerRepo{
		userIDs: []uuid.UUID{userA, userB},
		titles:  map[uuid.UUID][]UserTitleEntry{userA: nil, userB: {}},
	}
	// Override titlesErr to fail only for userA.
	// Since fakeWorkerRepo is simple, we use a custom fake instead.
	customRepo := &errorForUserARepo{
		fakeWorkerRepo: repo,
		failUserID:     userA,
	}

	eng := &fakeWorkerEngine{}

	w := newTestWorker(eng, customRepo)
	w.RunOnce(context.Background())

	_ = callCount // satisfies "declared and not used" — real assertion is on eng.evaluateCalls
	eng.mu.Lock()
	defer eng.mu.Unlock()
	// EvaluateAndUnlock should be called for both users (error happens after evaluate
	// in GetUserTitles, but both users must be attempted).
	if len(eng.evaluateCalls) != 2 {
		t.Errorf("expected 2 EvaluateAndUnlock calls (both users attempted), got %d: %v",
			len(eng.evaluateCalls), eng.evaluateCalls)
	}
}

// errorForUserARepo wraps fakeWorkerRepo and returns an error from GetUserTitles
// for a specific user, to simulate partial failure without affecting other users.
type errorForUserARepo struct {
	*fakeWorkerRepo
	failUserID uuid.UUID
}

func (e *errorForUserARepo) GetUserTitles(ctx context.Context, userID uuid.UUID) ([]UserTitleEntry, error) {
	if userID == e.failUserID {
		return nil, errors.New("simulated GetUserTitles failure")
	}
	return e.fakeWorkerRepo.GetUserTitles(ctx, userID)
}
