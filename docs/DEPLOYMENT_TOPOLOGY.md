# Dzeroth — Deployment Runbook

This document is the authoritative operational reference for Dzeroth staging
and production deployments. It covers topology, secrets provisioning,
PostgreSQL/Redis configuration, CORS, the trusted reverse proxy requirement,
migrations, startup, health verification, graceful shutdown, rollback, backups,
failure scenarios, and the pre-production security checklist.

Do not deploy to any environment without completing the applicable checklist
sections for that environment.

---

## Container topology

```
Internet
    │
    ▼
[ Reverse Proxy / Load Balancer ]   ← TLS termination here (operator-provisioned)
    │  :443 → :8080
    ▼
┌─────────────────────────── dzeroth_backend (private bridge network) ─────────┐
│                                                                                 │
│  ┌─────────────────┐     ┌──────────────────┐     ┌──────────────────┐        │
│  │   dzeroth-api   │────▶│    postgres       │     │     redis        │        │
│  │   :8080 (pub)   │     │    :5432 (priv)   │     │   :6379 (priv)   │        │
│  │   :9091 (priv)  │────▶│                  │     │                  │        │
│  └─────────────────┘     └──────────────────┘     └──────────────────┘        │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘

  (priv) = not published to host network
  (pub)  = published to host, must be fronted by a trusted reverse proxy
```

### Port publication rules

| Port | Service | Published in prod | Published in staging | Notes |
|------|---------|-------------------|---------------------|-------|
| 8080 | API (public) | Yes (`127.0.0.1:8080`) | Yes (`0.0.0.0:8080`) | Must sit behind a TLS-terminating trusted proxy |
| 9091 | Admin (metrics/health) | **No** | `127.0.0.1:9091` only | Never expose publicly |
| 5432 | PostgreSQL | **No** | **No** | Private network only |
| 6379 | Redis | **No** | **No** | Private network only |

---

## Admin port (:9091)

The admin server exposes:
- `GET /livez` — liveness (process alive, no dependency checks)
- `GET /readyz` — readiness (PostgreSQL and Redis reachable)
- `GET /health` — combined health with DB pool stats
- `GET /metrics` — Prometheus metrics

**This port must never be reachable from the public internet.**

In production, the admin port is not published. Access it from monitoring
infrastructure on the private network, or temporarily via:

```bash
docker exec -it <api-container> wget -qO- http://localhost:9091/metrics
docker exec -it <api-container> wget -qO- http://localhost:9091/readyz
```

In staging, the admin port is bound to `127.0.0.1:9091` only. Engineers can
SSH-tunnel to reach it:

```bash
ssh -L 9091:localhost:9091 staging-host
# then locally:
curl http://localhost:9091/metrics
curl http://localhost:9091/readyz
```

---

## Trusted reverse proxy requirement

### Why it is mandatory

The API binary serves plain HTTP. TLS termination and client-IP trust are the
reverse proxy's responsibility. The proxy must sit in front of port 8080.

### Rate limiting and source-IP integrity

The auth rate limiter keys requests by client IP:

```text
Redis key: rl:auth:{operation}:{ip}
```

The source IP is resolved by the backend in this priority order:

1. `X-Real-IP` request header
2. First IP in `X-Forwarded-For` request header
3. `r.RemoteAddr` (TCP peer address)

`chimw.RealIP` middleware (from chi) also rewrites `r.RemoteAddr` to the
value from `X-Real-IP` or `X-Forwarded-For` before route handlers run.

**If a client can send arbitrary `X-Real-IP` or `X-Forwarded-For` headers and
the proxy does not strip them, any client can claim any IP and bypass per-IP
rate limits entirely.**

### Required proxy behaviour

The trusted reverse proxy MUST:

1. Terminate the external TLS connection.
2. **Remove** any `X-Real-IP` and `X-Forwarded-For` headers arriving from
   the client before forwarding the request to the API.
3. **Set** `X-Real-IP` (or `X-Forwarded-For`) to the real client IP that the
   proxy observed on the accepted TCP connection.
4. Forward the request to `http://<host>:8080`.
5. Never allow clients to influence the forwarding headers that reach the API.

### Consequence of misconfiguration

If the proxy does not sanitize forwarding headers:
- Any client can set `X-Real-IP: 1.2.3.4` and operate from a different IP
  namespace for rate-limiting purposes.
- The access log `remote_ip` field becomes untrustworthy.
- Per-IP abuse detection is ineffective.

### Acceptable proxy products

nginx, Caddy, AWS ALB, GCP Cloud Load Balancing, Cloudflare, HAProxy, or any
product that provides configurable `X-Forwarded-For` / `X-Real-IP` sanitization.

### Reference nginx configuration

A reference nginx configuration is provided at `nginx/nginx.prod.conf`. It:
- Listens on port 443 (TLS) with a redirect from port 80.
- Proxies to the Go API container (`http://api:8080`).
- Sets `proxy_set_header X-Real-IP $remote_addr` — uses the accepted TCP connection
  IP, NOT the client-supplied header value. This is the correct setting.
- Does NOT include `proxy_set_header X-Real-IP $http_x_real_ip` (that would trust
  the client and defeat rate-limit protection).
- Sets proxy timeouts matching the Go API's 30-second handler timeout.
- Places HSTS (`Strict-Transport-Security`) at the proxy layer, not the application.
- Does NOT proxy the admin port (:9091) — admin is internal only.

Operators must replace placeholder certificate paths and domain names before use.
This configuration is a reference starting point, not a production-hardened template.

