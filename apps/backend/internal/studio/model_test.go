package studio

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// forbiddenStudioPublicMetricTags is the set of json tag names that must never
// appear on public-facing DTOs per CLAUDE.md §2.3 (Public Metric Lockdown).
// PostAnalytics is private/owner-only and intentionally contains aggregate counts,
// but it must never include follower counts, following counts, impression counts,
// or equivalent public social-validation metrics.
var forbiddenStudioPublicMetricTags = []string{
	"follower_count",
	"following_count",
	"like_count",
	"impression_count",
	"view_count",
	"reach_count",
	"engagement_count",
	"repost_count",
	"share_count",
}

// requiredPostAnalyticsTags are the private aggregate field tags that must
// be present on PostAnalytics to confirm the struct is correctly defined.
var requiredPostAnalyticsTags = []string{
	"reaction_count",
	"bookmark_count",
	"reply_count",
	"quote_count",
}

// jsonTagsOf returns all json tag values found on struct fields of t,
// recursing into embedded structs one level deep (mirrors user/model_test.go).
func jsonTagsOf(t reflect.Type) []string {
	var tags []string
	for i := range t.NumField() {
		field := t.Field(i)
		if field.Anonymous {
			embedded := field.Type
			if embedded.Kind() == reflect.Ptr {
				embedded = embedded.Elem()
			}
			if embedded.Kind() == reflect.Struct {
				tags = append(tags, jsonTagsOf(embedded)...)
			}
			continue
		}
		tag := field.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		if comma := strings.Index(tag, ","); comma >= 0 {
			tag = tag[:comma]
		}
		tags = append(tags, tag)
	}
	return tags
}

// TestPostAnalytics_HasRequiredPrivateCountFields verifies that PostAnalytics
// contains each expected private aggregate field (confirming struct correctness).
func TestPostAnalytics_HasRequiredPrivateCountFields(t *testing.T) {
	tags := jsonTagsOf(reflect.TypeOf(PostAnalytics{}))
	tagSet := make(map[string]bool, len(tags))
	for _, tag := range tags {
		tagSet[tag] = true
	}

	for _, required := range requiredPostAnalyticsTags {
		if !tagSet[required] {
			t.Errorf("PostAnalytics is missing required private count field with json tag %q", required)
		}
	}
}

// TestPostAnalytics_HasNoForbiddenPublicMetricFields uses reflection to assert
// that no json tag name matching the forbidden public-metric set appears on
// PostAnalytics. This documents that even private analytics types respect the
// broader metric-lockdown naming convention (CLAUDE.md §2.3).
func TestPostAnalytics_HasNoForbiddenPublicMetricFields(t *testing.T) {
	tags := jsonTagsOf(reflect.TypeOf(PostAnalytics{}))
	for _, forbidden := range forbiddenStudioPublicMetricTags {
		for _, tag := range tags {
			if tag == forbidden {
				t.Errorf("PostAnalytics has forbidden json tag %q — violates public metric lockdown (CLAUDE.md §2.3)", forbidden)
			}
		}
	}
}

// TestPostAnalytics_SerializedJSONHasNoForbiddenKeys belt-and-suspenders check:
// serialize a zero-value PostAnalytics to JSON and confirm no forbidden keys appear.
func TestPostAnalytics_SerializedJSONHasNoForbiddenKeys(t *testing.T) {
	pa := PostAnalytics{
		PostID:        "test-id",
		Content:       "hello world",
		PostType:      "original",
		CreatedAt:     "2024-01-01T00:00:00Z",
		ReactionCount: 1,
		BookmarkCount: 2,
		ReplyCount:    3,
		QuoteCount:    4,
	}

	b, err := json.Marshal(pa)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	jsonStr := string(b)

	for _, forbidden := range forbiddenStudioPublicMetricTags {
		if strings.Contains(jsonStr, forbidden) {
			t.Errorf("serialized PostAnalytics JSON contains forbidden key %q (CLAUDE.md §2.3)", forbidden)
		}
	}
}

// TestStudioPage_TerminatedField confirms that StudioPage has a terminated
// json field — required for the finite-feed contract (CLAUDE.md §2.1).
func TestStudioPage_TerminatedField(t *testing.T) {
	tags := jsonTagsOf(reflect.TypeOf(StudioPage{}))
	found := false
	for _, tag := range tags {
		if tag == "terminated" {
			found = true
			break
		}
	}
	if !found {
		t.Error("StudioPage is missing the 'terminated' json field — required by finite feed contract (CLAUDE.md §2.1)")
	}
}

// TestPostAnalytics_NotInPostDTO is a documentation test:
// PostAnalytics lives in internal/studio and cannot be imported by
// internal/post without introducing a circular dependency.
// This test confirms PostAnalytics is defined in the studio package,
// providing a compile-time guarantee that it cannot accidentally appear
// in post.PostDTO or any other public response type.
func TestPostAnalytics_IsInStudioPackage(t *testing.T) {
	typ := reflect.TypeOf(PostAnalytics{})
	if typ.PkgPath() != "github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/studio" {
		t.Errorf("PostAnalytics is in package %q, want internal/studio — isolation contract violated", typ.PkgPath())
	}
}
