// Package reaction implements the reaction domain for Dzeroth: per-user
// like/reaction toggle on posts with idempotent React/Unreact semantics.
//
// Per CLAUDE.md §2.3, no reaction counts are exposed in any public DTO.
// React and Unreact return HTTP 204 with no response body.
//
// The ReactionChecker interface is defined here so that internal/post can
// import and use it to surface viewer_has_reacted without creating a circular
// dependency (reaction → post → reaction).
package reaction

import (
	"context"

	"github.com/google/uuid"
)

// ReactionChecker allows other packages (e.g. internal/post) to query whether
// a given user has reacted to a post without importing the reaction package's
// concrete types or creating a circular dependency.
//
// *Service implements this interface.
type ReactionChecker interface {
	HasReacted(ctx context.Context, userID, postID uuid.UUID) (bool, error)
}

// PostAuthorLookup allows the reaction handler to resolve a post's author ID
// without importing internal/post directly, avoiding a circular dependency.
// post.Service satisfies this interface via its GetPost method wrapper wired
// in cmd/api/main.go.
type PostAuthorLookup interface {
	GetPostAuthorID(ctx context.Context, postID uuid.UUID) (uuid.UUID, error)
}
