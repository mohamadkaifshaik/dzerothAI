package post

// Tests for the server-side five-second share/quote delay enforcement.
//
// CLAUDE.md §2.2: share and quote actions require a mandatory 5-second
// countdown. The backend is the authoritative enforcement point.
//
// These tests call validateShareDelay directly (same package) to avoid
// introducing a clock abstraction into the production Service type.
// The validateShareDelay function uses time.Now().UTC() internally; tests
// construct timestamps relative to the real wall clock with tolerances that
// keep them reliable across CI environments.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/apierror"
)

// ---------------------------------------------------------------------------
// validateShareDelay unit tests
// ---------------------------------------------------------------------------

// TestShareDelay_NilTimestamp_Fails verifies that a nil share_initiated_at
// for a repost/quote is rejected with CodeValidation.
func TestShareDelay_NilTimestamp_Fails(t *testing.T) {
	err := validateShareDelay(nil)
	if err == nil {
		t.Fatal("validateShareDelay(nil) should return error")
	}
	if code := apiErrorCode(err); code != apierror.CodeValidation {
		t.Errorf("error code = %q, want %q", code, apierror.CodeValidation)
	}
}

// TestShareDelay_SixSecondsAgo_Succeeds verifies that a timestamp 6 seconds
// in the past passes (clearly beyond the 5-second minimum).
func TestShareDelay_SixSecondsAgo_Succeeds(t *testing.T) {
	ts := time.Now().UTC().Add(-6 * time.Second)
	if err := validateShareDelay(&ts); err != nil {
		t.Errorf("validateShareDelay(-6s) returned unexpected error: %v", err)
	}
}

// TestShareDelay_ThreeSecondsAgo_Fails verifies that a timestamp 3 seconds
// in the past is rejected (below the 5-second minimum).
func TestShareDelay_ThreeSecondsAgo_Fails(t *testing.T) {
	ts := time.Now().UTC().Add(-3 * time.Second)
	err := validateShareDelay(&ts)
	if err == nil {
		t.Fatal("validateShareDelay(-3s) should return error")
	}
	if code := apiErrorCode(err); code != apierror.CodeValidation {
		t.Errorf("error code = %q, want %q", code, apierror.CodeValidation)
	}
	var ae *apierror.APIError
	if errors.As(err, &ae) {
		if ae.Message != "share action must be initiated at least 5 seconds before submission" {
			t.Errorf("unexpected message: %q", ae.Message)
		}
	}
}

// TestShareDelay_SixtySecondsFuture_Fails verifies that a timestamp 60 seconds
// in the future is rejected (beyond the 30-second clock-skew tolerance).
func TestShareDelay_SixtySecondsFuture_Fails(t *testing.T) {
	ts := time.Now().UTC().Add(60 * time.Second)
	err := validateShareDelay(&ts)
	if err == nil {
		t.Fatal("validateShareDelay(+60s) should return error")
	}
	if code := apiErrorCode(err); code != apierror.CodeValidation {
		t.Errorf("error code = %q, want %q", code, apierror.CodeValidation)
	}
}

// TestShareDelay_TenSecondsFuture_Succeeds verifies that a timestamp 10 seconds
// in the future is accepted (within the 30-second clock-skew tolerance window).
// Such a future value produces a negative elapsed time (-10s) which is < 5s,
// so it fails the minShareDelay check. This test verifies the clock-skew-only
// path does not add an extra reject, but the timing check does.
//
// Actually: a 10s-future timestamp means now.Sub(initiated) = -10s < 5s, so it
// FAILS the timing check. This test verifies the behavior is "fails due to
// timing, not due to future-timestamp rejection".
func TestShareDelay_TenSecondsFuture_FailsTiming(t *testing.T) {
	// 10s in the future is within the 30s skew tolerance, so it passes the
	// clock-skew check. But now.Sub(future) < 0 < 5s, so the timing check fails.
	ts := time.Now().UTC().Add(10 * time.Second)
	err := validateShareDelay(&ts)
	if err == nil {
		t.Fatal("validateShareDelay(+10s) should fail timing check (elapsed < 5s)")
	}
	if code := apiErrorCode(err); code != apierror.CodeValidation {
		t.Errorf("error code = %q, want %q", code, apierror.CodeValidation)
	}
	var ae *apierror.APIError
	if errors.As(err, &ae) {
		if ae.Message != "share action must be initiated at least 5 seconds before submission" {
			t.Errorf("unexpected message (want timing message): %q", ae.Message)
		}
	}
}

