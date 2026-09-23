# ADR 0006 — Feed Pagination and Hard Termination

**Status:** ACCEPTED

Amended 2026-09-23: page size vs. max depth, bounded-window enforcement, and termination scope resolved (d2163e4, 67907f7).

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

Feed endpoints return a flat page:

```json
{
  "items": [],
  "next_cursor": "",
  "terminated": true
}
```

- `next_cursor` is the empty string `""` when `terminated` is `true`. It is never `null`.
- There is no `has_more` field.
- `terminated` is `true` when the current depth window is exhausted — `max_depth` has been
  reached or no further eligible items exist.

### Hard termination

The server enforces a page size and a hard maximum depth (`max_depth`) per feed. These are
implementation constants (`internal/feed/service.go`, `internal/post/service.go`), not
runtime configuration:

| Feed | Page size | Hard maximum depth |
|---|---|---|
| Home | 50 | 200 |
| Author | 50 | 200 |
| Hashtag | 50 | 200 |
| Thread | 25 | 100 |

Inner Circle and Discovery feeds remain deferred and are not implemented.

Each query first selects the current top-`max_depth` eligible rows, after all visibility
filters and in the feed's ordering. The cursor is then applied within that bounded window
and `page_size` rows are returned. A cursor can never widen the window.

A syntactically valid cursor outside the current window returns `items: []`,
`next_cursor: ""`, `terminated: true`.

### Termination scope

Resolved (2026-09-23, 67907f7): termination applies per cursor chain over the current
top-`max_depth` window. A request without a cursor starts a new chain from the current
window. There is no server-side session state or counter.

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
- The Go feed layer must accept and validate cursor values. Malformed cursors are rejected
  with `VALIDATION_ERROR` using each endpoint's existing status mapping: HTTP 400 for the
  author, thread, and hashtag feeds; HTTP 422 for the home feed. A syntactically valid
  cursor outside the current window is not rejected; it returns an empty, terminated page.
- The Flutter feed BLoC must be designed from Phase 3 with `FeedTerminatedState` as a
  first-class lifecycle state, not an afterthought.
- Phase 1 and Phase 2 implementation must use the `{items, next_cursor, terminated}` shape
  in any collection responses (even non-feed lists) for consistency.

## Validation / follow-up

- Go tests must verify: page size enforcement, cursor stability, and `terminated: true`
  trigger at the correct item count.
- Flutter BLoC tests must verify: the transition to `FeedTerminatedState` when
  `terminated: true` is received, and that no further requests are issued after termination.
- Termination scope is resolved; see "Termination scope" above.
