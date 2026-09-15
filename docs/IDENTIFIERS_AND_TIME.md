# Identifier and Time Conventions

**Status:** `BASELINE — CONFIRM DURING AUDIT`

The project must use one consistent identifier strategy and one consistent time representation.

## Rules

- Inspect existing database and Go models before choosing a new ID format.
- Do not introduce a second ID scheme for new tables or APIs.
- API IDs must serialize consistently.
- Timestamps must use a consistent UTC/storage strategy unless a domain requirement explicitly requires otherwise.
- Client display time may be localized, but persisted timestamps must remain unambiguous.
- Ranking week keys must follow the existing `YYYY-wWW` convention.
- Date/time calculations around weekly rotation must be timezone-aware and explicitly tested.

## Decision process

The architecture planner should record the final project-wide convention in an ADR after inspecting the current codebase.