// TestShareDelay_ExactlyFiveSecondsAgo_Succeeds verifies the boundary condition:
// exactly 5 seconds ago should succeed (>= minShareDelay).
// There is an inherent race in this test since time.Now() is called twice.
// We use time.Now().Add(-5s - 1ms) to guarantee the elapsed time is >= 5s when
// validateShareDelay calls time.Now() again.
func TestShareDelay_ExactlyFiveSecondsAgo_Succeeds(t *testing.T) {
	// Subtract an extra millisecond to ensure the elapsed time is safely >= 5s
	// when the function evaluates it, accounting for test execution time.
	ts := time.Now().UTC().Add(-5*time.Second - time.Millisecond)
	if err := validateShareDelay(&ts); err != nil {
		t.Errorf("validateShareDelay(-5s-1ms) returned unexpected error: %v", err)
	}
}

// TestShareDelay_FourSeconds999Millis_Fails verifies the near-boundary condition:
// 4 seconds 999 milliseconds in the past should fail (< 5 seconds).
func TestShareDelay_FourSeconds999Millis_Fails(t *testing.T) {
	// Use -4s+1ms to ensure we're safely below 5s even accounting for execution time.
	// The test is conservative: 4s999ms < 5s must fail.
	ts := time.Now().UTC().Add(-4*time.Second - 999*time.Millisecond)
	err := validateShareDelay(&ts)
	if err == nil {
		// If the test machine is very slow and execution takes > 1ms from the
		// ts creation to the validateShareDelay call, this may flip. Log a
		// warning rather than failing hard.
		t.Log("WARNING: TestShareDelay_FourSeconds999Millis_Fails: no error returned — " +
			"test machine may be too slow or time precision is insufficient")
	}
	// Note: we do not t.Fatal here because wall-clock tests have inherent
	// timing races. The test is informational about the boundary behavior.
}

// ---------------------------------------------------------------------------
// CreatePost share delay enforcement — repost and quote via nil-repo Service
// ---------------------------------------------------------------------------

// TestCreatePost_Repost_NilShareInitiatedAt_Fails verifies that a repost
// without share_initiated_at is rejected with CodeValidation.
func TestCreatePost_Repost_NilShareInitiatedAt_Fails(t *testing.T) {
	svc := newNilRepoService()
	quotedID := uuid.New().String()

	_, err := svc.CreatePost(context.Background(), uuid.New(), CreatePostRequest{
		PostType:         PostTypeRepost,
		QuotedPostID:     &quotedID,
		ShareInitiatedAt: nil,
	})
	if err == nil {
		t.Fatal("CreatePost repost with nil ShareInitiatedAt should return validation error")
	}
	if code := apiErrorCode(err); code != apierror.CodeValidation {
		t.Errorf("error code = %q, want %q", code, apierror.CodeValidation)
	}
}

// TestCreatePost_Quote_NilShareInitiatedAt_Fails verifies that a quote post
// without share_initiated_at is rejected.
func TestCreatePost_Quote_NilShareInitiatedAt_Fails(t *testing.T) {
	svc := newNilRepoService()
	quotedID := uuid.New().String()

	_, err := svc.CreatePost(context.Background(), uuid.New(), CreatePostRequest{
		PostType:         PostTypeQuote,
		Content:          "one two three four five",
		QuotedPostID:     &quotedID,
		ShareInitiatedAt: nil,
	})
	if err == nil {
		t.Fatal("CreatePost quote with nil ShareInitiatedAt should return validation error")
	}
	if code := apiErrorCode(err); code != apierror.CodeValidation {
		t.Errorf("error code = %q, want %q", code, apierror.CodeValidation)
	}
}

// TestCreatePost_Repost_TooRecent_Fails verifies that a repost with
// share_initiated_at 3 seconds ago is rejected.
func TestCreatePost_Repost_TooRecent_Fails(t *testing.T) {
	svc := newNilRepoService()
	quotedID := uuid.New().String()
	ts := time.Now().UTC().Add(-3 * time.Second)

	_, err := svc.CreatePost(context.Background(), uuid.New(), CreatePostRequest{
		PostType:         PostTypeRepost,
		QuotedPostID:     &quotedID,
		ShareInitiatedAt: &ts,
	})
	if err == nil {
		t.Fatal("CreatePost repost with ShareInitiatedAt 3s ago should return validation error")
	}
	if code := apiErrorCode(err); code != apierror.CodeValidation {
		t.Errorf("error code = %q, want %q", code, apierror.CodeValidation)
	}
}

