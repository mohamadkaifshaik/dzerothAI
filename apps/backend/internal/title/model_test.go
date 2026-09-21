// Package title — model tests.
//
// Verifies type constants, DTO field-level invariants, and the public metric
// lockdown contract on all title DTOs (CLAUDE.md §2.3).
// No database connection is required.
package title

import (
	"reflect"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// TitleCategory constants
// ---------------------------------------------------------------------------

func TestTitleCategory_Constants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value TitleCategory
		want  string
	}{
		{"milestone", CategoryMilestone, "milestone"},
		{"niche", CategoryNiche, "niche"},
		{"performance", CategoryPerformance, "performance"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.value) != tc.want {
				t.Errorf("TitleCategory %q = %q, want %q", tc.name, tc.value, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TitleStatus constants
// ---------------------------------------------------------------------------

func TestTitleStatus_Constants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value TitleStatus
		want  string
	}{
		{"active", StatusActive, "active"},
		{"grace_period", StatusGracePeriod, "grace_period"},
		{"revoked", StatusRevoked, "revoked"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.value) != tc.want {
				t.Errorf("TitleStatus %q = %q, want %q", tc.name, tc.value, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Public metric lockdown: TitleDefinitionDTO
// ---------------------------------------------------------------------------

// forbiddenMetricSubstrings are substrings that must not appear in any public
// title DTO field name or json tag — CLAUDE.md §2.3.
var forbiddenMetricSubstrings = []string{
	"count", "like", "impression", "bookmark", "follower",
	"share", "repost", "retweet", "view", "reach", "engagement",
}

func dtoFieldNames(t reflect.Type) (names []string, tags []string) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		names = append(names, strings.ToLower(f.Name))
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		if comma := strings.Index(tag, ","); comma >= 0 {
			tag = tag[:comma]
		}
		tags = append(tags, tag)
	}
	return names, tags
}

func assertNoMetricFields(t *testing.T, dt reflect.Type) {
	t.Helper()
	names, tags := dtoFieldNames(dt)
	for _, forbidden := range forbiddenMetricSubstrings {
		for _, name := range names {
			if strings.Contains(name, forbidden) {
				t.Errorf("%s field name %q contains forbidden substring %q (CLAUDE.md §2.3)", dt.Name(), name, forbidden)
			}
		}
		for _, tag := range tags {
			if strings.Contains(tag, forbidden) {
				t.Errorf("%s json tag %q contains forbidden substring %q (CLAUDE.md §2.3)", dt.Name(), tag, forbidden)
			}
		}
	}
}

func TestPublicMetricLockdown_TitleDefinitionDTO(t *testing.T) {
	t.Parallel()
	assertNoMetricFields(t, reflect.TypeOf(TitleDefinitionDTO{}))
}

func TestPublicMetricLockdown_UserTitleDTO(t *testing.T) {
	t.Parallel()
	assertNoMetricFields(t, reflect.TypeOf(UserTitleDTO{}))
}

func TestPublicMetricLockdown_TitleSummaryDTO(t *testing.T) {
	t.Parallel()
	assertNoMetricFields(t, reflect.TypeOf(TitleSummaryDTO{}))
}

func TestPublicMetricLockdown_UserTitlesResponse(t *testing.T) {
	t.Parallel()
	assertNoMetricFields(t, reflect.TypeOf(UserTitlesResponse{}))
}

func TestPublicMetricLockdown_PrimaryTitleResponse(t *testing.T) {
	t.Parallel()
	assertNoMetricFields(t, reflect.TypeOf(PrimaryTitleResponse{}))
}

// ---------------------------------------------------------------------------
// TitleStatusUpdate: nil pointer semantics
// ---------------------------------------------------------------------------

// TestTitleStatusUpdate_NilPointers verifies that a zero-value TitleStatusUpdate
// leaves all optional fields as nil, confirming the COALESCE sentinel behavior
// relied upon by UpdateUserTitleStatus.
func TestTitleStatusUpdate_NilPointers(t *testing.T) {
	t.Parallel()

	u := TitleStatusUpdate{Status: StatusActive}
	if u.GracePeriodEndsAt != nil {
		t.Error("GracePeriodEndsAt should be nil when not set")
	}
	if u.RevokedAt != nil {
		t.Error("RevokedAt should be nil when not set")
	}
	if u.GraceNotificationSent != nil {
		t.Error("GraceNotificationSent should be nil when not set")
	}
	if u.UnlockNotificationSent != nil {
		t.Error("UnlockNotificationSent should be nil when not set")
	}
}

// ---------------------------------------------------------------------------
// Repository sentinel errors
// ---------------------------------------------------------------------------

func TestRepositoryErrors_AreDistinct(t *testing.T) {
	t.Parallel()

	if ErrNotFound == ErrForbidden {
		t.Error("ErrNotFound and ErrForbidden must be distinct error values")
	}
	if ErrNotFound == nil || ErrForbidden == nil {
		t.Error("sentinel errors must not be nil")
	}
}

// ---------------------------------------------------------------------------
// TitleSummary: internal type retains ID for FK resolution
// ---------------------------------------------------------------------------

func TestTitleSummary_HasIDField(t *testing.T) {
	t.Parallel()

	dt := reflect.TypeOf(TitleSummary{})
	_, ok := dt.FieldByName("ID")
	if !ok {
		t.Error("TitleSummary must have an ID field for FK resolution in PostAuthor (Phase 8)")
	}
}
