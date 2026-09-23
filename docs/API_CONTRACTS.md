# Dzeroth API Contracts

This document is the API contract registry and convention guide.

## Contract status

- `IMPLEMENTED` — verified against the current backend router and handler code.
- `PROPOSED` — design approved; do not call from frontend code until `IMPLEMENTED`.
- `DEPRECATED` — still present but scheduled for removal.
- `REMOVED` — no longer valid.

## Core conventions

### Base URL

```
/api/v1/
```

Version is embedded in the path. Do not invent a second versioning scheme.

### Authentication header

```
Authorization: Bearer <jwt_access_token>
```

### Error shape

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Human-readable description of the problem.",
    "details": [
      { "field": "handle", "message": "must be 3–50 characters" }
    ]
  }
}
```

- `code` is a machine-readable uppercase snake_case constant. Flutter switches on `code`.
- `message` is human-readable and safe to display.
- `details` is present only for validation errors. It is absent for all other error types.
- Stack traces, SQL errors, internal service names, and credentials are never exposed.

### Pagination envelope (feeds and collections)

```json
{
  "data": [],
  "pagination": {
    "next_cursor": "base64url-opaque-string",
    "has_more": false,
    "terminated": true
  }
}
```

- `terminated: true` means the server has reached the hard feed boundary for this session.
- When `terminated` is `true`, `next_cursor` is `null` and `has_more` is `false`.
- The Flutter client must stop requesting and render the feed boundary UI when `terminated` is `true`.
- The cursor is an opaque base64url-encoded value. Clients must not construct or parse it.

### Single-resource response

```json
{
  "data": { }
}
```

### Additional conventions

- Inspect existing router registration before adding an endpoint.
- Search handlers, services, repositories, models, and tests before creating new ones.
- Authentication and authorization are enforced server-side.
- Request validation is enforced server-side even if Flutter validates first.
- Mutating operations must be idempotent where retries can occur.
- Public DTOs must never include social-validation metrics (likes, impressions,
  bookmark counts, follower counts, or equivalent fields).
- IDs are UUID v7, serialized as lowercase hyphenated strings.
- Timestamps are ISO 8601 with explicit Z suffix (`2026-09-18T12:00:00Z`).

---

## Contract registry

### Phase 1 — Foundation, identity, and profiles

| Area | Method | Endpoint | Status | Notes |
|---|---|---|---|---|
| Health | `GET` | `/health` | IMPLEMENTED | No auth required. Returns `{"status":"ok","db":"ok","redis":"ok"}` or `{"status":"degraded",...}`. |
| Auth | `POST` | `/api/v1/auth/register` | IMPLEMENTED | Create account. Returns access + refresh token pair. |
| Auth | `POST` | `/api/v1/auth/login` | IMPLEMENTED | Authenticate. Returns access + refresh token pair. |
| Auth | `POST` | `/api/v1/auth/refresh` | IMPLEMENTED | Rotate refresh token. Refresh token in request body. Returns new pair. |
| Auth | `POST` | `/api/v1/auth/logout` | IMPLEMENTED | Revoke current session. Requires `Authorization: Bearer`. |
| Identity | `GET` | `/api/v1/me` | IMPLEMENTED | Get own profile. Requires auth. |
| Identity | `PUT` | `/api/v1/me` | IMPLEMENTED | Update own profile (bio, display name, location, website). Requires auth. |
| Identity | `GET` | `/api/v1/me/settings` | IMPLEMENTED | Get account/privacy settings. Requires auth. |
| Identity | `PUT` | `/api/v1/me/settings` | IMPLEMENTED | Update account/privacy settings. Requires auth. |
| Identity | `PUT` | `/api/v1/me/avatar` | BLOCKED | Avatar upload (multipart). Blocked on media storage provider decision (OPEN-1). |
| Identity | `PUT` | `/api/v1/me/header` | BLOCKED | Header image upload (multipart). Blocked on media storage provider decision (OPEN-1). |
| Profiles | `GET` | `/api/v1/users/:id` | IMPLEMENTED | Get a user's public profile by UUID. Requires auth. Returns 404 when blocked (block state is never revealed). Response must not include public validation metrics. |

### Phase 2 — Posts and conversations

| Area | Method | Endpoint | Status | Notes |
|---|---|---|---|---|
| Posts | `POST` | `/api/v1/posts` | IMPLEMENTED | Auth required. Rate limited: 30 requests per 15 min per user (fail-closed Redis policy). See request/response below. |
| Posts | `GET` | `/api/v1/posts/{postId}` | IMPLEMENTED | No auth required. Returns single post. Public DTO — zero social-validation metrics. |
| Posts | `DELETE` | `/api/v1/posts/{postId}` | IMPLEMENTED | Auth required. Owner only. Soft delete (sets `is_deleted` flag). |
| Posts | `GET` | `/api/v1/posts/{postId}/thread` | IMPLEMENTED | No auth required. Paginated thread replies. Cursor-based. `terminated: true` when server-side max depth (100) is reached. |
| Posts | `GET` | `/api/v1/users/{userId}/posts` | IMPLEMENTED | No auth required. Paginated author post list. Cursor-based. `terminated: true` when server-side max (200) is reached. |

#### `POST /api/v1/posts` — Create post

**Auth:** `Authorization: Bearer <token>` required.

**Request body:**

```json
{
  "content": "Post text up to 500 Unicode code points.",
  "post_type": "original | reply | quote | repost",
  "parent_id": "<uuid-v7 | null>",
  "quoted_post_id": "<uuid-v7 | null>",
  "share_initiated_at": "<ISO 8601 UTC | absent>"
}
```

- `content`: required for `original`, `reply`, and `quote`. Must not exceed 500 Unicode code points.
- `post_type`: required. One of `original`, `reply`, `quote`, `repost`.
- `parent_id`: required for `reply`. The post being replied to.
- `quoted_post_id`: required for `quote` and `repost`. The post being quoted or reposted.
- `share_initiated_at`: **required for `quote` and `repost`; ignored if supplied for `original` and `reply`.** ISO 8601 UTC timestamp (e.g. `2026-09-22T10:00:00Z`) recording when the client began the mandatory 5-second countdown. The backend independently validates the elapsed time from this value to the request arrival time.
- For `quote` posts, `content` must contain at least 5 distinct words (Dzeroth §2.2). The five-second client countdown is a UX guard; the backend enforces both the word-count rule and the timing rule independently.
- Mentions (`@handle`) and hashtags (`#tag`) are extracted and persisted automatically.