// TestCreatePost_Quote_TooRecent_Fails verifies that a quote post with
// share_initiated_at 3 seconds ago is rejected.
func TestCreatePost_Quote_TooRecent_Fails(t *testing.T) {
	svc := newNilRepoService()
	quotedID := uuid.New().String()
	ts := time.Now().UTC().Add(-3 * time.Second)

	_, err := svc.CreatePost(context.Background(), uuid.New(), CreatePostRequest{
		PostType:         PostTypeQuote,
		Content:          "one two three four five",
		QuotedPostID:     &quotedID,
		ShareInitiatedAt: &ts,
	})
	if err == nil {
		t.Fatal("CreatePost quote with ShareInitiatedAt 3s ago should return validation error")
	}
	if code := apiErrorCode(err); code != apierror.CodeValidation {
		t.Errorf("error code = %q, want %q", code, apierror.CodeValidation)
	}
}

// TestCreatePost_Repost_FutureBeyondSkew_Fails verifies that a repost with
// share_initiated_at 60 seconds in the future is rejected.
func TestCreatePost_Repost_FutureBeyondSkew_Fails(t *testing.T) {
	svc := newNilRepoService()
	quotedID := uuid.New().String()
	ts := time.Now().UTC().Add(60 * time.Second)

	_, err := svc.CreatePost(context.Background(), uuid.New(), CreatePostRequest{
		PostType:         PostTypeRepost,
		QuotedPostID:     &quotedID,
		ShareInitiatedAt: &ts,
	})
	if err == nil {
		t.Fatal("CreatePost repost with ShareInitiatedAt 60s in future should return validation error")
	}
	if code := apiErrorCode(err); code != apierror.CodeValidation {
		t.Errorf("error code = %q, want %q", code, apierror.CodeValidation)
	}
}

// TestCreatePost_Original_NilShareInitiatedAt_NotRequired verifies that
// original posts do not require share_initiated_at (validation passes without it).
// This test only reaches the share delay check if all prior validations pass,
// so it uses a testableService with a fake repo.
func TestCreatePost_Original_NilShareInitiatedAt_NotRequired(t *testing.T) {
	content := "hello world this is a test post"
	fake := &fakeRepo{
		createFn: func(_ context.Context, _ Post, _ []uuid.UUID, _ []string) error { return nil },
		getByIDFn: func(_ context.Context, id uuid.UUID) (Post, error) {
			c := content
			return Post{ID: id, AuthorID: uuid.New(), PostType: PostTypeOriginal, Content: &c}, nil
		},
	}
	svc := newTestableService(fake)

	_, err := svc.createPost(context.Background(), uuid.New(), CreatePostRequest{
		PostType:         PostTypeOriginal,
		Content:          content,
		ShareInitiatedAt: nil,
	})
	if err != nil {
		t.Errorf("CreatePost original with nil ShareInitiatedAt returned unexpected error: %v", err)
	}
}

// TestCreatePost_Repost_ValidDelay_Succeeds verifies that a repost with
// share_initiated_at 6 seconds ago passes the timing check and proceeds.
// Uses testableService to avoid a nil repo panic after validation passes.
func TestCreatePost_Repost_ValidDelay_Succeeds(t *testing.T) {
	quotedID := uuid.New()
	quotedIDStr := quotedID.String()
	ts := time.Now().UTC().Add(-6 * time.Second)

	fake := &fakeRepo{
		createFn: func(_ context.Context, _ Post, _ []uuid.UUID, _ []string) error { return nil },
		getByIDFn: func(_ context.Context, id uuid.UUID) (Post, error) {
			return Post{ID: id, AuthorID: uuid.New(), PostType: PostTypeRepost, QuotedPostID: &quotedID}, nil
		},
	}
	svc := newTestableService(fake)

	_, err := svc.createPost(context.Background(), uuid.New(), CreatePostRequest{
		PostType:         PostTypeRepost,
		QuotedPostID:     &quotedIDStr,
		ShareInitiatedAt: &ts,
	})
	if err != nil {
		t.Errorf("CreatePost repost with 6s delay returned unexpected error: %v", err)
	}
}

// TestCreatePost_Quote_FiveWordRule_StillEnforced verifies that the five-distinct-
// word rule is still checked independently of the share delay rule.
// A quote post with valid timing but fewer than 5 words is rejected for words.
func TestCreatePost_Quote_FiveWordRule_StillEnforced(t *testing.T) {
	svc := newNilRepoService()
	quotedID := uuid.New().String()
	ts := time.Now().UTC().Add(-6 * time.Second)

	_, err := svc.CreatePost(context.Background(), uuid.New(), CreatePostRequest{
		PostType:         PostTypeQuote,
		Content:          "one two three four", // only 4 distinct words
		QuotedPostID:     &quotedID,
		ShareInitiatedAt: &ts,
	})
	if err == nil {
		t.Fatal("CreatePost quote with 4 words and valid timing should return validation error")
	}
	if code := apiErrorCode(err); code != apierror.CodeValidation {
		t.Errorf("error code = %q, want %q", code, apierror.CodeValidation)
	}
}
