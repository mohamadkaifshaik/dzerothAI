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
| Health | `GET` | `/health` | PROPOSED | No auth required. Returns `{"status":"ok","db":"ok","redis":"ok"}` or `{"status":"degraded",...}`. |
| Auth | `POST` | `/api/v1/auth/register` | PROPOSED | Create account. Returns access + refresh token pair. |
| Auth | `POST` | `/api/v1/auth/login` | PROPOSED | Authenticate. Returns access + refresh token pair. |
| Auth | `POST` | `/api/v1/auth/refresh` | PROPOSED | Rotate refresh token. Refresh token in request body. Returns new pair. |
| Auth | `POST` | `/api/v1/auth/logout` | PROPOSED | Revoke current session. Requires `Authorization: Bearer`. |
| Identity | `GET` | `/api/v1/me` | PROPOSED | Get own profile. Requires auth. |
| Identity | `PUT` | `/api/v1/me` | PROPOSED | Update own profile (bio, display name, location, website). Requires auth. |
| Identity | `GET` | `/api/v1/me/settings` | PROPOSED | Get account/privacy settings. Requires auth. |
| Identity | `PUT` | `/api/v1/me/settings` | PROPOSED | Update account/privacy settings. Requires auth. |
| Identity | `PUT` | `/api/v1/me/avatar` | PROPOSED — BLOCKED | Avatar upload (multipart). Blocked on media storage provider decision (OPEN-1). |
| Identity | `PUT` | `/api/v1/me/header` | PROPOSED — BLOCKED | Header image upload (multipart). Blocked on media storage provider decision (OPEN-1). |
| Profiles | `GET` | `/api/v1/users/:id` | PROPOSED | Get a user's public profile by UUID. Requires auth. Response must not include public validation metrics. |

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
  "parent_post_id": "<uuid-v7 | null>",
  "quoted_post_id": "<uuid-v7 | null>"
}
```

- `content`: required for `original`, `reply`, and `quote`. Must not exceed 500 Unicode code points.
- `post_type`: required. One of `original`, `reply`, `quote`, `repost`.
- `parent_post_id`: required for `reply`. The post being replied to.
- `quoted_post_id`: required for `quote`. The post being quoted.
- For `quote` posts, `content` must contain at least 5 distinct words (Dzeroth §2.2). The five-second client countdown is a UI concern; the backend enforces the word-count rule independently.
- Mentions (`@handle`) and hashtags (`#tag`) are extracted and persisted automatically.

**Response:** `201 Created` — single-resource envelope containing the created `PostDTO`.

**Key behaviors:**

- 500 Unicode code-point limit enforced at PostgreSQL (CHECK), Go service, and API validation layers.
- Five-distinct-word rule enforced server-side for `quote` post type.
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
  "parent_post_id": "<uuid-v7 | null>",
  "quoted_post_id": "<uuid-v7 | null>",
  "thread_root_id": "<uuid-v7 | null>",
  "created_at": "2026-09-18T12:00:00Z",
  "is_deleted": false
}
```

- No likes, impressions, bookmark counts, follower counts, or equivalent metrics are present in this DTO.

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

### Phase 3+ (not yet designed — do not implement)

| Area | Endpoint | Status | Notes |
|---|---|---|---|
| Feed | `/api/v1/feeds/inner-circle` | PROPOSED | Define only after Phase 3 architecture. |
| Feed | `/api/v1/feeds/discovery` | PROPOSED | Define only after Phase 3 architecture. |
| Search | Search endpoints | PROPOSED | Define only after Phase 4 architecture. |
| Notifications | Notification endpoints | PROPOSED | Define only after Phase 4 architecture. |

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