The `nginx` service is not included in `docker-compose.prod.yml`. The proxy is
provisioned separately (as a system service, a sidecar container, or a cloud LB).

### Required trusted-proxy topology

```
Internet → Nginx (TLS termination, :443) → Go API (plain HTTP, :8080)
```

Running the Go API with port 8080 directly exposed to the internet is NOT
supported for production. Without the trusted proxy:
- No TLS termination.
- `X-Real-IP` and `X-Forwarded-For` are client-controlled — rate limits are bypassed.
- Access logs show client-supplied IPs — untrustworthy.

---

## Secrets provisioning

### What must be secret

| Variable | Description | How to generate |
|---|---|---|
| `POSTGRES_PASSWORD` | PostgreSQL superuser password | `openssl rand -base64 32` |
| `REDIS_PASSWORD` | Redis authentication password | `openssl rand -base64 32` |
| `JWT_SECRET` | JWT HMAC-SHA256 signing secret (min 32 bytes) | `openssl rand -base64 48` |

### Rules

- **Do not commit secrets to git.** `deploy/secrets/` is gitignored via
  `deploy/secrets/.gitignore` and the root `.gitignore`. Never override this.
- **Do not bake secrets into Docker images.** The Dockerfile contains no
  secrets; all configuration is injected at runtime via environment variables.
- **Do not place secrets into Compose YAML.** `docker-compose.prod.yml` and
  `docker-compose.staging.yml` contain only non-secret configuration.
  Secrets are loaded from `deploy/secrets/env.*` via `env_file`.
- **Use a separate `JWT_SECRET` for staging and production.** Staging tokens
  must not be valid on production.
- **Rotate `JWT_SECRET` with care.** Rotation invalidates all active sessions —
  all users will be required to re-authenticate.

### Provisioning procedure

```bash
# 1. On the deployment host, create the secrets directory if not already present
mkdir -p deploy/secrets
chmod 700 deploy/secrets

# 2. Copy the template
cp deploy/env.production.example deploy/secrets/env.production

# 3. Fill in all REQUIRED_ values — do NOT use the placeholder values
#    Generate each secret:
openssl rand -base64 32   # for POSTGRES_PASSWORD and REDIS_PASSWORD
openssl rand -base64 48   # for JWT_SECRET

# 4. Edit deploy/secrets/env.production and replace each REQUIRED_ placeholder
#    Do not skip any REQUIRED_ value — the API will refuse to start if missing

# 5. Verify no REQUIRED_ placeholder remains
grep 'REQUIRED_' deploy/secrets/env.production && echo "INCOMPLETE — fix before deploying"
```

### Secret manager note

`deploy/secrets/env.*` files are plain files on the deployment host. For
production hardening, consider injecting environment variables from a secrets
manager (HashiCorp Vault, AWS Secrets Manager, GCP Secret Manager, Azure Key
Vault, etc.) instead of plain files. **No secrets manager is currently
implemented in this repository.** That is a required future hardening task.

---

## PostgreSQL configuration

### Bundled Compose service (development / self-hosted staging)

`docker-compose.staging.yml` and `docker-compose.prod.yml` include a
`postgis/postgis:15-3.3` PostgreSQL service. This image starts with `ssl=off`
in `postgresql.conf` — it has **no TLS configured**.

When using the bundled service:
```text
POSTGRES_SSL_MODE=disable
```

### Managed PostgreSQL (recommended for production)

Managed services (AWS RDS for PostgreSQL 15+, Google Cloud SQL, Supabase,
Neon, etc.) configure server-side TLS automatically. Use:

```text
POSTGRES_SSL_MODE=require        # TLS required, system CA pool
POSTGRES_SSL_MODE=verify-full    # TLS required + hostname verification (strongest)
```

`require` is the minimum acceptable for a managed service.
`verify-full` is recommended when you control the CA.

### SSL mode reference

| PostgreSQL deployment | `POSTGRES_SSL_MODE` |
|---|---|
| Managed service (RDS, Cloud SQL, etc.) | `require` or `verify-full` |
| Bundled Compose service, no certs | `disable` |
| Bundled Compose service with custom certs | `require` or `verify-full` |

**`POSTGRES_SSL_MODE=disable` is NOT acceptable when connecting to a managed
PostgreSQL service over a shared network.**

### Self-hosted PostgreSQL TLS

If you need TLS on the bundled Compose service, you must:
1. Generate or obtain a server certificate and key.
2. Mount them into the postgres container.
3. Supply a custom `postgresql.conf` with `ssl = on`, `ssl_cert_file`,
   `ssl_key_file`.
4. Update the compose postgres service definition with the mounts and config.

This is outside the scope of the current compose files.

### Connecting to managed PostgreSQL (without bundled service)

Remove the `postgres:` service and `pgdata_prod:` volume from the compose file.
Set `POSTGRES_HOST`, `POSTGRES_PORT`, and credentials in `env.production`.

---

## Redis configuration

### Bundled Compose service (development / self-hosted staging)

The bundled `redis:7-alpine` service does not support TLS. It uses password
authentication only:

```text
REDIS_TLS=false
REDIS_PASSWORD=<strong password>
```

The password is passed to `redis-server --requirepass` at container startup.
Unauthenticated connections are rejected with `NOAUTH Authentication required.`

### Production Redis

For production, Redis authentication is always required (`REDIS_PASSWORD`).
If your Redis infrastructure supports TLS (AWS ElastiCache TLS mode, Redis
Cloud, Upstash TLS, etc.), enable it:

