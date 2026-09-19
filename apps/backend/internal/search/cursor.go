package search

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// userCursor is the decoded form of the opaque pagination cursor for user search results.
// Cursor format: base64url-encoded JSON {"after_handle":"<handle>","after_id":"<uuid-string>"}.
// Ordering is by (handle ASC, id ASC) to match the user search ORDER BY clause.
type userCursor struct {
	AfterHandle string `json:"after_handle"`
	AfterID     string `json:"after_id"`
}

// encode serializes the cursor to the opaque base64url string sent to clients.
func (c userCursor) encode() string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodeUserCursor parses the opaque cursor string into a userCursor.
// Returns an error if the string is malformed or contains invalid values.
func decodeUserCursor(s string) (*userCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("search: decode user cursor base64: %w", err)
	}
	var c userCursor
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("search: decode user cursor json: %w", err)
	}
	if c.AfterHandle == "" || c.AfterID == "" {
		return nil, fmt.Errorf("search: decode user cursor: missing required fields")
	}
	return &c, nil
}
