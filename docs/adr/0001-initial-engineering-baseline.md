# ADR 0001 — Initial Engineering Baseline

**Status:** ACCEPTED

## Context

Dzeroth is intended to be a production-grade text-first social platform with familiar X/Twitter-like functionality while remaining an independent implementation. The project has Flutter frontend requirements, Go backend requirements, PostgreSQL/PostGIS persistence, Redis-backed ranking/caching requirements, and strict anti-addiction/privacy invariants.

## Decision

Use the existing project architecture and the layered Claude Code operating model:

- Flutter/Dart frontend with Clean Architecture and BLoC.
- Go backend with `cmd/` and `internal/`.
- PostgreSQL 15+ with PostGIS where spatial data is required.
- Redis 7+ for caching/ranking use cases defined by the product.
- Raw SQL migrations under the backend migration directory.
- `.claude/agents/` for specialized execution roles.
- `.claude/rules/` for focused engineering constraints.
- `docs/` for durable product and architecture knowledge.
- `CLAUDE.md` as the global project constitution.

Agents must inspect existing code before creating or modifying architecture.

## Alternatives considered

### Build feature-by-feature without an audit

Rejected because it increases the risk of duplicate modules, conflicting APIs, and incompatible data models.

### Put every rule in one large instruction file

Rejected because large global instructions increase context noise and make specialized rules harder to maintain.

### Let each agent define its own conventions

Rejected because independent agent conventions create drift and duplicate implementations.

## Consequences

- Initial implementation work should begin with audit and architecture planning.
- Significant architectural decisions become searchable and reviewable.
- Feature work must stay aligned with the registry and roadmap.
- Documentation becomes part of the engineering system rather than an afterthought.

## Validation / follow-up

Run the project auditor and architecture planner before substantial implementation. Update this ADR if the baseline architecture materially changes.
