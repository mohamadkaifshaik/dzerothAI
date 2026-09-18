# ADR 0004 — Identifier Strategy: UUID v7

**Status:** ACCEPTED

## Context

`docs/IDENTIFIERS_AND_TIME.md` deferred the identifier decision to the architecture planner,
requiring one consistent identifier scheme across Go, PostgreSQL, and Flutter. No existing
database schema or model constrains the choice; this is a greenfield decision.

Requirements:
- Globally unique without coordination
- Suitable for PostgreSQL primary keys
- Compatible with Go's standard library ecosystem
- Portable to Flutter/Dart as a string

## Decision

**UUID v7** for all entity primary keys.

- Generated in the Go application layer using `github.com/google/uuid`.
  The library must be at version 1.6.0 or later, which introduced v7 support.
  UUID v7 support must be verified against the selected version before use.
- Stored in PostgreSQL as the native `uuid` column type.
- Serialized in all API responses as a lowercase hyphenated string:
  `"018f4a3c-1234-7abc-8def-000000000001"`
- Flutter/Dart models represent IDs as `String`.

**Timestamps:** All timestamp columns use `TIMESTAMPTZ` in PostgreSQL, stored in UTC.
API responses serialize timestamps as ISO 8601 with explicit Z suffix.
See `docs/IDENTIFIERS_AND_TIME.md` for the full timestamp convention.

**JWT `jti` claim:** Uses UUID v4 (standard JWT practice). It is not a foreign key to any
database table and does not need to be time-ordered.

## Alternatives considered

**UUID v4 (random).**
Random insertion causes index fragmentation in PostgreSQL B-tree indexes at scale.
Rejected in favor of v7's time-ordered prefix.

**ULID.**
Non-standard; requires an additional library; no meaningful advantage over UUID v7 for
this use case. Rejected.

**Auto-increment integer.**
Leaks entity count information. Incompatible with distributed or federated contexts.
Rejected.

**Database-generated UUIDs (`gen_random_uuid()`).**
Moves ID generation responsibility into PostgreSQL, making the application unable to know
the ID before the insert. Rejected; application-generated UUIDs allow the ID to be set
before the database call, simplifying retry logic and idempotent upserts.

## Consequences

- All migration files must use the `uuid` column type for primary keys.
- Go code imports `github.com/google/uuid` and calls `uuid.New()` for v4 (JWT `jti`) and
  the appropriate v7 constructor for entity IDs once the library version is confirmed.
- The Go module must declare the correct version of `github.com/google/uuid` in `go.mod`.
- Do not introduce a second ID scheme for any new table or API.

## Validation / follow-up

- Confirm `github.com/google/uuid` v1.6+ is available and supports `uuid.NewV7()` or
  equivalent before writing the first migration or model.
- Add a test that verifies IDs are time-ordered for consecutive inserts within the same
  millisecond boundary.
