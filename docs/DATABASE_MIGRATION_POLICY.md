# Dzeroth — Database Migration Policy

**Status:** `PHASE 8D-4 — Migration ownership documented`

This document records the authoritative migration ownership policy: how migrations
run, what deployment topology they are safe for, and what to do when horizontal
scaling is needed.

---

## Current behavior

Migrations run automatically at API startup.

The API binary embeds all SQL migration files at compile time via `//go:embed`
in `apps/backend/db/migrations.go`. On startup, `runMigrations()` in
`cmd/api/main.go` calls `golang-migrate`'s `m.Up()` which:

- Opens a `schema_migrations` tracking table if it does not exist.
- Applies all pending `.up.sql` files in numeric order.
- Records the highest applied version and a `dirty` flag.
- Uses PostgreSQL advisory locks internally to serialize concurrent migrate calls.
- Is idempotent: `ErrNoChange` is not treated as an error.

Migration runs are logged at startup: `"database migrations applied"`.

---

## Safe for: single-instance deployment

The current migration approach is **safe for single-instance deployment** — one
API container running at any given time.

The golang-migrate library uses PostgreSQL advisory locks (`pg_advisory_lock`)
to prevent simultaneous migration runs. If two API instances start at exactly the
same time, one will acquire the lock and run migrations; the other will wait and
then detect `ErrNoChange`.

However, in practice, production deployments use sequential restarts (stop old
container, start new container), which eliminates the concurrent-startup race
entirely.

---

## NOT safe for: simultaneous multi-instance startup without coordination

If multiple API containers are started simultaneously from scratch (e.g. in a
Kubernetes Deployment with `replicas: 3`), the following race can occur:

1. All three instances attempt `m.Up()` concurrently.
2. golang-migrate's advisory lock serializes them, but the first instance to
   acquire the lock runs migrations; the others retry.
3. This works in practice due to the advisory lock, but the startup latency is
   unpredictable and the pattern is fragile.

**The embedded-migration pattern is not validated for simultaneous scale-out.**
Do not rely on it for horizontal scaling without the mitigation below.

---

## Mitigation for horizontal scaling

If the deployment architecture requires running multiple API instances
simultaneously (e.g. Kubernetes with replicas > 1 or zero-downtime blue/green):

### Option 1: Dedicated migration job (preferred)

Run a dedicated migration step before deploying the API instances:

```yaml
# Kubernetes example: init container or pre-deploy job
- name: migrate
  image: dzeroth-api:<tag>
  command: ["/app/api", "--migrate-only"]  # requires a --migrate-only flag (not yet implemented)
  env:
    - name: POSTGRES_HOST
      value: postgres
    # ... other required vars
```

This requires adding a `--migrate-only` flag to the API binary that runs
migrations and exits. **This flag is not currently implemented.**

### Option 2: External golang-migrate CLI (alternative)

Run the standalone `golang-migrate` CLI tool in a CI step or init container
before starting any API instances:

```bash
migrate -path db/migrations -database "pgx5://..." up
```

Then disable migration-on-startup in the API (requires a config flag — not
currently implemented).

### Option 3: Expand/contract migration strategy

Design all schema migrations to be backward compatible (additive only in the
forward direction), so that old and new API instances can run simultaneously
against the same schema. This is the safest long-term pattern and does not
require external migration coordination.

---

## Current production topology

The current `docker-compose.prod.yml` topology deploys a single API container.
Sequential restart (stop → start) is the deployment model. Migration on startup
is safe and correct for this topology.

**No code change is required for the current single-instance topology.**

---

## References

- Startup sequence: `docs/DEPLOYMENT_TOPOLOGY.md` — Application startup sequence
- Migration failure recovery: `docs/DEPLOYMENT_TOPOLOGY.md` — If migration fails (dirty flag)
- Migration SQL: `apps/backend/db/migrations/`
