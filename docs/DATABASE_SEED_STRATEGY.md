# Database Seed Strategy

**Status:** `PLANNED — VERIFY DURING AUDIT`

Seeds must support development and automated tests without contaminating production.

## Rules

- Production migrations remain the authoritative schema mechanism.
- Test data should be deterministic.
- Development seed data should be clearly separated from production data.
- Never place real user data or credentials in seeds.
- Seeds must respect foreign keys, constraints, and application invariants.
- Spatial seed data must use the established PostGIS representation.
- Ranking seed data must reflect the Redis/PostgreSQL ownership model.

## Before implementation

The database agent must inspect:

- existing migrations
- existing seed scripts
- test fixtures
- local Docker/Compose database setup
- CI database setup

Do not create a second seed mechanism if one already exists.
