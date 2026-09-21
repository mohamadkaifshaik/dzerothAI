# Dzeroth — Environment Strategy

**Status:** `UPDATED — Phase 8D-4 (Docker Secrets _FILE convention added)`

This document records the authoritative environment variable definitions and
the separation strategy between local, test/CI, staging, and production
environments. All variables listed here have been verified in
`apps/backend/internal/config/config.go`.

---

## Environment names

| Value | Used by | Behaviour |
|---|---|---|
| `local` | Developer workstations | CORS: allow all origins. Logger: development (colored, human-readable). |
| `test` | CI / automated tests | CORS: allow all origins. Logger: development. |
| `staging` | Staging host | CORS: explicit allowlist (`CORS_ALLOWED_ORIGINS`). Logger: production (JSON). |
| `production` | Production host | CORS: explicit allowlist. Logger: production (JSON). |

`ENVIRONMENT` defaults to `local` when unset. Non-local/test environments log a
startup warning if `CORS_ALLOWED_ORIGINS` is empty.

In `docker-compose.staging.yml` and `docker-compose.prod.yml`, `ENVIRONMENT` is
hardcoded in the compose `environment:` block (`staging`/`production`) and
cannot be accidentally overridden by the `env_file`.

---

## All environment variables

Authoritative source: `apps/backend/internal/config/config.go`.

| Variable | Required | Default | Description |
|---|---|---|---|
| `POSTGRES_USER` | Yes | — | PostgreSQL user |
| `POSTGRES_PASSWORD` | Yes | — | PostgreSQL password (secret) |
| `POSTGRES_DB` | Yes | — | PostgreSQL database name |
| `POSTGRES_HOST` | Yes | — | PostgreSQL host (use `postgres` in compose) |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_SSL_MODE` | No | `disable` | pgx `sslmode` parameter |
| `REDIS_ADDR` | Yes | — | Redis address `host:port` (use `redis:6379` in compose) |
| `REDIS_PASSWORD` | No | `""` | Redis AUTH password |
| `REDIS_TLS` | No | `false` | Enable TLS for Redis connection |
| `JWT_SECRET` | Yes | — | HMAC-SHA256 signing secret, minimum 32 bytes (secret) |
| `API_PORT` | No | `8080` | HTTP API listener port |
| `ADMIN_ADDR` | No | `127.0.0.1:9091` | Admin/observability HTTP listener address (loopback default; use `:9091` if a sidecar scrapes across the bridge) |
| `ENVIRONMENT` | No | `local` | Runtime environment name |
| `LOG_LEVEL` | No | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `CORS_ALLOWED_ORIGINS` | No | `""` | Comma-separated exact origins for CORS (required in staging/production) |
| `SESSION_CLEANUP_INTERVAL` | No | `1h` | Session cleanup worker interval (Go duration string) |

---

## Secret vs non-secret

| Variable | Secret? | Where it lives |
|---|---|---|
| `POSTGRES_PASSWORD` | **Yes** | `deploy/secrets/env.*` only, never committed |
| `REDIS_PASSWORD` | **Yes** | `deploy/secrets/env.*` only, never committed |
| `JWT_SECRET` | **Yes** | `deploy/secrets/env.*` only, never committed |
| All others | No | `deploy/secrets/env.*` or compose `environment:` block |

---

## Docker Secrets `_FILE` convention

For variables marked as secrets, the Go config package supports a `_FILE` suffix
variant. When `<VAR>_FILE` is set, the secret is read from the file at that path.
This follows the Docker Secrets standard pattern where secrets mount as files.

| `_FILE` variable | Plain variable | Supported |
|---|---|---|
| `JWT_SECRET_FILE` | `JWT_SECRET` | Yes |
| `POSTGRES_PASSWORD_FILE` | `POSTGRES_PASSWORD` | Yes |
| `REDIS_PASSWORD_FILE` | `REDIS_PASSWORD` | Yes |

**Precedence:** `_FILE` takes precedence over the plain env var. If both are set,
the file value is used.

**Backward compatibility:** if only the plain env var is set (current default
deployment), it continues to work unchanged.

**Opt-in for operators:** the current `docker-compose.prod.yml` uses plain env vars
via `env_file: deploy/secrets/env.production`. Operators who want Docker Secrets
file isolation can mount secret files and set the `_FILE` variants instead.

See `secrets/README.md` for setup instructions and `docs/DEPLOYMENT_TOPOLOGY.md`
for the Docker Secrets Compose example.

---

## Environment separation rules

- Secrets must never be committed to git. `.gitignore` covers `.env`, `.env.*`,
  `*.env`, and `deploy/secrets/env.*`.
- Local `.env` files at the project root and in `apps/backend/` are gitignored.
- The root `.env` (for local Docker Compose use) sets non-secret dev defaults.
  It must not contain production values.
- `deploy/secrets/env.*` files exist only on deployment hosts. The
  `deploy/secrets/` directory is self-gitignored via `deploy/secrets/.gitignore`.
- Production secrets must be different from staging secrets.
- CI uses test databases / mock values. It never connects to production.

---

## Configuration for each environment

### Local development

Use `apps/backend/.env.example` as a reference. Copy it to `apps/backend/.env`
(gitignored). Or use the root `docker-compose.yml` with a root `.env` file.

```text
ENVIRONMENT=local
POSTGRES_SSL_MODE=disable
REDIS_TLS=false
LOG_LEVEL=debug
```

### CI / test

Environment variables are set in the GitHub Actions workflow
(`.github/workflows/ci.yml`). No secrets file is used in CI — the migration
smoke test spins up a fresh postgres container with known test credentials.

### Staging

Copy `deploy/env.staging.example` to `deploy/secrets/env.staging` on the
staging host. Set `POSTGRES_SSL_MODE=disable` when using the bundled compose
postgres. Set explicit `CORS_ALLOWED_ORIGINS`.

### Production

Copy `deploy/env.production.example` to `deploy/secrets/env.production` on
the production host. Set `POSTGRES_SSL_MODE=require` (or `verify-full`) for
managed PostgreSQL. Set `REDIS_TLS=true` if using managed Redis with TLS.
Set explicit `CORS_ALLOWED_ORIGINS`.

---

## .env file auto-loading caveat

Docker Compose auto-loads a `.env` file from the project root when running
`docker compose`. If the root `.env` defines any variable that a compose
service also defines via `${VAR:-default}` substitution, the root `.env` value
wins over the default.

This was discovered during Phase 8C-2 validation: the root `.env` defines
`POSTGRES_DB=dzeroth`, which overrode `${POSTGRES_DB:-dzeroth_staging}` in
the staging compose file, causing PostgreSQL to initialize the wrong database.

**Fix applied in commit `4e9caa2`:** Both compose files now hardcode the
postgres init variables (`POSTGRES_DB`, `POSTGRES_USER`) as literal values
rather than compose interpolation expressions. The root `.env` no longer
affects postgres initialization.
