// Package title implements the title domain: title definitions, user title
// instances, lifecycle status management, and owner-only DTOs.
//
// Titles are earned through qualification (Phase 4) and displayed as public
// badges in post author blocks (Phase 8). The TitleSummaryDTO is the only
// title type exposed on public-facing DTOs; internal qualification state and
// notification flags are never serialized into any public response.
//
// Per CLAUDE.md §2.3, no social-validation metrics are present in any DTO.
package title

import (
	"time"

	"github.com/google/uuid"
)

// TitleCategory represents the category of a title definition.
// Values match the title_definitions_category_values check constraint
// in migration 0014.
type TitleCategory string

const (
	CategoryMilestone   TitleCategory = "milestone"
	CategoryNiche       TitleCategory = "niche"
	CategoryPerformance TitleCategory = "performance"
)

// TitleStatus represents the lifecycle status of a user_titles row.
// Values match the user_titles_status_values check constraint in migration 0015.
// Lifecycle: active → grace_period → revoked.
type TitleStatus string

const (
	StatusActive      TitleStatus = "active"
	StatusGracePeriod TitleStatus = "grace_period"
	StatusRevoked     TitleStatus = "revoked"
)

// TitleDefinition is the internal representation of a title_definitions row.
// It must never be serialized directly into an API response.
// Use TitleDefinitionDTO for public responses.
type TitleDefinition struct {
	ID          uuid.UUID
	Slug        string
	DisplayName string
	Description *string
	Category    TitleCategory
	IsRevocable bool
	IsActive    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// UserTitle is the internal representation of a user_titles row.
// It must never be serialized directly into an API response.
// Use UserTitleDTO for owner-only responses.
type UserTitle struct {
	ID                     uuid.UUID
	UserID                 uuid.UUID
	TitleDefinitionID      uuid.UUID
	Status                 TitleStatus
	GracePeriodEndsAt      *time.Time
	GraceNotificationSent  bool
	UnlockNotificationSent bool
	RevokedAt              *time.Time
	UnlockedAt             time.Time
	CreatedAt              time.Time
}

// TitleSummary is the minimal internal representation of a title shown in
// public post author blocks and primary title responses. The ID is retained
// for internal FK resolution; only Slug and DisplayName are surfaced to callers
// via TitleSummaryDTO.
type TitleSummary struct {
	ID          uuid.UUID
	Slug        string
	DisplayName string
}

// UserTitleEntry is returned by Repository.GetUserTitles. It combines a
// UserTitle row with the joined title definition display fields (Slug,
// DisplayName, Category, IsRevocable) needed to build UserTitleDTO responses
// without N+1 queries in the service layer. Exported so service and
// integration-test code outside this package can consume it.
type UserTitleEntry struct {
	UserTitle
	Slug        string
	DisplayName string
	Category    TitleCategory
	IsRevocable bool
}

// TitleStatusUpdate carries the fields to apply to a user_titles row via
// UpdateUserTitleStatus. Status is always required. Nil pointer fields are
// left unchanged by the database (via COALESCE). Used by the qualification
// worker (Phase 4) and notification worker (Phase 7).
type TitleStatusUpdate struct {
	// Status is required — always pass the intended new (or current) status.
	Status TitleStatus
	// GracePeriodEndsAt: set when transitioning to grace_period. Nil = keep existing.
	GracePeriodEndsAt *time.Time
	// RevokedAt: set when transitioning to revoked. Nil = keep existing.
	RevokedAt *time.Time
	// GraceNotificationSent: set to non-nil true to mark sent. Nil = keep existing.
	GraceNotificationSent *bool
	// UnlockNotificationSent: set to non-nil true to mark sent. Nil = keep existing.
	UnlockNotificationSent *bool
}

// ---------------------------------------------------------------------------
// Public DTOs
// ---------------------------------------------------------------------------

// TitleSummaryDTO is the public-safe minimal title representation used in
// PostAuthor blocks (Phase 8) and PrimaryTitleResponse. Contains only the
// fields required for display.
type TitleSummaryDTO struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
}

// TitleDefinitionDTO is the public DTO for a single active title definition.
// Returned by GET /api/v1/titles (Phase 9). is_active=false definitions are
// excluded from this response by the repository.
type TitleDefinitionDTO struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category"`
	IsRevocable bool   `json:"is_revocable"`
}

// UserTitleDTO is the owner-only DTO for a single user_titles row joined with
// its title definition. Returned in UserTitlesResponse for GET /api/v1/titles/me
// (Phase 9). Revoked titles are excluded from this response by the repository.
// Per CLAUDE.md §2.3, no social-validation metric fields are present.
type UserTitleDTO struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Category    string `json:"category"`
	IsRevocable bool   `json:"is_revocable"`
	Status      string `json:"status"`
	UnlockedAt  string `json:"unlocked_at"` // ISO 8601 UTC
}

// UserTitlesResponse is the owner-only list response for GET /api/v1/titles/me.
// Contains only active and grace_period titles; revoked titles are excluded.
// PrimaryID is the user_titles.id of the currently selected primary title,
// or nil if none is set.
type UserTitlesResponse struct {
	Items     []UserTitleDTO `json:"items"`
	PrimaryID *string        `json:"primary_id,omitempty"`
}

// SetPrimaryTitleRequest is the request body for PUT /api/v1/titles/me/primary.
type SetPrimaryTitleRequest struct {
	UserTitleID string `json:"user_title_id"`
}

// PrimaryTitleResponse is the response for GET /api/v1/titles/me/primary and
// PUT /api/v1/titles/me/primary. PrimaryTitle is nil when no primary title
// is set.
type PrimaryTitleResponse struct {
	PrimaryTitle *TitleSummaryDTO `json:"primary_title"`
}
