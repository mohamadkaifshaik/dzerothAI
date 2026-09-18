# ADR 0006 — Feed Pagination and Hard Termination

**Status:** ACCEPTED

## Context

CLAUDE.md section 2.1 is an absolute product rule: there must be no infinite scrolling.
Feeds are finite. The backend must not provide an effectively infinite feed that the client
merely chooses not to consume. Both the server and the Flutter client must enforce this.

Feed implementation is Phase 3 work. This ADR establishes the contract that Phase 1
(foundation) and Phase 2 (posts) must be consistent with, so the envelope shape and
termination mechanism are not reinvented later.

## Decision

### Pagination strategy: cursor-based with opaque base64url cursor

Cursor-based pagination is used for all feed endpoints. Offset pagination is explicitly
rejected for feeds (see Alternatives).

The cursor is an opaque base64url-encoded JSON value containing the last-seen item's UUID
and timestamp:

```json
{ "after_id": "<uuid-v7>", "ts": "<ISO-8601-UTC>" }
```

The `ts` field provides a secondary sort anchor for stability when two items share the
same timestamp (unlikely with UUID v7 but possible). The client treats the cursor as
fully opaque. It must never construct or parse a cursor value.

### Pagination envelope

All feed and collection endpoints return:

```json
{
  "data": [],
  "pagination": {
    "next_cursor": "base64url...",
    "has_more": false,
    "terminated": true
  }
}
```

- `next_cursor` is `null` when `terminated` is `true` or when there are no more items.
- `has_more` is `false` when `terminated` is `true` or when no further items exist.
- `terminated` is `true` when the server has reached the hard feed depth limit for this request.

### Hard termination

The server enforces a maximum feed depth (`max_depth`) per feed type. Example values
(configurable, not hard-coded):

| Feed type | Phase 3 default |
|---|---|
| Home timeline | 200 items |
| Inner Circle | 150 items |
| Discovery | 100 items |

When the cursor position would exceed `max_depth`, the server returns the final page with
`terminated: true` and `next_cursor: null`. It does not return a cursor that would allow
fetching beyond the limit.

### Open decision — termination scope

Whether feed termination resets on app restart (per-session) or persists for a defined
server-side period (per-day or per-calendar-period) is not yet decided. This does not
affect Phase 1 or Phase 2. The decision must be recorded before Phase 3 implementation
begins.

### Flutter client obligations

- The feed BLoC must handle a `terminated: true` response by transitioning to
  `FeedTerminatedState`.
- The `FeedTerminatedState` renders the "Go Touch Grass" boundary UI.
- The client must not issue another page request after receiving `terminated: true`.
- There is no "load more" button or pull-to-refresh that bypasses termination.

### Public validation metrics

Feed item DTOs must not include any social-validation metrics:
- `like_count`, `impression_count`, `bookmark_count`, `follower_count`, `repost_count`,
  or any equivalent field.

These are enforced by using explicit DTO structs in Go that do not contain these fields.
The server does not omit fields from a full model — it uses a separate DTO type that never
had those fields.

## Alternatives considered

**Offset pagination.**
Rejected. Offset pagination produces inconsistent results when new content is inserted
between pages. New posts shift the offset, causing items to be skipped or repeated.
This is incompatible with a live social network feed.

**Client-only termination (no server enforcement).**
Rejected by CLAUDE.md section 2.1. The backend must not provide an effectively infinite
feed. A client-only limit can be bypassed by any API client.

**Keyset pagination without termination.**
Rejected. Keyset without a hard limit is equivalent to infinite scrolling at the API level.

## Consequences

- Every feed endpoint must implement `max_depth` enforcement and the `terminated` flag.
- The Go feed repository layer must accept and validate cursor values, rejecting malformed
  or out-of-range cursors.
- The Flutter feed BLoC must be designed from Phase 3 with `FeedTerminatedState` as a
  first-class lifecycle state, not an afterthought.
- Phase 1 and Phase 2 implementation must use the envelope shape defined here in any
  collection responses (even non-feed lists) for consistency.

## Validation / follow-up

- Go tests must verify: page size enforcement, cursor stability, and `terminated: true`
  trigger at the correct item count.
- Flutter BLoC tests must verify: the transition to `FeedTerminatedState` when
  `terminated: true` is received, and that no further requests are issued after termination.
- Before Phase 3 begins, resolve the open decision on termination scope and update this ADR.