**Response:** `201 Created` — single-resource envelope containing the created `PostDTO`.

**Key behaviors:**

- 500 Unicode code-point limit enforced at PostgreSQL (CHECK), Go service, and API validation layers.
- Five-distinct-word rule enforced server-side for `quote` post type.
- **Five-second share delay rule enforced server-side for `quote` and `repost` types (CLAUDE.md §2.2):**
  - `share_initiated_at` is required; absence returns `400 VALIDATION_ERROR` with message `"share_initiated_at is required for repost and quote posts"`.
  - A malformed timestamp fails request decoding: `400 VALIDATION_ERROR` with message `"Invalid JSON body."`.
  - Values more than 30 seconds in the future are rejected (clock-skew tolerance for NTP drift): `400 VALIDATION_ERROR` with message `"share_initiated_at is too far in the future"`.
  - If `time.Since(share_initiated_at) < 5s` the request is rejected: `400 VALIDATION_ERROR` with message `"share action must be initiated at least 5 seconds before submission"`.
  - For `original` and `reply` types the field is not validated; a supplied value is ignored.
  - Do NOT add a client boolean flag to bypass this check. The backend enforces it.
- Response `PostDTO` contains zero public social-validation metrics (no likes, impressions, bookmark counts, or follower counts).
- Rate limit exceeded returns `429 Too Many Requests` with standard error shape.

#### `GET /api/v1/posts/{postId}` — Get single post

**Auth:** none required.

**Response:** `200 OK` — single-resource envelope containing `PostDTO`.

**PostDTO shape:**

```json
{
  "id": "<uuid-v7>",
  "content": "Post text.",
  "post_type": "original | reply | quote | repost",
  "author": {
    "id": "<uuid-v7>",
    "handle": "username",
    "display_name": "Display Name"
  },
  "parent_id": "<uuid-v7 | null>",
  "quoted_post_id": "<uuid-v7 | null>",
  "thread_root_id": "<uuid-v7 | null>",
  "created_at": "2026-09-18T12:00:00Z",
  "is_deleted": false
}
```

