// Package user implements the user profile domain: internal model, public DTOs,
// and mapping functions. Per CLAUDE.md section 2.3, public DTOs must never include
// social-validation metrics (follower counts, like counts, impression counts, etc.).
package user

import (
	"time"

	"github.com/google/uuid"
)

// User is the internal representation of a users row.
// It must never be serialized directly into an API response.
// Use ToPublicProfile or ToOwnProfile to produce client-safe DTOs.
type User struct {
	ID            uuid.UUID
	Handle        string
	DisplayName   string
	Email         string
	EmailVerified bool
	Bio           *string
	AvatarURL     *string
	HeaderURL     *string
	Location      *string
	WebsiteURL    *string
	IsPrivate     bool
	IsSuspended   bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// PublicProfile is the DTO sent to any authenticated caller requesting another user's profile.
//
// METRIC LOCKDOWN (CLAUDE.md §2.3): this struct intentionally has no follower_count,
// following_count, like_count, post_count, impression_count, bookmark_count, or any
// equivalent social-validation field. Do not add such fields.
type PublicProfile struct {
	ID          string  `json:"id"`
	Handle      string  `json:"handle"`
	DisplayName string  `json:"display_name"`
	Bio         *string `json:"bio"`
	AvatarURL   *string `json:"avatar_url"`
	HeaderURL   *string `json:"header_url"`
	Location    *string `json:"location"`
	WebsiteURL  *string `json:"website_url"`
	IsPrivate   bool    `json:"is_private"`
	// JoinedAt is the ISO 8601 UTC timestamp of account creation.
	JoinedAt string `json:"joined_at"`
}

// OwnProfile extends PublicProfile with private fields visible only to the account owner.
// It is returned exclusively on GET /api/v1/me. It must not be returned for other users.
type OwnProfile struct {
	PublicProfile
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
}

// ToPublicProfile maps a User to a PublicProfile DTO.
// The caller is responsible for ensuring authorization before calling this function.
func ToPublicProfile(u *User) *PublicProfile {
	return &PublicProfile{
		ID:          u.ID.String(),
		Handle:      u.Handle,
		DisplayName: u.DisplayName,
		Bio:         u.Bio,
		AvatarURL:   u.AvatarURL,
		HeaderURL:   u.HeaderURL,
		Location:    u.Location,
		WebsiteURL:  u.WebsiteURL,
		IsPrivate:   u.IsPrivate,
		JoinedAt:    u.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// ToOwnProfile maps a User to an OwnProfile DTO.
// Must only be called for the authenticated user's own profile.
func ToOwnProfile(u *User) *OwnProfile {
	return &OwnProfile{
		PublicProfile: *ToPublicProfile(u),
		Email:         u.Email,
		EmailVerified: u.EmailVerified,
	}
}