```text
REDIS_TLS=true
REDIS_PASSWORD=<strong password>
```

The Redis client uses the system CA certificate pool for TLS verification.
Self-signed certificates are not supported without additional CA configuration.

### Redis unavailability behaviour

Redis is non-fatal at startup: if `Connect` fails, the API logs an error and
continues. Auth endpoints that require Redis for rate limiting will fail closed
(HTTP 503) until Redis is reachable. This is the correct fail-safe behaviour —
never disable rate limiting in degraded Redis states.

### Connecting to managed Redis (without bundled service)

Remove the `redis:` service and `redisdata_prod:` volume from the compose file.
Set `REDIS_ADDR`, `REDIS_PASSWORD`, and `REDIS_TLS` in `env.production`.

---

## CORS configuration

In `local` and `test` environments, all origins are permitted (no CORS
restriction). In `staging` and `production`, an explicit allowlist is required.

```text
CORS_ALLOWED_ORIGINS=https://app.dzeroth.com
# Multiple origins:
CORS_ALLOWED_ORIGINS=https://app.dzeroth.com,https://www.dzeroth.com
```

Rules:
- Origins must be exact (scheme + host + port). Wildcards are not supported.
- Requests from unlisted origins receive no `Access-Control-Allow-Origin` header.
- The browser blocks the response. The server still processes the request.
- The API logs a warning at startup if `CORS_ALLOWED_ORIGINS` is empty in a
  non-local/test environment: `cors allowlist configured origin_count=0`.

If the Flutter web app or CDN is served from a different origin than expected,
update `CORS_ALLOWED_ORIGINS` — do not switch to `ENVIRONMENT=local` in
staging/production as that re-enables wildcard CORS.

---

## Health and observability

### Endpoints

| Endpoint | Listener | Purpose | Expected healthy response |
|---|---|---|---|
| `GET /livez` | admin :9091 | Liveness: is the process alive? | `{"status":"ok"}` HTTP 200 |
| `GET /readyz` | admin :9091 | Readiness: are dependencies reachable? | `{"status":"ready"}` HTTP 200 |
| `GET /health` | both :8080 and :9091 | Combined health + DB pool stats | `{"status":"ok","db":"ok","redis":"ok","db_pool":{...}}` HTTP 200 |
| `GET /metrics` | admin :9091 | Prometheus metrics | Text metrics output HTTP 200 |

### When to use each endpoint

**`/livez`** — Use for container liveness probes (e.g. Kubernetes liveness).
Returns 200 as long as the HTTP server goroutine is alive. Never fails on
dependency issues. Only the process death triggers a non-200 response.

**`/readyz`** — Use for load balancer readiness / Kubernetes readiness probes.
Returns 200 only when both PostgreSQL and Redis respond to ping within 3 seconds.
Returns 503 if either dependency is unreachable. A load balancer should stop
routing traffic to a non-ready instance.

**`/health`** — Use for operational monitoring dashboards, post-deploy smoke
tests, and manual health checks. Returns 200 with DB pool statistics when both
dependencies are healthy. Returns 503 with `"status":"degraded"` when a
dependency is unavailable.

**`/metrics`** — Use for Prometheus scraping. Exposes:
- `dzeroth_build_info{version, commit, build_time}` — build identity gauge
- `http_requests_total{method, route, status_code}` — request counter
- `http_request_duration_seconds{method, route}` — latency histogram
- `dzeroth_auth_events_total{category, result}` — auth event counter
- `dzeroth_rate_limit_events_total{category, result}` — rate limit counter
- `dzeroth_feed_terminations_total{feed_type}` — feed termination counter
- `dzeroth_db_pool_connections_{total,acquired,idle,max}` — DB pool gauges
- `dzeroth_redis_up` — Redis availability gauge (0=down, 1=up)
- `dzeroth_redis_errors_total` — Redis error counter (registered; dormant until
  a central Redis abstraction is introduced in a future phase)

**`/metrics` must never be exposed on the public API listener (port 8080).**
It is served only on the admin listener (:9091). Do not add `/metrics` to the
public router.

### Post-deploy health verification

After starting or rolling a new version:

```bash
# 1. Check readiness (wait for this to return 200 before routing traffic)
curl -s -o /dev/null -w "%{http_code}" http://localhost:9091/readyz
# Expected: 200

# 2. Check health detail
curl -s http://localhost:8080/health | python -m json.tool
# Expected: {"status":"ok","db":"ok","redis":"ok","db_pool":{...}}

# 3. Confirm build identity in metrics
curl -s http://localhost:9091/metrics | grep dzeroth_build_info
# Expected: dzeroth_build_info{build_time="...",commit="<sha>",version="<tag>"} 1

# 4. Confirm API is serving (unauthenticated endpoint)
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/health
# Expected: 200
```

---

## Migration procedure

### How migrations work

The API binary embeds all SQL migration files at compile time (via `//go:embed`
in `apps/backend/db/migrations.go`). On startup, `runMigrations()` in
`cmd/api/main.go` calls `golang-migrate`'s `m.Up()` which:
- opens a `schema_migrations` tracking table if it does not exist
- applies all pending `.up.sql` files in numeric order
- records the highest applied version and a `dirty` flag
- is idempotent: `ErrNoChange` is not treated as an error

**Migrations run automatically on every API startup.** There is no separate
migration runner process. The binary connects to PostgreSQL, runs migrations,
then starts the HTTP servers.

### Safe migration sequence

