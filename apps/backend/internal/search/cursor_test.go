package search

import (
	"testing"
)

// TestUserCursorRoundTrip verifies that encode → decode is a lossless round-trip.
func TestUserCursorRoundTrip(t *testing.T) {
	t.Parallel()

	orig := userCursor{
		AfterHandle: "alice",
		AfterID:     "018f1234-5678-7abc-def0-123456789abc",
	}

	encoded := orig.encode()
	if encoded == "" {
		t.Fatal("encode returned empty string")
	}

	decoded, err := decodeUserCursor(encoded)
	if err != nil {
		t.Fatalf("decodeUserCursor returned error: %v", err)
	}

	if decoded.AfterHandle != orig.AfterHandle {
		t.Errorf("AfterHandle: want %q, got %q", orig.AfterHandle, decoded.AfterHandle)
	}
	if decoded.AfterID != orig.AfterID {
		t.Errorf("AfterID: want %q, got %q", orig.AfterID, decoded.AfterID)
	}
}

// TestDecodeUserCursor_InvalidBase64 verifies that a malformed cursor returns an error.
func TestDecodeUserCursor_InvalidBase64(t *testing.T) {
	t.Parallel()

	_, err := decodeUserCursor("!!!not-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64 cursor, got nil")
	}
}

// TestDecodeUserCursor_MissingFields verifies that a cursor missing required fields
// returns an error.
func TestDecodeUserCursor_MissingFields(t *testing.T) {
	t.Parallel()

	// Encode a cursor with only AfterHandle set (AfterID is empty).
	c := userCursor{AfterHandle: "bob", AfterID: ""}
	encoded := c.encode()

	_, err := decodeUserCursor(encoded)
	if err == nil {
		t.Fatal("expected error for cursor with empty AfterID, got nil")
	}
}
