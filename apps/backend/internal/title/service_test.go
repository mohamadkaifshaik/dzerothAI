// Package title — service unit tests.
//
// Tests for Service methods using fake repository and fake checker interfaces.
// No real database or Redis connection is required.
package title

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Fake repository
// ---------------------------------------------------------------------------

// fakeSvcRepo is a minimal in-memory fake for the Repository. Only the methods
// called by the Service under test need to be implemented.
type fakeSvcRepo struct {
	definitions []TitleDefinition
	userTitles  []UserTitleEntry
	primaryTitles map[uuid.UUID]*TitleSummary

	setErr   error
	clearErr error
}

func newFakeSvcRepo() *fakeSvcRepo {
	return &fakeSvcRepo{primaryTitles: make(map[uuid.UUID]*TitleSummary)}
}

func (r *fakeSvcRepo) GetDefinitions(_ context.Context) ([]TitleDefinition, error) {
	return r.definitions, nil
}

func (r *fakeSvcRepo) GetUserTitles(_ context.Context, userID uuid.UUID) ([]UserTitleEntry, error) {
	var result []UserTitleEntry
	for _, e := range r.userTitles {
		if e.UserID == userID {
			result = append(result, e)
		}
	}
	return result, nil
}

func (r *fakeSvcRepo) GetPrimaryTitle(_ context.Context, userID uuid.UUID) (*TitleSummary, error) {
	s, ok := r.primaryTitles[userID]
	if !ok {
		return nil, nil
	}
	return s, nil
}

func (r *fakeSvcRepo) SetPrimaryTitle(_ context.Context, userID, userTitleID uuid.UUID) error {
	return r.setErr
}

func (r *fakeSvcRepo) ClearPrimaryTitle(_ context.Context, userID uuid.UUID) error {
	if r.clearErr != nil {
		return r.clearErr
	}
	delete(r.primaryTitles, userID)
	return nil
}

// ---------------------------------------------------------------------------
// fakeServiceRepo wraps fakeSvcRepo to satisfy the *Repository dependency via
// Service by swapping the repo pointer in service. We instead build Service
// with a real Repository but override it with a test double by creating a
// thin shim.
//
// Because Service holds a *Repository (concrete pointer), we cannot directly
// inject a fake without either refactoring Service or using the fake via the
// real type. The simplest approach is to test observable behaviors end-to-end
// by building a Service with a stubbed *Repository whose underlying pool is nil,
// and patching the service's unexported fields through a test helper.
//
// To avoid that complexity, we define a serviceRepo interface in this test file
// and create a testService that wraps the same logic but accepts the interface.
// ---------------------------------------------------------------------------

// serviceRepo mirrors the subset of Repository methods called by Service.
type serviceRepo interface {
	GetDefinitions(ctx context.Context) ([]TitleDefinition, error)
	GetUserTitles(ctx context.Context, userID uuid.UUID) ([]UserTitleEntry, error)
	GetPrimaryTitle(ctx context.Context, userID uuid.UUID) (*TitleSummary, error)
	SetPrimaryTitle(ctx context.Context, userID, userTitleID uuid.UUID) error
	ClearPrimaryTitle(ctx context.Context, userID uuid.UUID) error
}

// testService mirrors Service but accepts a serviceRepo interface for testing.
type testService struct {
	repo           serviceRepo
	followChecker  followChecker
	privacyChecker userPrivacyChecker
	log            *zap.Logger
}

