# Dzeroth Database and Redis Rules

## PostgreSQL

Use PostgreSQL 15+ and PostGIS as established by the project.

For precise spatial proximity requirements use:

```sql
GEOGRAPHY(Point, 4326)
```

Use appropriate spatial indexes.

## Migrations

Before creating a migration:

1. inspect all migrations
2. inspect current schema
3. search target objects
4. determine whether the change already exists
5. create one authoritative migration

Never create duplicate migrations for the same logical change.

Never casually edit an applied migration.

Never perform destructive schema changes without an explicit migration
strategy.

## Constraints

Database constraints should enforce invariants where appropriate.

Application validation does not replace database integrity.

## Runtime SQL

Follow the repository's existing runtime data-access convention.

Do not create a second database-access layer.

Use parameterized queries.

## Redis

Redis 7+ is used for cache/ranking responsibilities already defined by the
project.

Weekly ranking keys:

```text
leaderboard:zone:type:YYYY-wWW
```

Use ZSETs for localized ranking.

Define:

- TTL
- invalidation
- rebuild behavior
- failure behavior
- retry behavior

Redis must not accidentally become the sole durable source of truth.

## Weekly Titles

Sunday snapshot/rotation must be:

- idempotent
- timezone-aware
- observable
- retry-safe
- resilient to partial failure

Persist finalized weekly titles into PostgreSQL `weekly_titles`.