```text
1. Verify the new image is built and available (docker pull or local build).

2. Verify database connectivity:
   docker exec <api-container> wget -qO- http://localhost:9091/readyz

3. (Optional: zero-downtime) Deploy a backward-compatible migration first
   without updating the application image, then update the image once the
   migration is verified. This is expand/contract strategy — not required
   for all migrations, but required for any breaking schema change.

4. Stop the current API container:
   docker compose -f docker-compose.prod.yml stop api

5. Update IMAGE_TAG and start the new container:
   IMAGE_TAG=<new-tag> docker compose -f docker-compose.prod.yml up -d --no-deps api

   # --no-deps is required. Without it, Compose reconciles all dependency
   # services (postgres, redis) and may recreate them if their stored
   # config-hash is stale relative to the current compose file. PostgreSQL
   # and Redis are stateful; unnecessary recreation is never safe during a
   # migration sequence. --no-deps restricts the operation to the api
   # service only.

6. Wait for the container to start and migrations to complete:
   docker compose -f docker-compose.prod.yml logs api --follow
   # Look for: "database migrations applied"

7. Verify readiness:
   docker exec <api-container> wget -qO- http://localhost:9091/readyz
   # Expected: {"status":"ready"}

8. Verify migration tracking:
   docker exec <postgres-container> \
     psql -U dzeroth -d dzeroth -c \
     "SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1;"
   # Expected: version = <expected>, dirty = f

9. Verify health:
   curl -s http://localhost:8080/health
   # Expected: {"status":"ok",...}
```

### If migration fails (dirty flag)

If the API exits with `migration up:` in its error log, and the `dirty` column
is `t` in `schema_migrations`, the migration was interrupted mid-run.

Do NOT restart the API without resolving the dirty state.

```bash
# Inspect the dirty migration
docker exec <postgres-container> \
  psql -U dzeroth -d dzeroth -c \
  "SELECT version, dirty FROM schema_migrations;"

# If dirty=t: manually clean up the partial migration or force the version
# back to the last clean version using golang-migrate CLI before restarting.
# This requires human review of the affected migration SQL.
```

---

## Application startup sequence

The API binary performs these steps on every startup:

```text
1. Load and validate configuration (fails fast if required vars are missing)
2. Initialize structured logger (zap)
3. Log build identity (version, commit, build_time)
4. Connect to PostgreSQL (fatal if unreachable)
5. Run pending migrations (fatal if migration fails)
6. Connect to Redis (non-fatal — logs error, continues)
7. Wire all services
8. Register Prometheus metrics
9. Build routers (public :8080 and admin :9091)
10. Start session cleanup background worker
11. Start HTTP servers
12. Block on SIGTERM / SIGINT
```

A container in a `Restarting` loop typically indicates step 1 (missing env
var), step 4 (PostgreSQL unreachable or wrong credentials), or step 5 (failed
migration). Check `docker compose logs api` immediately.

---

## Staging deployment procedure

```bash
# Prerequisites:
#   - API image built and tagged
#   - deploy/secrets/env.staging populated (no REQUIRED_ placeholders)
#   - docker-compose.staging.yml present

export IMAGE_TAG=<git-sha-or-tag>

# 1. Bring up the stack (infrastructure first via depends_on)
IMAGE_TAG=${IMAGE_TAG} docker compose -f docker-compose.staging.yml up -d

# 2. Tail logs until "http server listening"
docker compose -f docker-compose.staging.yml logs api --follow

# 3. Verify readiness
curl -s http://localhost:9091/readyz      # {"status":"ready"}

# 4. Verify health
curl -s http://localhost:8080/health      # {"status":"ok","db":"ok","redis":"ok",...}

# 5. Verify build identity
curl -s http://localhost:9091/metrics | grep dzeroth_build_info

# 6. Run smoke test
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/health
```

---

## Production deployment procedure

There are two distinct cases. Use the correct procedure for each.

### Case A — First-time stack initialisation (PostgreSQL and Redis not yet running)

Use this only when bringing up a new environment from scratch. Running `up -d`
without `--no-deps` is intentional here because PostgreSQL and Redis do not
exist yet and must be started.

```bash
# Prerequisites:
#   - All staging checks passed
#   - Production approval obtained
#   - API image built from the same commit as staged
#   - deploy/secrets/env.production populated (no REQUIRED_ placeholders)
#   - Reverse proxy is configured and TLS certificate is valid

export IMAGE_TAG=<same-tag-as-staged>
export VERSION=$(git describe --tags --always)
export COMMIT=$(git rev-parse --short HEAD)
export BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# 1. Build the image (or pull if pushed to a registry)
docker build -f apps/backend/Dockerfile apps/backend/ \
  --build-arg VERSION=${VERSION} \
  --build-arg COMMIT=${COMMIT} \
  --build-arg BUILD_TIME=${BUILD_TIME} \
  -t dzeroth-api:${IMAGE_TAG}

# 2. Start the full stack (first time only — all services including postgres and redis)
IMAGE_TAG=${IMAGE_TAG} docker compose -f docker-compose.prod.yml up -d

# 3. Tail logs until "http server listening"
docker compose -f docker-compose.prod.yml logs api --follow

# 4. Verify readiness (from within the host — admin port not published)
docker exec $(docker compose -f docker-compose.prod.yml ps -q api) \
  wget -qO- http://localhost:9091/readyz

# 5. Verify health
curl -s http://localhost:8080/health

# 6. Verify through reverse proxy (end-to-end TLS)
curl -s https://dzeroth.com/health
```

### Case B — API image rollout (PostgreSQL and Redis already running)

