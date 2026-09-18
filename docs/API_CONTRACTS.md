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

### Phase 2+ (not yet designed — do not implement)

| Area | Endpoint | Status | Notes |
|---|---|---|---|
| Feed | `/api/v1/feeds/inner-circle` | PROPOSED | Define only after Phase 3 architecture. |
| Feed | `/api/v1/feeds/discovery` | PROPOSED | Define only after Phase 3 architecture. |
| Posts | Post endpoints | PROPOSED | Define only after Phase 2 architecture. |
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