- No likes, impressions, bookmark counts, follower counts, or equivalent metrics are present in this DTO.
- Authenticated callers receive an additional `viewer_has_reacted: bool` field. The field is absent for unauthenticated requests.

#### `DELETE /api/v1/posts/{postId}` — Soft delete post

**Auth:** `Authorization: Bearer <token>` required. Caller must own the post.

**Response:** `204 No Content`.

**Key behaviors:**

- Sets `is_deleted = true` on the post row; does not physically remove the record.
- Returns `403 Forbidden` if the authenticated user does not own the post.
- Returns `404 Not Found` if the post does not exist or is already deleted.

#### `GET /api/v1/posts/{postId}/thread` — Get thread replies

**Auth:** none required.

**Query parameters:**

- `cursor` (optional): opaque base64url cursor for pagination.

**Response:** `200 OK` — pagination envelope.

```json
{
  "data": [ /* array of PostDTO */ ],
  "pagination": {
    "next_cursor": "<base64url | null>",
    "has_more": false,
    "terminated": true
  }
}
```

**Key behaviors:**

- `terminated: true` when server-side max depth (100 replies) is reached. Flutter client must stop requesting and display the feed boundary UI.
- Finite feed — no infinite scrolling.

#### `GET /api/v1/users/{userId}/posts` — Get author post list

**Auth:** none required.

**Query parameters:**

- `cursor` (optional): opaque base64url cursor for pagination.

**Response:** `200 OK` — pagination envelope (same shape as thread endpoint above).

**Key behaviors:**

- `terminated: true` when server-side max (200 posts) is reached. Flutter client must stop requesting and display the feed boundary UI.
- Finite feed — no infinite scrolling.
- Response items are `PostDTO`; no public social-validation metrics.

### Phase 3 — Feeds, follows, block/mute, bookmarks

| Area | Method | Endpoint | Status | Notes |
|---|---|---|---|---|
| Follow | `POST` | `/api/v1/users/{userID}/follow` | IMPLEMENTED | Auth required. Idempotent. Rate limited: 60/15min per user (fail-closed). |
| Follow | `DELETE` | `/api/v1/users/{userID}/follow` | IMPLEMENTED | Auth required. No-op if not following. |
| Follow | `GET` | `/api/v1/users/{userID}/following` | IMPLEMENTED | Auth required. Cursor-paginated. `terminated: true` at 200 items. |
| Follow | `GET` | `/api/v1/users/{userID}/followers` | IMPLEMENTED | Auth required. Cursor-paginated. `terminated: true` at 200 items. |
| Block | `POST` | `/api/v1/users/{userID}/block` | IMPLEMENTED | Auth required. Idempotent. Atomically removes follows in both directions. Rate limited: 30/15min. Returns 204. |
| Block | `DELETE` | `/api/v1/users/{userID}/block` | IMPLEMENTED | Auth required. No-op if not blocked. Returns 204. |
| Mute | `POST` | `/api/v1/users/{userID}/mute` | IMPLEMENTED | Auth required. Idempotent. Rate limited: shared with block limit. Returns 204. |
| Mute | `DELETE` | `/api/v1/users/{userID}/mute` | IMPLEMENTED | Auth required. No-op if not muted. Returns 204. |
| Feed | `GET` | `/api/v1/feeds/home` | IMPLEMENTED | Auth required. Follower-based DB-first JOIN. Cursor-paginated. `terminated: true` at 200 items. Filters deleted, blocked, muted, private-account posts (server-side). |
| Bookmark | `POST` | `/api/v1/posts/{postID}/bookmark` | IMPLEMENTED | Auth required. Idempotent. Returns 204. Rate limited: 120/15min. |
| Bookmark | `DELETE` | `/api/v1/posts/{postID}/bookmark` | IMPLEMENTED | Auth required. No-op if not bookmarked. Returns 204. |
| Bookmark | `GET` | `/api/v1/me/bookmarks` | IMPLEMENTED | Auth required. Owner-only (JWT callerID only — no path param). Cursor-paginated. `terminated: true` at 200 items. Soft-deleted posts excluded. |

