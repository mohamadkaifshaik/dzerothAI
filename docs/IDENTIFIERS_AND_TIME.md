# Identifier and Time Conventions

**Status:** `DECIDED — see ADR 0004`

The project uses one consistent identifier strategy and one consistent time representation
across Go, PostgreSQL, and Flutter/Dart.

## Identifiers

**Strategy: UUID v7**

- All entity primary keys use UUID v7.
- UUID v7 is time-ordered, which avoids random B-tree page splits in PostgreSQL indexes
  and supports stable sort ordering for feeds and timelines.
- Generated in the Go application layer using `github.com/google/uuid` (v1.6+ supports v7).
- Stored in PostgreSQL as the native `uuid` column type.
- Serialized in API responses as a lowercase hyphenated string:
  `"018f4a3c-1234-7abc-8def-000000000001"`
- Flutter/Dart models use `String` for IDs received from the API.
- Application code must verify UUID v7 support in the selected library version before use.

**Rules:**
- Do not introduce a second ID scheme for new tables or APIs.
- Do not use auto-increment integers (leaks entity count; not suitable for distributed contexts).
- Do not use UUID v4 (random insertion causes index fragmentation at scale).
- The `jti` claim in JWTs is a separate UUID v4 (standard JWT practice); it is not a
  foreign key to any database table.

## Timestamps

**Strategy: TIMESTAMPTZ stored in UTC**

- All timestamp columns use `TIMESTAMPTZ` (timestamp with time zone) in PostgreSQL.
- Go code uses `time.Time`; pgx/v5 scans `TIMESTAMPTZ` into `time.Time` in UTC automatically.
- API responses serialize timestamps as ISO 8601 strings with explicit UTC Z suffix:
  `"2026-09-18T12:00:00Z"`
- Flutter/Dart parses with `DateTime.parse(s).toUtc()`. All `DateTime` instances in domain
  models are UTC. Localization to display time zones is a presentation-layer concern only.
- All `created_at` and `updated_at` columns use `DEFAULT now()`.
- Business logic must not perform wall-clock-relative comparisons without explicit timezone handling.

## Weekly ranking keys

- Weekly keys follow the format: `leaderboard:{zone}:{type}:{YYYY-wWW}`
- Week boundaries follow ISO 8601 (week starts Monday).
- Week number is zero-padded: `2026-w38`.
- Sunday-night rotation logic must derive week boundaries from UTC timestamps explicitly.
- Date/time calculations around weekly rotation must be timezone-aware and tested.

## References

- ADR 0004 — Identifier strategy decision and rationale.
- `.claude/rules/database.md` — PostgreSQL and Redis key conventions.