This is the normal production deployment path. PostgreSQL and Redis are
long-running stateful services; they must not be reconciled or recreated as
part of an API image swap.

**Always use `--no-deps` for API-only image rollouts.**

Without `--no-deps`, Compose reconciles all dependency services and will
recreate PostgreSQL and Redis if their stored Compose config-hash is stale
relative to the current compose file. This can happen whenever
`docker-compose.prod.yml` has changed since the dependencies were last
started — even if the change did not affect the postgres or redis service
definitions. Named volumes survive recreation, but unexpected recreation of
stateful services is never acceptable during a routine API rollout.

`--no-deps` restricts the `up -d` operation to the `api` service only.
PostgreSQL and Redis are not inspected, compared, or touched.

```bash
export IMAGE_TAG=<same-tag-as-staged>

# 1. Build the image
docker build -f apps/backend/Dockerfile apps/backend/ \
  --build-arg VERSION=${IMAGE_TAG} \
  --build-arg COMMIT=${IMAGE_TAG} \
  --build-arg BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -t dzeroth-api:${IMAGE_TAG}

# 2. Roll the API service only — --no-deps prevents dependency recreation
IMAGE_TAG=${IMAGE_TAG} docker compose -f docker-compose.prod.yml up -d --no-deps api

# 3. Tail logs until "http server listening"
docker compose -f docker-compose.prod.yml logs api --follow

# 4. Verify readiness
docker exec $(docker compose -f docker-compose.prod.yml ps -q api) \
  wget -qO- http://localhost:9091/readyz

# 5. Verify health
curl -s http://localhost:8080/health

# 6. Verify through reverse proxy (end-to-end TLS)
curl -s https://dzeroth.com/health
```

> **PostgreSQL and Redis changes require a separate deliberate procedure.**
> Do NOT include a compose service definition change for postgres or redis in an
> API-only image rollout. Any intended change to PostgreSQL or Redis (image
> upgrade, configuration change, volume migration) must be planned, scheduled,
> and executed as a separate maintenance operation with a backup taken
> immediately beforehand. See the Backups and Emergency recovery sections.

---

## Staging-to-production promotion sequence

```text
git commit (on feature branch)
       ↓
CI (go test, go vet, flutter analyze, flutter test, Docker build, migrations smoke test, race detector)
       ↓
merge to main
       ↓
CI runs again on main
       ↓
docker build -t dzeroth-api:<sha>
       ↓
staging deployment (IMAGE_TAG=<sha>)
       ↓
staging health/readyz verification
       ↓
staging API smoke test
       ↓
production approval (human sign-off)
       ↓
production deployment (same IMAGE_TAG=<sha>)
       ↓
production health/readyz verification
       ↓
production smoke test
```

No automatic production deployment exists. Production deployment requires a
human approval step. CI does not push to production.

---

## Graceful shutdown

The API binary handles `SIGTERM` and `SIGINT` with the following sequence:

```text
SIGTERM / SIGINT received
       ↓
"shutdown signal received" logged
       ↓
workerCancel() called — session cleanup worker context cancelled
       ↓
workerWG.Wait() — block until session cleanup worker exits
       ↓
"session cleanup worker stopped" logged
       ↓
srv.Shutdown(ctx) — drain in-flight HTTP requests (15s timeout)
       ↓
adminSrv.Shutdown(ctx) — drain admin server
       ↓
"server stopped cleanly" logged
       ↓
process exits 0
```

`docker compose stop api` sends SIGTERM. Both `docker-compose.prod.yml` and
`docker-compose.staging.yml` set `stop_grace_period: 20s` on the API service,
giving the application 20 seconds to drain in-flight requests before Docker
sends SIGKILL. This is 5 seconds above the Go API's own 15-second shutdown
timeout.

**Do not use forced SIGKILL (`docker kill`) as the normal deployment path.**
Let the container stop gracefully.

If the API container is hard-killed (`SIGKILL`), the session cleanup worker
may be mid-batch. The cleanup SQL is idempotent — restarting will not leave
orphaned data.

---

## Healthcheck constraint

The Dockerfile HEALTHCHECK uses the `API_PORT` build argument and ENV:

```dockerfile
ARG API_PORT=8080
ENV API_PORT=${API_PORT}
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD wget -qO- http://localhost:${API_PORT}/health || exit 1
```

The `API_PORT` environment variable controls what port the HTTP server listens
on (`config.go` default: `8080`). The `ENV API_PORT=8080` in the Dockerfile
makes this explicit and the HEALTHCHECK references `${API_PORT}`.

**If `API_PORT` is overridden to a non-8080 value at runtime without also
overriding the build-time ARG, the Dockerfile ENV and HEALTHCHECK will target
8080.** To change the port: rebuild the image with `--build-arg API_PORT=<port>`.
The production and staging env templates set `API_PORT=8080`; changing the
port in a running container without rebuilding the image is not supported.

---

## Application rollback

### Application image rollback

An application image rollback is straightforward: set `IMAGE_TAG` to the
previous version and restart the API service. Use `--no-deps` for the same
reason as a forward rollout — PostgreSQL and Redis must not be touched.

```bash
IMAGE_TAG=<previous-tag> docker compose -f docker-compose.prod.yml up -d --no-deps api
docker exec $(docker compose -f docker-compose.prod.yml ps -q api) \
  wget -qO- http://localhost:9091/readyz
```

### Database migration rollback limitations

**NOT all application releases can safely roll back their database schema.**

