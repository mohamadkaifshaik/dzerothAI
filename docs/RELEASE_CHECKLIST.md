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
- [ ] Backup/restore path is validated for production changes.

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

- [ ] Structured logs work.
- [ ] Metrics/health checks work where configured.
- [ ] Alerts are meaningful.
- [ ] Deployment procedure is documented.
- [ ] Rollback procedure is documented.
- [ ] Post-deployment smoke test is ready.

## Final review

- [ ] Production reviewer has inspected the diff.
- [ ] Known risks are documented.
- [ ] Release artifact is reproducible.
