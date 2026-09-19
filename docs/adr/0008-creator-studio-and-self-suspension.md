# ADR 0008 — Creator Studio Analytics and Self-Suspension

**Status:** ACCEPTED

## Context

Phase 5 adds two owner-only features that required architectural decisions:

1. **Creator Studio analytics**: What data is exposed, at what granularity, and how is
   the owner-only access enforced?
2. **Self-suspension**: What happens to existing sessions, and is the account
   permanently deleted or just suspended?

## Decision

### Creator Studio analytics scope

Creator Studio exposes **current-state aggregate counts only**:

| Field | Source |
|---|---|
| `reaction_count` | `COUNT(*)` from `reactions` WHERE `post_id` |
| `bookmark_count` | `COUNT(*)` from `bookmarks` WHERE `post_id` |
| `reply_count` | `COUNT(*)` from `posts` WHERE `parent_post_id` AND `post_type='reply'` |
| `quote_count` | `COUNT(*)` from `posts` WHERE `quoted_post_id` AND `post_type='quote'` |

**No historical data, time-series, or trend lines are stored or returned in Phase 5.**
Trend analysis and growth metrics are deferred to a future analytics phase.

The `PostAnalytics` struct is marked PRIVATE in code comments and documentation.
It must never appear in any public DTO or unauthenticated response.

The endpoint (`GET /api/v1/me/studio/analytics`) is authenticated-only.
The JWT `callerID` is the exclusive scope parameter — no path param is accepted.
This ensures the endpoint cannot be called for another user's analytics.

The response is cursor-paginated and terminates at 50 items per CLAUDE.md §2.1
(no infinite scrolling). The `GoTouchGrassWidget` is rendered at termination.

### Self-suspension design

Self-suspension (`DELETE /api/v1/me/account`) **suspends** the account — it does not
physically delete records.

- `users.is_suspended = TRUE` is set.
- `auth.Service.RevokeAllSessions` is called immediately to revoke all refresh tokens.
- Existing JWT access tokens remain valid for up to ~15 minutes until natural expiry.
  This is accepted: changing the JWT architecture (e.g. adding a token blocklist) to
  shorten this window is a Phase 7 concern.
- `auth.Service.Login` and `auth.Service.Refresh` both check `is_suspended`; a
  suspended account cannot obtain new tokens.

The Flutter `SettingsScreen` navigates to `/auth/login` immediately upon receiving
`SettingsAccountSuspended` from the BLoC, providing a clean UX transition even though
the access token may not be immediately revoked.

The endpoint is idempotent: calling it on an already-suspended account is a no-op
(204) rather than an error.

## Consequences

- Creator Studio analytics do not expose historical data — future trend analysis
  requires new schema design.
- Suspended users retain their data in PostgreSQL. Future admin tooling (Phase 7)
  can implement physical deletion or restoration.
- The ~15 minute access token window after self-suspension is a known security
  trade-off, accepted for Phase 5.
