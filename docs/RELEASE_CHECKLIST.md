# Dzeroth Release Checklist

## Repository

- [ ] Git working tree is understood.
- [ ] Intended branch is confirmed.
- [ ] No accidental generated files.
- [ ] No secrets.
- [ ] No duplicate modules/endpoints/models.

## Build

- [ ] Flutter analyze passes.
- [ ] Flutter tests pass.
- [ ] Go formatting passes.
- [ ] Go tests pass.
- [ ] Backend static analysis passes.
- [ ] Dependency/security checks pass where configured.

## Database

- [ ] Migrations are ordered and reversible where practical.
- [ ] No destructive migration without explicit approval.
- [ ] Indexes and constraints are reviewed.
- [ ] Backup/restore path is validated for production changes. See `docs/BACKUP_RECOVERY.md`.
- [ ] Migration ownership policy reviewed — single-instance safe; see `docs/DATABASE_MIGRATION_POLICY.md`.

## API

- [ ] Contract changes are documented.
- [ ] Authorization is tested.
- [ ] Error behavior is consistent.
- [ ] Pagination is bounded and tested.
- [ ] Idempotency is tested for retry-sensitive mutations.

## Product invariants

- [ ] No infinite feed.
- [ ] Hard feed termination works.
- [ ] Share/quote countdown is enforced server-side.
- [ ] Quote text minimum of 5 distinct words is enforced client/server.
- [ ] Public validation metrics remain hidden.
- [ ] Creator analytics remain private.

## Operations

- [ ] Structured logs work (verified in staging).
- [ ] `/livez`, `/readyz`, `/health`, `/metrics` respond correctly on admin port (:9091).
- [ ] `/health` accessible on public port (:8080).
- [ ] `dzeroth_build_info` metric shows correct version/commit/build_time.
- [ ] Session cleanup worker starts and stops cleanly (verified in logs).
- [ ] Graceful shutdown completes without SIGKILL (`stop_grace_period: 20s` in compose).
- [ ] Deployment procedure documented — see `docs/DEPLOYMENT_TOPOLOGY.md`.
- [ ] Rollback procedure documented — see `docs/DEPLOYMENT_TOPOLOGY.md` rollback section.
- [ ] Rollback compatibility assessed for any schema migrations in this release.
- [ ] PostgreSQL backup strategy confirmed. See `docs/BACKUP_RECOVERY.md`. Scripts: `scripts/backup/pg_backup.sh` / `pg_restore.sh`.
- [ ] Reverse proxy provisioned and forwarding headers sanitized. Reference config: `nginx/nginx.prod.conf`.
- [ ] `CORS_ALLOWED_ORIGINS` explicitly set (no wildcard in staging/production).
- [ ] Admin port (:9091) not reachable from public internet.
- [ ] PostgreSQL and Redis ports not reachable from public internet.
- [ ] Security headers present in API responses (`X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`).
- [ ] Prometheus scrape configured and `dzeroth_redis_up` / error-rate alerts active. See `docs/MONITORING.md`.
- [ ] Pre-production security checklist completed — see `docs/DEPLOYMENT_TOPOLOGY.md`.
- [ ] Post-deployment smoke test passed (`/health` returns ok after deploy).

## Final review

- [ ] Production reviewer has inspected the diff.
- [ ] Known risks are documented (see `docs/DEPLOYMENT_TOPOLOGY.md` — Remaining infrastructure gaps).
- [ ] Release artifact is reproducible (image built from tagged commit with ldflags).