#### `GET /api/v1/feeds/home` — Home timeline

**Auth:** `Authorization: Bearer <token>` required.

**Query parameters:**

- `cursor` (optional): opaque base64url cursor for pagination.

**Response:** `200 OK`

```json
{
  "items": [ /* array of PostDTO */ ],
  "next_cursor": "<base64url | null>",
  "terminated": true
}
```

**Key behaviors:**

- `terminated: true` at server-enforced maximum of 200 posts per session. Flutter client must stop requesting and display the "Go Touch Grass" boundary UX.
- Soft-deleted posts (`is_deleted = TRUE`) are excluded.
- Posts from blocked users (in either direction) are excluded.
- Posts from muted users are excluded.
- Posts from private-account users the caller does not follow are excluded.
- Termination is per-session (resets on new client session). No server-side daily counter.

#### `GET /api/v1/users/{userID}/following` and `GET /api/v1/users/{userID}/followers`

**Auth:** `Authorization: Bearer <token>` required.

**Response shape:**

```json
{
  "items": [
    { "id": "<uuid-v7>", "handle": "username", "display_name": "Display Name", "avatar_url": null }
  ],
  "next_cursor": "<base64url | null>",
  "terminated": true
}
```

- Zero follower/following count fields — public aggregate counts are permanently forbidden (CLAUDE.md §2.3).

#### `GET /api/v1/me/bookmarks` — Bookmark list

**Auth:** `Authorization: Bearer <token>` required.

**Response shape:**

```json
{
  "items": [
    { "post_id": "<uuid-v7>", "created_at": "2026-09-18T12:00:00Z", "post": { /* PostDTO */ } }
  ],
  "next_cursor": "<base64url | null>",
  "terminated": true
}
```

- Owner-only. `BookmarkDTO` is never nested inside `PostDTO`. No `bookmark_count` field exists anywhere.

### Phase 4 — Reactions, notifications, search

| Area | Method | Endpoint | Status | Notes |
|---|---|---|---|---|
| Reactions | `POST` | `/api/v1/posts/{postID}/react` | IMPLEMENTED | Auth required. Idempotent. Returns 204. Rate limited: 120/15min (fail-open). |
| Reactions | `DELETE` | `/api/v1/posts/{postID}/react` | IMPLEMENTED | Auth required. No-op if not reacted. Returns 204. |
| Notifications | `GET` | `/api/v1/me/notifications` | IMPLEMENTED | Auth required. Owner-only. Cursor-paginated. `terminated: true` at 100 items. |
| Notifications | `PUT` | `/api/v1/me/notifications/read` | IMPLEMENTED | Auth required. Marks all caller's notifications read. Returns 204. |
| Search | `GET` | `/api/v1/search/posts?q=` | IMPLEMENTED | Auth optional. Cursor-paginated. `terminated: true` at 50 items. Empty `q` returns 400. |
| Search | `GET` | `/api/v1/search/users?q=` | IMPLEMENTED | Auth optional. Cursor-paginated. `terminated: true` at 50 items. Empty `q` returns 400. |

**Notes on `GET /api/v1/posts/{postID}`:** when the caller is authenticated, the response includes a `viewer_has_reacted: bool` field. For unauthenticated callers the field is absent. All other public DTO constraints (zero social-validation metrics) remain in force.

### Phase 5 — Safety and creator functionality

| Area | Method | Endpoint | Status | Notes |
|---|---|---|---|---|
| Reports | `POST` | `/api/v1/posts/{postId}/report` | IMPLEMENTED | Auth required. Idempotent (duplicate pending → 204 no-op). Rate limited: 10/15min fail-closed. Self-report returns 400. Block relationships do not prevent reporting. |
| Reports | `POST` | `/api/v1/users/{userId}/report` | IMPLEMENTED | Auth required. Same rules as post report. |
| Studio | `GET` | `/api/v1/me/studio/analytics` | IMPLEMENTED | Auth required. Owner-scoped (JWT callerID only). Cursor-paginated. `terminated: true` at 50 items. Response: `PostAnalytics` items — private aggregate counts only (reactions, bookmarks, replies, quotes). Zero public metrics. |
| Identity | `DELETE` | `/api/v1/me/account` | IMPLEMENTED | Auth required. Soft-suspends account (`is_suspended=TRUE`). Revokes all refresh tokens immediately. Returns 204. Idempotent. |

