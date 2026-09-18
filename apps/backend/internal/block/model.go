// Package block implements the block and mute domain for Dzeroth.
//
// Privacy enforcement rules (CLAUDE.md §2.3 and §12):
//   - A blocked user receives a 404 (not 403) on profile lookup — the block
//     state must never be revealed to the blocked party.
//   - Both directions of a block are treated symmetrically: if A blocks B, B
//     also cannot see A's profile.
//   - Mutes suppress content in feeds without notifying the muted user.
//   - No public aggregate counts (block count, mute count) are ever exposed.
package block

import "github.com/google/uuid"

// BlockedUserIDs is an internal value type used by the home timeline feed
// filtering logic (Phase 4). It is never serialized into an API response.
type BlockedUserIDs struct {
	IDs []uuid.UUID
}

// MutedUserIDs is an internal value type used by the home timeline feed
// filtering logic (Phase 4). It is never serialized into an API response.
type MutedUserIDs struct {
	IDs []uuid.UUID
}