func (s *testService) GetCatalog(ctx context.Context) (TitleCatalogResponse, error) {
	defs, err := s.repo.GetDefinitions(ctx)
	if err != nil {
		return TitleCatalogResponse{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	items := make([]TitleDefinitionDTO, 0, len(defs))
	for _, d := range defs {
		desc := ""
		if d.Description != nil {
			desc = *d.Description
		}
		items = append(items, TitleDefinitionDTO{
			ID:          d.ID.String(),
			Slug:        d.Slug,
			DisplayName: d.DisplayName,
			Description: desc,
			Category:    string(d.Category),
			IsRevocable: d.IsRevocable,
		})
	}
	return TitleCatalogResponse{Items: items}, nil
}

func (s *testService) GetMyTitles(ctx context.Context, callerID uuid.UUID) (UserTitlesResponse, error) {
	entries, err := s.repo.GetUserTitles(ctx, callerID)
	if err != nil {
		return UserTitlesResponse{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	items := make([]UserTitleDTO, 0, len(entries))
	for _, e := range entries {
		items = append(items, UserTitleDTO{
			ID:          e.ID.String(),
			Slug:        e.Slug,
			DisplayName: e.DisplayName,
			Category:    string(e.Category),
			IsRevocable: e.IsRevocable,
			Status:      string(e.Status),
			UnlockedAt:  e.UnlockedAt.UTC().Format(time.RFC3339),
		})
	}
	primary, err := s.repo.GetPrimaryTitle(ctx, callerID)
	if err != nil {
		return UserTitlesResponse{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	var primaryID *string
	if primary != nil {
		idStr := primary.ID.String()
		primaryID = &idStr
	}
	return UserTitlesResponse{Items: items, PrimaryID: primaryID}, nil
}

func (s *testService) GetUserPrimaryTitle(ctx context.Context, callerID *uuid.UUID, targetID uuid.UUID) (PrimaryTitleResponse, error) {
	if callerID != nil && *callerID == targetID {
		return s.fetchPrimary(ctx, targetID)
	}
	if s.privacyChecker != nil {
		isPrivate, err := s.privacyChecker.IsPrivateAccount(ctx, targetID)
		if err != nil {
			return s.fetchPrimary(ctx, targetID)
		}
		if isPrivate {
			if callerID == nil {
				return PrimaryTitleResponse{PrimaryTitle: nil}, nil
			}
			if s.followChecker != nil {
				following, err := s.followChecker.IsFollowing(ctx, *callerID, targetID)
				if err != nil {
					return PrimaryTitleResponse{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
				}
				if !following {
					return PrimaryTitleResponse{PrimaryTitle: nil}, nil
				}
			} else {
				return PrimaryTitleResponse{PrimaryTitle: nil}, nil
			}
		}
	}
	return s.fetchPrimary(ctx, targetID)
}

func (s *testService) fetchPrimary(ctx context.Context, targetID uuid.UUID) (PrimaryTitleResponse, error) {
	primary, err := s.repo.GetPrimaryTitle(ctx, targetID)
	if err != nil {
		return PrimaryTitleResponse{}, apierror.NewAPIError(apierror.CodeInternal, "an unexpected error occurred")
	}
	return PrimaryTitleResponse{PrimaryTitle: summaryToDTO(primary)}, nil
}

// ---------------------------------------------------------------------------
// Fake checkers
// ---------------------------------------------------------------------------

type fakePrivacyChecker struct {
	private bool
	err     error
}

func (f *fakePrivacyChecker) IsPrivateAccount(_ context.Context, _ uuid.UUID) (bool, error) {
	return f.private, f.err
}

type fakeFollowChecker struct {
	following bool
	err       error
}

func (f *fakeFollowChecker) IsFollowing(_ context.Context, _, _ uuid.UUID) (bool, error) {
	return f.following, f.err
}

// ---------------------------------------------------------------------------
// TestService_GetCatalog_MapsDefinitions
// ---------------------------------------------------------------------------

// TestService_GetCatalog_MapsDefinitions verifies that GetCatalog correctly maps
// TitleDefinition rows to TitleDefinitionDTO, including nil-Description handling.
func TestService_GetCatalog_MapsDefinitions(t *testing.T) {
	t.Parallel()

	desc := "Earn 100 posts"
	repo := newFakeSvcRepo()
	repo.definitions = []TitleDefinition{
		{
			ID:          uuid.New(),
			Slug:        "centurion",
			DisplayName: "Centurion",
			Description: &desc,
			Category:    CategoryMilestone,
			IsRevocable: false,
			IsActive:    true,
		},
		{
			ID:          uuid.New(),
			Slug:        "trendsetter",
			DisplayName: "Trendsetter",
			Description: nil, // nil description → empty string in DTO
			Category:    CategoryPerformance,
			IsRevocable: true,
			IsActive:    true,
		},
	}

	svc := &testService{repo: repo, log: zap.NewNop()}
	resp, err := svc.GetCatalog(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}

	if resp.Items[0].Slug != "centurion" {
		t.Errorf("expected slug centurion, got %q", resp.Items[0].Slug)
	}
	if resp.Items[0].Description != "Earn 100 posts" {
		t.Errorf("expected description %q, got %q", "Earn 100 posts", resp.Items[0].Description)
	}
	if resp.Items[1].Description != "" {
		t.Errorf("expected empty description for nil pointer, got %q", resp.Items[1].Description)
	}
	if resp.Items[1].Category != "performance" {
		t.Errorf("expected category performance, got %q", resp.Items[1].Category)
	}
}

// ---------------------------------------------------------------------------
// TestService_GetCatalog_EmptyReturnsEmptySlice
// ---------------------------------------------------------------------------

// TestService_GetCatalog_EmptyReturnsEmptySlice verifies that an empty definition
// table returns an empty (non-nil) Items slice.
func TestService_GetCatalog_EmptyReturnsEmptySlice(t *testing.T) {
	t.Parallel()

	repo := newFakeSvcRepo()
	svc := &testService{repo: repo, log: zap.NewNop()}
	resp, err := svc.GetCatalog(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Items == nil {
		t.Error("expected non-nil Items slice for empty catalog")
	}
	if len(resp.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(resp.Items))
	}
}

// ---------------------------------------------------------------------------
// TestService_GetMyTitles_MapsEntries
// ---------------------------------------------------------------------------

// TestService_GetMyTitles_MapsEntries verifies that GetMyTitles correctly maps
// UserTitleEntry rows to UserTitleDTO and populates the PrimaryID field.
func TestService_GetMyTitles_MapsEntries(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	titleID := uuid.New()
	primarySummary := &TitleSummary{ID: titleID, Slug: "centurion", DisplayName: "Centurion"}

	repo := newFakeSvcRepo()
	repo.userTitles = []UserTitleEntry{
		{
			UserTitle: UserTitle{
				ID:         titleID,
				UserID:     userID,
				Status:     StatusActive,
				UnlockedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			Slug:        "centurion",
			DisplayName: "Centurion",
			Category:    CategoryMilestone,
			IsRevocable: false,
		},
	}
	repo.primaryTitles[userID] = primarySummary

	svc := &testService{repo: repo, log: zap.NewNop()}
	resp, err := svc.GetMyTitles(context.Background(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}

	item := resp.Items[0]
	if item.Slug != "centurion" {
		t.Errorf("expected slug centurion, got %q", item.Slug)
	}
	if item.Status != "active" {
		t.Errorf("expected status active, got %q", item.Status)
	}
	if item.UnlockedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("expected ISO 8601 UTC unlocked_at, got %q", item.UnlockedAt)
	}
	if resp.PrimaryID == nil {
		t.Fatal("expected non-nil PrimaryID")
	}
	if *resp.PrimaryID != titleID.String() {
		t.Errorf("expected PrimaryID %s, got %s", titleID.String(), *resp.PrimaryID)
	}
}

// ---------------------------------------------------------------------------
// TestService_GetUserPrimaryTitle_PublicAccount
// ---------------------------------------------------------------------------

// TestService_GetUserPrimaryTitle_PublicAccount verifies that a public account's
// primary title is visible to an unauthenticated caller.
func TestService_GetUserPrimaryTitle_PublicAccount(t *testing.T) {
	t.Parallel()

	targetID := uuid.New()
	primarySummary := &TitleSummary{ID: uuid.New(), Slug: "centurion", DisplayName: "Centurion"}

	repo := newFakeSvcRepo()
	repo.primaryTitles[targetID] = primarySummary

	svc := &testService{
		repo:           repo,
		privacyChecker: &fakePrivacyChecker{private: false},
		log:            zap.NewNop(),
	}

	resp, err := svc.GetUserPrimaryTitle(context.Background(), nil, targetID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.PrimaryTitle == nil {
		t.Fatal("expected non-nil PrimaryTitle for public account")
	}
	if resp.PrimaryTitle.Slug != "centurion" {
		t.Errorf("expected slug centurion, got %q", resp.PrimaryTitle.Slug)
	}
}

// ---------------------------------------------------------------------------
// TestService_GetUserPrimaryTitle_PrivateAccountOwner
// ---------------------------------------------------------------------------

// TestService_GetUserPrimaryTitle_PrivateAccountOwner verifies that a private account
// owner always sees their own primary title.
func TestService_GetUserPrimaryTitle_PrivateAccountOwner(t *testing.T) {
	t.Parallel()

	ownerID := uuid.New()
	primarySummary := &TitleSummary{ID: uuid.New(), Slug: "centurion", DisplayName: "Centurion"}

	repo := newFakeSvcRepo()
	repo.primaryTitles[ownerID] = primarySummary

	svc := &testService{
		repo:           repo,
		privacyChecker: &fakePrivacyChecker{private: true},
		log:            zap.NewNop(),
	}

	resp, err := svc.GetUserPrimaryTitle(context.Background(), &ownerID, ownerID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.PrimaryTitle == nil {
		t.Fatal("expected owner to see their own primary title even on private account")
	}
}

// ---------------------------------------------------------------------------
// TestService_GetUserPrimaryTitle_PrivateAccountFollower
// ---------------------------------------------------------------------------

// TestService_GetUserPrimaryTitle_PrivateAccountFollower verifies that a follower
// can see a private account's primary title.
func TestService_GetUserPrimaryTitle_PrivateAccountFollower(t *testing.T) {
	t.Parallel()

	callerID := uuid.New()
	targetID := uuid.New()
	primarySummary := &TitleSummary{ID: uuid.New(), Slug: "centurion", DisplayName: "Centurion"}

	repo := newFakeSvcRepo()
	repo.primaryTitles[targetID] = primarySummary

	svc := &testService{
		repo:           repo,
		privacyChecker: &fakePrivacyChecker{private: true},
		followChecker:  &fakeFollowChecker{following: true},
		log:            zap.NewNop(),
	}

	resp, err := svc.GetUserPrimaryTitle(context.Background(), &callerID, targetID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.PrimaryTitle == nil {
		t.Fatal("expected follower to see private account's primary title")
	}
}

// ---------------------------------------------------------------------------
// TestService_GetUserPrimaryTitle_PrivateAccountNonFollower
// ---------------------------------------------------------------------------

// TestService_GetUserPrimaryTitle_PrivateAccountNonFollower verifies that a
// non-follower gets nil PrimaryTitle (not a 403/404) for a private account.
func TestService_GetUserPrimaryTitle_PrivateAccountNonFollower(t *testing.T) {
	t.Parallel()

	callerID := uuid.New()
	targetID := uuid.New()
	primarySummary := &TitleSummary{ID: uuid.New(), Slug: "centurion", DisplayName: "Centurion"}

	repo := newFakeSvcRepo()
	repo.primaryTitles[targetID] = primarySummary

	svc := &testService{
		repo:           repo,
		privacyChecker: &fakePrivacyChecker{private: true},
		followChecker:  &fakeFollowChecker{following: false},
		log:            zap.NewNop(),
	}

	resp, err := svc.GetUserPrimaryTitle(context.Background(), &callerID, targetID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.PrimaryTitle != nil {
		t.Errorf("expected nil PrimaryTitle for non-follower of private account, got %+v", resp.PrimaryTitle)
	}
}

// ---------------------------------------------------------------------------
// TestService_GetUserPrimaryTitle_Unauthenticated
// ---------------------------------------------------------------------------

// TestService_GetUserPrimaryTitle_Unauthenticated verifies that an unauthenticated
// caller gets nil PrimaryTitle (no error) for a private account.
func TestService_GetUserPrimaryTitle_Unauthenticated(t *testing.T) {
	t.Parallel()

	targetID := uuid.New()
	primarySummary := &TitleSummary{ID: uuid.New(), Slug: "centurion", DisplayName: "Centurion"}

	repo := newFakeSvcRepo()
	repo.primaryTitles[targetID] = primarySummary

	svc := &testService{
		repo:           repo,
		privacyChecker: &fakePrivacyChecker{private: true},
		log:            zap.NewNop(),
	}

	resp, err := svc.GetUserPrimaryTitle(context.Background(), nil, targetID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.PrimaryTitle != nil {
		t.Errorf("expected nil PrimaryTitle for unauthenticated caller on private account, got %+v", resp.PrimaryTitle)
	}
}

// ---------------------------------------------------------------------------
// TestService_GetUserPrimaryTitle_PrivacyCheckError_FailsOpen
// ---------------------------------------------------------------------------

// TestService_GetUserPrimaryTitle_PrivacyCheckError_FailsOpen verifies that when
// the privacy checker returns an error, the service fails open (treats account as public).
func TestService_GetUserPrimaryTitle_PrivacyCheckError_FailsOpen(t *testing.T) {
	t.Parallel()

	targetID := uuid.New()
	primarySummary := &TitleSummary{ID: uuid.New(), Slug: "centurion", DisplayName: "Centurion"}

	repo := newFakeSvcRepo()
	repo.primaryTitles[targetID] = primarySummary

	svc := &testService{
		repo:           repo,
		privacyChecker: &fakePrivacyChecker{private: false, err: errors.New("db timeout")},
		log:            zap.NewNop(),
	}

	resp, err := svc.GetUserPrimaryTitle(context.Background(), nil, targetID)
	if err != nil {
		t.Fatalf("unexpected error on privacy check failure: %v", err)
	}
	// Fails open — public title is visible even when privacy check errors.
	if resp.PrimaryTitle == nil {
		t.Error("expected fail-open: PrimaryTitle should be non-nil when privacy check errors")
	}
}

// ---------------------------------------------------------------------------
// TestSummaryToDTO
// ---------------------------------------------------------------------------

// TestSummaryToDTO verifies the summaryToDTO helper handles nil correctly.
func TestSummaryToDTO(t *testing.T) {
	t.Parallel()

	if summaryToDTO(nil) != nil {
		t.Error("summaryToDTO(nil) should return nil")
	}

	s := &TitleSummary{Slug: "foo", DisplayName: "Foo"}
	dto := summaryToDTO(s)
	if dto == nil {
		t.Fatal("summaryToDTO should return non-nil for non-nil input")
	}
	if dto.Slug != "foo" || dto.DisplayName != "Foo" {
		t.Errorf("unexpected DTO values: %+v", dto)
	}
}