#### `POST /api/v1/posts/{postId}/report` and `POST /api/v1/users/{userId}/report` — Submit report

**Auth:** `Authorization: Bearer <token>` required.

**Request body:**

```json
{
  "reason": "spam | harassment | misinformation | hate_speech | violence | other",
  "detail": "Optional free-text up to 500 characters."
}
```

**Response:** `204 No Content`.

**Key behaviors:**

- Reporter identity is stored server-side but never returned in any response.
- Duplicate pending report from same reporter → `ON CONFLICT DO NOTHING` → 204 (idempotent).
- Self-report → `400 VALIDATION_ERROR`.
- Block relationships do not prevent reporting.
- Rate limit exceeded → `429 Too Many Requests`.

#### `GET /api/v1/me/studio/analytics` — Creator Studio analytics

**Auth:** `Authorization: Bearer <token>` required.

**Query parameters:**

- `cursor` (optional): opaque base64url cursor.

**Response:** `200 OK`

```json
{
  "items": [
    {
      "post_id": "<uuid-v7>",
      "content": "Post text.",
      "post_type": "original | reply | quote | repost",
      "created_at": "2026-09-18T12:00:00Z",
      "reaction_count": 0,
      "bookmark_count": 0,
      "reply_count": 0,
      "quote_count": 0
    }
  ],
  "next_cursor": "<base64url | null>",
  "terminated": true
}
```

**Key behaviors:**

- Scoped to caller only — no path param accepted.
- `terminated: true` at server-enforced max of 50 posts. Flutter renders `GoTouchGrassWidget`.
- All count fields are private analytics. They must never appear in any public DTO.
- Soft-deleted posts excluded.

### Phase 6 — Topics/Trends (hashtag browsing)

| Area | Method | Path | Status | Notes |
|---|---|---|---|---|
| Topics | `GET` | `/api/v1/hashtags/{tag}/posts` | IMPLEMENTED | Auth optional. Cursor-paginated. `terminated: true` at 200 posts. Block filtering applied for authenticated callers. Tag normalized server-side (lowercase, `#` stripped). |

#### `GET /api/v1/hashtags/{tag}/posts` — Hashtag feed

**Auth:** optional (Bearer token). When authenticated, posts by blocked users are excluded.

**Path param:** `tag` — hashtag without `#` (e.g. `golang`, not `#golang`).

**Query params:** `cursor` (opaque base64url cursor from previous response).

**Response:**
```json
{
  "items": [ /* PostDTO array — zero public metrics */ ],
  "next_cursor": "<opaque>",
  "terminated": true
}
```

- `terminated: true` at server-enforced max of 200 posts. Flutter renders `GoTouchGrassWidget`.
- Returns `400 VALIDATION_ERROR` for malformed tag format.
- Soft-deleted posts excluded.

### Phase 9 — Title System HTTP API

| Area | Method | Endpoint | Status | Notes |
|---|---|---|---|---|
| Titles | `GET` | `/api/v1/titles/catalog` | IMPLEMENTED | No auth required. Returns active title definitions. `top_1pct_creator` excluded (`is_active=false`). |
| Titles | `GET` | `/api/v1/titles/me` | IMPLEMENTED | JWT required. Returns caller's active+grace_period titles and primary_title_id. Revoked titles excluded. |
| Titles | `GET` | `/api/v1/titles/me/primary` | IMPLEMENTED | JWT required. Returns caller's current primary title or `null`. |
| Titles | `PUT` | `/api/v1/titles/me/primary` | IMPLEMENTED | JWT required. Sets caller's primary title. Returns 403 if title not owned or not active/grace_period. |
| Titles | `DELETE` | `/api/v1/titles/me/primary` | IMPLEMENTED | JWT required. Clears caller's primary title. Idempotent — 204 even if no primary is set. |
| Titles | `GET` | `/api/v1/titles/{userID}/primary` | IMPLEMENTED | Auth optional. Privacy-aware: private accounts return `null` for unauthenticated callers and non-followers. Returns 400 for invalid UUID. |

