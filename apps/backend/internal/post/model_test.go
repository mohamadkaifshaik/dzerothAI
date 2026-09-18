// Package post — model tests.
//
// This file verifies the public metric lockdown contract on PostDTO
// (CLAUDE.md §2.3). Tests use reflection to inspect struct field names and
// json tags, and also serialize to JSON to catch any leakage at the wire level.
package post

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// forbiddenMetricSubstrings are substrings that must NOT appear in any PostDTO
// field name or json tag. A match on any substring is sufficient to violate
// the public metric lockdown rule (CLAUDE.md §2.3).
var forbiddenMetricSubstrings = []string{
	"count",
	"like",
	"impression",
	"bookmark",
	"follower",
	"share",
	"repost",
	"retweet",
	"view",
	"reach",
	"engagement",
}

// postDTOFieldNames returns all field names (lowercase) and json tag values
// found on a struct type, recursing one level into embedded/nested structs.
func postDTOFieldNames(t reflect.Type) (names []string, tags []string) {
	for i := range t.NumField() {
		f := t.Field(i)

		// Recurse into nested structs (not just embedded/anonymous ones).
		ft := f.Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && ft != reflect.TypeOf(time.Time{}) {
			subNames, subTags := postDTOFieldNames(ft)
			names = append(names, subNames...)
			tags = append(tags, subTags...)
		}

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

// TestPostDTO_NoPublicMetricFields verifies — via reflection — that PostDTO
// contains no field that could expose social-validation metrics publicly.
// This is a compile-time-enforced contract per CLAUDE.md §2.3.
func TestPostDTO_NoPublicMetricFields(t *testing.T) {
	names, tags := postDTOFieldNames(reflect.TypeOf(PostDTO{}))

	for _, forbidden := range forbiddenMetricSubstrings {
		for _, name := range names {
			if strings.Contains(name, forbidden) {
				t.Errorf("PostDTO field name %q contains forbidden substring %q — violates public metric lockdown (CLAUDE.md §2.3)", name, forbidden)
			}
		}
		for _, tag := range tags {
			if strings.Contains(tag, forbidden) {
				t.Errorf("PostDTO json tag %q contains forbidden substring %q — violates public metric lockdown (CLAUDE.md §2.3)", tag, forbidden)
			}
		}
	}
}

// TestPostDTO_SerializedJSONHasNoMetricKeys is a belt-and-suspenders check:
// serialize a PostDTO to JSON and confirm none of the forbidden substrings
// appear as keys in the output.
func TestPostDTO_SerializedJSONHasNoMetricKeys(t *testing.T) {
	content := "hello world"
	dto := PostDTO{
		ID:        uuid.New().String(),
		AuthorID:  uuid.New().String(),
		Author:    PostAuthor{ID: uuid.New().String(), Handle: "testuser", DisplayName: "Test User"},
		PostType:  PostTypeOriginal,
		Content:   &content,
		IsDeleted: false,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	b, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("json.Marshal PostDTO failed: %v", err)
	}
	jsonStr := strings.ToLower(string(b))

	for _, forbidden := range forbiddenMetricSubstrings {
		if strings.Contains(jsonStr, `"`+forbidden) {
			t.Errorf("serialized PostDTO JSON contains forbidden key fragment %q — violates public metric lockdown (CLAUDE.md §2.3)", forbidden)
		}
	}
}

// ---------------------------------------------------------------------------
// FeedCursor encode / decode round-trip
// ---------------------------------------------------------------------------

// TestFeedCursor_EncodeDecodRoundTrip verifies that a cursor survives a
// round-trip through Encode → DecodeCursor without data loss.
func TestFeedCursor_EncodeDecodeRoundTrip(t *testing.T) {
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7: %v", err)
	}
	ts := time.Now().UTC().Truncate(time.Second)

	original := FeedCursor{AfterID: id, Timestamp: ts}
	encoded := original.Encode()
	if encoded == "" {
		t.Fatal("Encode returned empty string")
	}

	decoded, err := DecodeCursor(encoded)
	if err != nil {
		t.Fatalf("DecodeCursor failed: %v", err)
	}
	if decoded.AfterID != original.AfterID {
		t.Errorf("AfterID round-trip: got %v, want %v", decoded.AfterID, original.AfterID)
	}
	// Timestamps are serialized at nanosecond precision; compare after UTC truncation.
	if !decoded.Timestamp.UTC().Equal(original.Timestamp.UTC()) {
		t.Errorf("Timestamp round-trip: got %v, want %v", decoded.Timestamp.UTC(), original.Timestamp.UTC())
	}
}

// TestDecodeCursor_InvalidBase64_ReturnsError ensures malformed cursor strings
// are rejected cleanly.
func TestDecodeCursor_InvalidBase64_ReturnsError(t *testing.T) {
	_, err := DecodeCursor("!!!not-base64!!!")
	if err == nil {
		t.Fatal("DecodeCursor accepted an invalid base64 string — expected an error")
	}
}

// TestDecodeCursor_ValidBase64ButInvalidJSON_ReturnsError checks that valid
// base64 encoding of non-JSON data is rejected.
func TestDecodeCursor_ValidBase64ButInvalidJSON_ReturnsError(t *testing.T) {
	import64 := "dGhpcyBpcyBub3QganNvbg" // base64url of "this is not json"
	_, err := DecodeCursor(import64)
	if err == nil {
		t.Fatal("DecodeCursor accepted valid base64 with invalid JSON payload")
	}
}

// TestToDTO_MapsAllFields verifies the ToDTO mapping function produces correct
// field values from an internal Post.
func TestToDTO_MapsAllFields(t *testing.T) {
	id := uuid.New()
	authorID := uuid.New()
	content := "test content"
	now := time.Now().UTC().Truncate(time.Second)

	p := Post{
		ID:        id,
		AuthorID:  authorID,
		PostType:  PostTypeOriginal,
		Content:   &content,
		IsDeleted: false,
		CreatedAt: now,
		UpdatedAt: now,
		Author: PostAuthor{
			ID:          authorID.String(),
			Handle:      "testhandle",
			DisplayName: "Test Display",
		},
	}

	dto := ToDTO(p)

	if dto.ID != id.String() {
		t.Errorf("ID: got %q, want %q", dto.ID, id.String())
	}
	if dto.AuthorID != authorID.String() {
		t.Errorf("AuthorID: got %q, want %q", dto.AuthorID, authorID.String())
	}
	if dto.PostType != PostTypeOriginal {
		t.Errorf("PostType: got %q, want %q", dto.PostType, PostTypeOriginal)
	}
	if dto.Content == nil || *dto.Content != content {
		t.Errorf("Content: got %v, want %q", dto.Content, content)
	}
	if !strings.HasSuffix(dto.CreatedAt, "Z") {
		t.Errorf("CreatedAt %q does not end with Z — must be UTC RFC3339", dto.CreatedAt)
	}
}