Down migrations exist in the repository (`*.down.sql` files) but:
- Down migrations may be destructive (drop tables, drop columns, remove data).
- A previously applied migration that added a column can be reversed, but
  data written to that column is lost.
- Some schema changes (e.g. constraint additions) may not be safely reversible
  after application data has been written.

Before rolling back an application image that includes a schema migration:

1. Verify whether the older image version is compatible with the NEW schema
   (after migration) — this is the safest path.
2. If the older image requires the old schema, run the down migration manually
   via `golang-migrate` CLI before rolling back the image.
3. Evaluate data loss. Down migrations for certain phases (e.g. dropping a
   column) are not recoverable without a database backup.

**The safest rollback strategy is to design backward-compatible migrations
(expand/contract) so that the old and new image versions both work with the
post-migration schema.**

### Emergency recovery

If the API is completely non-functional after deployment and schema rollback
would cause unacceptable data loss:

1. Stop the API container.
2. Restore from the last known-good PostgreSQL backup.
3. Redeploy the last known-good image.
4. Verify health and readiness.
5. Document the incident.

---

## Backups

### PostgreSQL

PostgreSQL data is the primary stateful component. It is stored in the
`pgdata_prod` Docker volume.

**A Docker volume existing is NOT a backup.** Volume data is not protected
against host failure, accidental deletion (`docker volume rm`), or corruption.

**Backup implementation status:** Operational. Daily automated `pg_dump` backups
run at 02:00 UTC via a systemd timer, compressed with gzip, integrity-checked,
and uploaded to S3 (`dzeroth-production-postgres-backups-2026`, ap-south-2).
Restore was verified on 2026-09-21.

See `docs/BACKUP_RECOVERY.md` for the complete backup configuration, IAM
requirements, S3 setup, restore procedure, and restore drill record.

Known limitations:
- No point-in-time recovery (PITR) — recovery granularity is the last backup.
- Single host — a total host failure loses data written since the last backup.
- No cross-region S3 replication.

### Redis

Redis data (`redisdata_prod`) is used for rate limiting state. Redis data loss
causes no permanent data corruption — rate limit counters reset to zero.
Session-related state lives in PostgreSQL, not Redis. Backing up Redis is
recommended but not critical for correctness.

---

## Failure scenarios

### PostgreSQL unavailable at startup

**Symptom:** API container exits immediately. Logs: `fatal: database: db: ping:`.

**Response:**
1. Verify PostgreSQL is running: `docker compose ps postgres`.
2. Verify credentials: `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` match
   the database server configuration.
3. Verify `POSTGRES_HOST` resolves to the correct host (should be `postgres`
   within compose, or the managed service hostname externally).
4. If using `POSTGRES_SSL_MODE=require` against the bundled Compose service,
   change to `POSTGRES_SSL_MODE=disable` — the bundled image has no TLS.

---

### Redis unavailable at startup

**Symptom:** API logs `redis unavailable at startup — rate limiting will fail closed`
but continues running. Auth endpoints return HTTP 503.

**Response:**
1. Verify Redis is running: `docker compose ps redis`.
2. Verify `REDIS_PASSWORD` matches the password Redis was started with.
3. Verify `REDIS_ADDR` is correct.
4. Check Redis logs: `docker compose logs redis`.
5. Once Redis recovers, the API automatically reconnects on the next rate-limit
   call — no restart required.

---

### Migration failure

**Symptom:** API exits. Logs contain `fatal: migrations: migration up:`.

**Response:**
1. Check `schema_migrations` for `dirty=t`.
2. If dirty: do NOT restart. Review the failed migration SQL manually.
3. Fix the underlying cause (connectivity, permissions, schema conflict).
4. Use `golang-migrate` CLI to force the version back to the last clean state
   if needed, then restart the API.

---

### Readiness failure (`/readyz` returns 503)

**Symptom:** `curl http://localhost:9091/readyz` returns `{"status":"not_ready","db":"unavailable"}` or `{"status":"not_ready","redis":"unavailable"}`.

**Response:**
1. Check the specific failing dependency in the response body.
2. For DB: check `docker compose ps postgres`, verify credentials and SSL mode.
3. For Redis: check `docker compose ps redis`, verify password.
4. Do not route production traffic to a non-ready instance.

---

### CORS misconfiguration

**Symptom:** Flutter web app receives CORS errors. Browser console shows
`Access-Control-Allow-Origin` missing or wrong.

**Response:**
1. Verify `CORS_ALLOWED_ORIGINS` in the env file exactly matches the frontend
   origin (scheme + host + port). Trailing slashes are not allowed.
2. Verify `ENVIRONMENT` is not `local` or `test` (which would use `corsAllowAll`).
3. Restart the API after changing the CORS configuration.
4. API logs `cors allowlist configured origin_count=N` on startup — verify N > 0.

---

### Redis authentication failure

**Symptom:** Redis logs `WRONGPASS` or `NOAUTH`. API logs Redis connection errors.

**Response:**
1. Verify `REDIS_PASSWORD` in `env.production` / `env.staging` matches the
   password the Redis container was started with.
2. The Redis container is started with `redis-server --requirepass "$REDIS_PASSWORD"`.
   If the password was changed, recreate the Redis container.
3. Restart the API after fixing the password.

---

### PostgreSQL TLS mismatch

**Symptom:** API exits. Logs: `FATAL: SSL is not enabled on the server` or similar.

**Response:**
1. You are using `POSTGRES_SSL_MODE=require` against a PostgreSQL instance that
   has SSL disabled.
