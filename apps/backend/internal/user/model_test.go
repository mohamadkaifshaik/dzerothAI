package user

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func makeFullUser() *User {
	id := uuid.New()
	bio := "A test bio."
	avatar := "https://cdn.example.com/avatar.png"
	header := "https://cdn.example.com/header.png"
	loc := "Earth"
	website := "https://example.com"
	return &User{
		ID:            id,
		Handle:        "testuser",
		DisplayName:   "Test User",
		Email:         "test@example.com",
		EmailVerified: true,
		Bio:           &bio,
		AvatarURL:     &avatar,
		HeaderURL:     &header,
		Location:      &loc,
		WebsiteURL:    &website,
		IsPrivate:     false,
		IsSuspended:   false,
		CreatedAt:     time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2024, 1, 16, 8, 0, 0, 0, time.UTC),
	}
}

func makeMinimalUser() *User {
	id := uuid.New()
	return &User{
		ID:          id,
		Handle:      "minimal",
		DisplayName: "Minimal User",
		Email:       "minimal@example.com",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
}

// forbiddenJSONTags is the set of json tag names that must never appear on
// public-facing DTOs per CLAUDE.md section 2.3 (Public Metric Lockdown).
var forbiddenJSONTags = []string{
	"follower_count",
	"following_count",
	"like_count",
	"post_count",
	"impression_count",
	"bookmark_count",
}

// jsonTagsOf returns all json tag values found on fields of the struct v
// (searched recursively through embedded structs one level deep).
func jsonTagsOf(t reflect.Type) []string {
	var tags []string
	for i := range t.NumField() {
		field := t.Field(i)
		if field.Anonymous {
			// Embedded struct — recurse one level.
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
		// Strip options like ",omitempty".
		if comma := strings.Index(tag, ","); comma >= 0 {
			tag = tag[:comma]
		}
		tags = append(tags, tag)
	}
	return tags
}

// ---------------------------------------------------------------------------
// ToPublicProfile
// ---------------------------------------------------------------------------

func TestToPublicProfile_ContainsRequiredFields(t *testing.T) {
	u := makeFullUser()
	p := ToPublicProfile(u)

	if p.ID != u.ID.String() {
		t.Errorf("ID: got %q, want %q", p.ID, u.ID.String())
	}
	if p.Handle != u.Handle {
		t.Errorf("Handle: got %q, want %q", p.Handle, u.Handle)
	}
	if p.DisplayName != u.DisplayName {
		t.Errorf("DisplayName: got %q, want %q", p.DisplayName, u.DisplayName)
	}
	if p.Bio != u.Bio {
		t.Error("Bio pointer mismatch")
	}
	if p.AvatarURL != u.AvatarURL {
		t.Error("AvatarURL pointer mismatch")
	}
	if p.HeaderURL != u.HeaderURL {
		t.Error("HeaderURL pointer mismatch")
	}
	if p.Location != u.Location {
		t.Error("Location pointer mismatch")
	}
	if p.WebsiteURL != u.WebsiteURL {
		t.Error("WebsiteURL pointer mismatch")
	}
	if p.IsPrivate != u.IsPrivate {
		t.Errorf("IsPrivate: got %v, want %v", p.IsPrivate, u.IsPrivate)
	}
}

func TestToPublicProfile_JoinedAtIsISO8601UTC(t *testing.T) {
	u := makeFullUser()
	p := ToPublicProfile(u)

	if !strings.HasSuffix(p.JoinedAt, "Z") {
		t.Errorf("JoinedAt %q does not end with 'Z' — must be ISO 8601 UTC", p.JoinedAt)
	}

	// Verify it round-trips through RFC3339 parsing.
	parsed, err := time.Parse(time.RFC3339, p.JoinedAt)
	if err != nil {
		t.Errorf("JoinedAt %q is not valid RFC3339: %v", p.JoinedAt, err)
	}
	if !parsed.Equal(u.CreatedAt.UTC()) {
		t.Errorf("parsed JoinedAt %v does not match CreatedAt %v", parsed, u.CreatedAt.UTC())
	}
}

// TestPublicProfile_NoPublicMetricFields is the metric-lockdown invariant test.
// It uses reflection to assert that no json tag names matching the forbidden
// metric set appear on PublicProfile or any of its embedded types.
func TestPublicProfile_NoPublicMetricFields(t *testing.T) {
	tags := jsonTagsOf(reflect.TypeOf(PublicProfile{}))
	for _, forbidden := range forbiddenJSONTags {
		for _, tag := range tags {
			if tag == forbidden {
				t.Errorf("PublicProfile has forbidden json tag %q — violates public metric lockdown (CLAUDE.md §2.3)", forbidden)
			}
		}
	}
}

func TestToPublicProfile_NilOptionalFieldsAreNil(t *testing.T) {
	u := makeMinimalUser()
	// Bio, AvatarURL, HeaderURL, Location, WebsiteURL are nil by zero value.
	p := ToPublicProfile(u)

	if p.Bio != nil {
		t.Errorf("Bio: got %v, want nil", p.Bio)
	}
	if p.AvatarURL != nil {
		t.Errorf("AvatarURL: got %v, want nil", p.AvatarURL)
	}
	if p.HeaderURL != nil {
		t.Errorf("HeaderURL: got %v, want nil", p.HeaderURL)
	}
	if p.Location != nil {
		t.Errorf("Location: got %v, want nil", p.Location)
	}
	if p.WebsiteURL != nil {
		t.Errorf("WebsiteURL: got %v, want nil", p.WebsiteURL)
	}
}

// ---------------------------------------------------------------------------
// ToOwnProfile
// ---------------------------------------------------------------------------

func TestToOwnProfile_ContainsPublicFieldsPlusPrivate(t *testing.T) {
	u := makeFullUser()
	own := ToOwnProfile(u)

	// Public fields are embedded.
	if own.ID != u.ID.String() {
		t.Errorf("ID: got %q, want %q", own.ID, u.ID.String())
	}
	if own.Handle != u.Handle {
		t.Errorf("Handle: got %q, want %q", own.Handle, u.Handle)
	}

	// Private fields.
	if own.Email != u.Email {
		t.Errorf("Email: got %q, want %q", own.Email, u.Email)
	}
	if own.EmailVerified != u.EmailVerified {
		t.Errorf("EmailVerified: got %v, want %v", own.EmailVerified, u.EmailVerified)
	}
}

// TestOwnProfile_NoPublicMetricFields verifies that OwnProfile also has no
// forbidden metric fields, checking embedded PublicProfile via reflection.
func TestOwnProfile_NoPublicMetricFields(t *testing.T) {
	tags := jsonTagsOf(reflect.TypeOf(OwnProfile{}))
	for _, forbidden := range forbiddenJSONTags {
		for _, tag := range tags {
			if tag == forbidden {
				t.Errorf("OwnProfile has forbidden json tag %q — violates public metric lockdown (CLAUDE.md §2.3)", forbidden)
			}
		}
	}
}

// TestPublicProfile_SerializedJSONHasNoMetricKeys provides a belt-and-suspenders
// check: serialize to JSON and confirm forbidden keys are absent in the output.
func TestPublicProfile_SerializedJSONHasNoMetricKeys(t *testing.T) {
	u := makeFullUser()
	p := ToPublicProfile(u)

	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	jsonStr := string(b)

	for _, forbidden := range forbiddenJSONTags {
		if strings.Contains(jsonStr, forbidden) {
			t.Errorf("serialized PublicProfile JSON contains forbidden key %q", forbidden)
		}
	}
}