#### `GET /api/v1/titles/catalog` — Title catalog

**Auth:** none required.

**Response:** `200 OK`

```json
{
  "items": [
    {
      "id": "<uuid-v7>",
      "slug": "founding_member",
      "display_name": "Founding Member",
      "description": "Optional description text.",
      "category": "milestone | niche | performance",
      "is_revocable": false
    }
  ]
}
```

- Returns only definitions where `is_active = true`. `top_1pct_creator` is excluded.
- `description` is omitted when empty (`omitempty`).
- No social-validation metrics in this or any title DTO.

#### `GET /api/v1/titles/me` — My titles

**Auth:** `Authorization: Bearer <token>` required.

**Response:** `200 OK`

```json
{
  "items": [
    {
      "id": "<user-title-uuid>",
      "slug": "founding_member",
      "display_name": "Founding Member",
      "category": "milestone",
      "is_revocable": false,
      "status": "active | grace_period",
      "unlocked_at": "2026-09-18T12:00:00Z"
    }
  ],
  "primary_id": "<user-title-uuid>"
}
```

- Revoked titles are excluded.
- `primary_id` is absent from the JSON object (not `null`) when no primary is set (`omitempty`).

#### `GET /api/v1/titles/me/primary` — My primary title

**Auth:** `Authorization: Bearer <token>` required.

**Response:** `200 OK`

```json
{
  "primary_title": {
    "slug": "founding_member",
    "display_name": "Founding Member"
  }
}
```

- `primary_title` is `null` when no primary title is set.

#### `PUT /api/v1/titles/me/primary` — Set primary title

**Auth:** `Authorization: Bearer <token>` required.

**Request body:**

```json
{
  "user_title_id": "<user-title-uuid>"
}
```

- `user_title_id` must be a valid UUID.
- The referenced `user_titles` row must be owned by the caller and have `status IN ('active', 'grace_period')`.

**Response:** `200 OK` — same shape as `GET /titles/me/primary`, populated with the new primary.

**Error responses:**

- `400 VALIDATION_ERROR` — `user_title_id` is missing or not a valid UUID.
- `403 FORBIDDEN` — title does not belong to the caller, or has status `revoked`.

#### `DELETE /api/v1/titles/me/primary` — Clear primary title

**Auth:** `Authorization: Bearer <token>` required.

**Response:** `204 No Content`.

- Idempotent: returns `204` even if no primary title is currently set.

#### `GET /api/v1/titles/{userID}/primary` — Get a user's primary title

**Auth:** optional (Bearer token). Privacy rules apply based on caller identity.

**Path param:** `userID` — target user UUID.

**Response:** `200 OK` — same shape as `GET /titles/me/primary`.

**Privacy rules (enforced server-side):**

- Owner (`callerID == targetID`): always returns the title.
- Public account: returns the title for any caller (authenticated or not).
- Private account + unauthenticated caller: returns `{"primary_title": null}` — NOT 403/404.
- Private account + authenticated non-follower: returns `{"primary_title": null}`.
- Private account + authenticated follower: returns the title.

**Error responses:**

- `400 VALIDATION_ERROR` — `userID` is not a valid UUID.

---

### Phase 5+ / deferred (not yet designed — do not implement)

| Area | Endpoint | Status | Notes |
|---|---|---|---|
| Feed | `/api/v1/feeds/inner-circle` | DEFERRED | Requires separate architecture design. |
| Feed | `/api/v1/feeds/discovery` | DEFERRED | Requires content ranking logic. |
| Ranking | Redis ZSET + weekly_titles | DEFERRED | Phase 6 ranking deferred to dedicated future feature. |

---

## Contract change protocol

For any endpoint change:

1. Find the current route registration.
2. Find its handler/service/repository path.
3. Find all frontend consumers.
4. Find tests.
5. Determine backward-compatibility impact.
6. Update this registry.
7. Update tests.
8. Review security and authorization.
9. Review the diff for duplicate or orphaned endpoints.

Never create a second endpoint because an existing endpoint was hard to find.