2. If using the bundled Compose service: change to `POSTGRES_SSL_MODE=disable`.
3. If using a managed PostgreSQL: verify the service has TLS enabled and the
   connection string is correct.

---

### Reverse proxy forwarding-header misconfiguration

**Symptom:** Rate limiting ineffective (all requests appear to come from the
same IP, e.g. `127.0.0.1` or the proxy's internal IP). Access logs show wrong
`remote_ip`.

**Response:**
1. Verify the proxy strips incoming `X-Real-IP` / `X-Forwarded-For` headers
   from clients and sets them to the true observed client IP.
2. Check the API access log `remote_ip` field — it should show the real end-user
   IP, not the proxy's internal address.
3. Fix the proxy configuration. Do not disable rate limiting as a workaround.

---

### Healthcheck failure

**Symptom:** Container shows `(unhealthy)` in `docker compose ps`.

**Response:**
1. Check API logs for startup errors.
2. Verify the API is running on port 8080 (the default — see healthcheck
   constraint section above).
3. Manually verify: `docker exec <container> wget -qO- http://localhost:8080/health`.
4. If `API_PORT` was overridden to a non-8080 value, the healthcheck is targeting
   the wrong port — revert `API_PORT` to `8080` or update the Dockerfile.

---

### Graceful shutdown timeout

**Symptom:** Container does not stop within Docker's default 10-second stop
timeout. Docker sends SIGKILL.

**Response:**
1. Increase `stop_grace_period` on the API service in the compose file if
   in-flight requests genuinely need more than 10 seconds to complete.
2. The session cleanup worker exits immediately on context cancellation — it
   does not block shutdown.
3. The HTTP server has a 15-second shutdown timeout in code
   (`context.WithTimeout(bgCtx, 15*time.Second)`). If Docker kills the process
   before 15 seconds, the compose `stop_grace_period` is too short. Set it
   to at least `20s`.

---

## Pre-production security checklist

Use this checklist before any production deployment. Status labels:

- **Implemented** — code/config exists and has been tested.
- **Configured** — requires operator action per deployment (set in env file).
- **Required** — must be done but is not a code/config task.
- **Not yet implemented** — known gap; document workaround or accept risk.

| # | Item | Status |
|---|---|---|
| 1 | Production secrets (`POSTGRES_PASSWORD`, `REDIS_PASSWORD`, `JWT_SECRET`) supplied via `deploy/secrets/env.production`, not committed | **Configured** |
| 2 | No `.env` committed to the repository | **Implemented** (gitignore) |
| 3 | PostgreSQL port 5432 not reachable from the public internet | **Implemented** (no port binding in compose) |
| 4 | Redis port 6379 not reachable from the public internet | **Implemented** (no port binding in compose) |
| 5 | Admin port 9091 not published to host in production | **Implemented** (no `ports:` entry for 9091 in prod compose) |
| 6 | API behind a trusted TLS-terminating reverse proxy | **Required** (proxy not in repository) |
| 7 | Proxy strips/replaces `X-Real-IP` and `X-Forwarded-For` before forwarding | **Required** (proxy configuration, not in repository) |
| 8 | Explicit `CORS_ALLOWED_ORIGINS` set (no wildcard) | **Configured** |
| 9 | `ENVIRONMENT=production` set (enforces CORS allowlist, JSON logging) | **Implemented** (hardcoded in compose `environment:` block) |
| 10 | PostgreSQL TLS: `POSTGRES_SSL_MODE=require` or `verify-full` for managed PostgreSQL | **Configured** (template default is `require`) |
| 11 | Redis authentication: `REDIS_PASSWORD` set | **Configured** |
| 12 | Redis TLS: `REDIS_TLS=true` if infrastructure supports it | **Configured** (template default is `false` — update for managed Redis) |
| 13 | JWT secret is at least 32 bytes, generated with `openssl rand` | **Configured** |
| 14 | Staging uses a different `JWT_SECRET` from production | **Required** (operator responsibility) |
| 15 | All migrations reviewed and applied | **Implemented** (auto-applied at startup) |
| 16 | `/readyz` returns 200 before routing traffic | **Implemented** (endpoint exists and is correct) |
| 17 | `/health` returns `"status":"ok"` | **Implemented** |
| 18 | `/metrics` not publicly reachable (admin port only) | **Implemented** |
| 19 | PostgreSQL backup strategy confirmed and restore tested | **Implemented** (`scripts/backup/pg_backup.sh`, `pg_restore.sh`; see `docs/BACKUP_RECOVERY.md`). Backup timer installed and active. Restore drill completed 2026-09-21 (14 tables, schema_migrations v13, dirty=false). |
| 20 | Rollback compatibility assessed for any schema changes | **Required** (human review) |
| 21 | Production reverse proxy provisioned with valid TLS certificate | **Reference config** (`nginx/nginx.prod.conf`); operator must provision and configure with real certs. |
| 22 | Docker Secrets `_FILE` convention supported; opt-in for operators | **Implemented** (config.go supports `JWT_SECRET_FILE`, `POSTGRES_PASSWORD_FILE`, `REDIS_PASSWORD_FILE`; see `secrets/README.md`). Default deployment still uses plain env files. |
| 23 | External monitoring / alerting on `dzeroth_redis_up`, `readyz`, error rates | **Reference config** (`monitoring/prometheus.yml`; see `docs/MONITORING.md`). Operator must deploy Prometheus and configure alerting. |
| 24 | Go dependency vulnerability scan (govulncheck) | **Required** (manual; not in CI). govulncheck v1.4.0 run 2026-09-23 at e3bfd95: 0 reachable vulnerabilities, 0 vulnerabilities in imported packages; 1 module-level advisory GO-2026-5932 (golang.org/x/crypto/openpgp, not imported by the backend; backend uses x/crypto/bcrypt; no fixed version available). |

---

## Remaining infrastructure gaps

The following are confirmed gaps that exist after Phase 8C. They are known,
documented, and accepted. They do not block staging validation but must be
resolved before production readiness.

| Gap | Status | Impact |
|---|---|---|
| **Production reverse proxy** | Reference config in `nginx/nginx.prod.conf`. Operator must provision and configure separately. | Without it: no TLS, no XFF sanitization, no rate-limit protection. |
| **Managed PostgreSQL (production)** | Not provisioned. Bundled compose postgres is self-hosted without TLS. | `POSTGRES_SSL_MODE=require` will fail against bundled service; must use `disable` or provision managed PostgreSQL. |
| **Managed Redis (production)** | Not provisioned. Bundled compose redis has no TLS. | `REDIS_TLS=true` not functional until managed Redis is provisioned. |
| **Secrets manager** | Docker Secrets `_FILE` convention supported by config.go. `secrets/README.md` documents file creation. Plain `env_file` still used by default in Compose files. | Secrets still visible via `docker inspect` Config.Env when using plain env vars. Operators can opt into Docker Secrets file pattern. |
| **PostgreSQL backups** | **Operational.** Daily backup timer active (`dzeroth-backup.timer`). S3 upload verified. See `docs/BACKUP_RECOVERY.md`. | No production data loss protection until the systemd timer is installed on any replacement host. |
| **Restore testing** | **Completed.** Restore drill performed 2026-09-21: isolated restore to `dzeroth_restore_drill`, 14 tables, schema_migrations v13, dirty=false. Production database not modified. Repeat drill monthly. | — |
| **External monitoring / alerting** | `monitoring/prometheus.yml` reference scrape config added. See `docs/MONITORING.md`. No alertmanager or Grafana dashboards provided. | No proactive notification on failures until operator configures Prometheus alerting. |
| **Deployment platform / CD pipeline** | Not implemented. Manual `docker compose` deployment only. | No automated production rollout or rollback trigger. |
| **Healthcheck port flexibility** | `ARG API_PORT=8080` + `ENV API_PORT` added to Dockerfile; HEALTHCHECK uses `${API_PORT}`. | Overriding `API_PORT` at runtime without rebuilding the image is still not supported. |
| **Redis error counter wiring** | `dzeroth_redis_errors_total` is registered but dormant. Wiring deferred — requires adding `*InfraMetrics` to `RateLimitMiddleware` signature. | Redis errors not counted in metrics. See `docs/MONITORING.md`. |

---

## Environment variable reference

See `deploy/env.production.example` and `deploy/env.staging.example` for the
full variable reference with inline documentation.

The authoritative variable list and default values are in
`apps/backend/internal/config/config.go`.

---

## Volumes

| Volume | Purpose | Backup required |
|--------|---------|-----------------|
| `pgdata_prod` | PostgreSQL data | **Yes — see `docs/BACKUP_RECOVERY.md`** |
| `redisdata_prod` | Redis AOF/RDB persistence | Recommended |
| `pgdata_staging` | Staging PostgreSQL | No |
| `redisdata_staging` | Staging Redis | No |

Production PostgreSQL backups run daily via `scripts/backup/pg_backup.sh`
and the systemd timer (`dzeroth-backup.timer`). See `docs/BACKUP_RECOVERY.md`
for the full configuration, S3 bucket setup, and restore procedure.

---

## PostgreSQL TLS and the bundled Compose service

The env templates (`deploy/env.production.example`, `deploy/env.staging.example`)
default to `POSTGRES_SSL_MODE=require`. This setting is intended for **managed
PostgreSQL** services (AWS RDS, Google Cloud SQL, Supabase, Neon, etc.) that
configure server-side TLS automatically.

**The bundled `postgres:` service uses `postgis/postgis:15-3.3` which starts
with `ssl=off`. Connecting with `POSTGRES_SSL_MODE=require` against this service
will cause the API to fail on startup with an SSL connection error.**

| PostgreSQL deployment | Correct `POSTGRES_SSL_MODE` |
|----------------------|----------------------------|
| Managed service (RDS, Cloud SQL, etc.) | `require` or `verify-full` |
| Bundled Compose service (no TLS configured) | `disable` |
| Bundled Compose service with custom certs mounted | `require` or `verify-full` |

---

## Replacing compose services with managed infrastructure

### PostgreSQL

Remove the `postgres:` service and `pgdata_prod:` volume from `docker-compose.prod.yml`.
Set in `deploy/secrets/env.production`:

```text
POSTGRES_HOST=<managed-hostname>
POSTGRES_PORT=5432
POSTGRES_DB=dzeroth
POSTGRES_USER=dzeroth
POSTGRES_PASSWORD=<managed-password>
POSTGRES_SSL_MODE=require
```

Acceptable managed services: AWS RDS for PostgreSQL 15+, Google Cloud SQL,
Supabase, Neon.

### Redis

Remove the `redis:` service and `redisdata_prod:` volume from `docker-compose.prod.yml`.
Set in `deploy/secrets/env.production`:

```text
REDIS_ADDR=<managed-hostname>:6379
REDIS_PASSWORD=<managed-password>
REDIS_TLS=true
```

Acceptable managed services: AWS ElastiCache, Redis Cloud, Upstash.
