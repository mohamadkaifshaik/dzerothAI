# Dzeroth — Deployment Topology

This document describes the container topology for production and staging
deployments, the network isolation model, and the operational requirements that
must be satisfied before a deployment is attempted.

---

## Container topology

```
Internet
    │
    ▼
[ Reverse Proxy / Load Balancer ]   ← TLS termination here
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
  (pub)  = published to host, intended to be fronted by a reverse proxy
```

### Port publication rules

| Port | Service | Published in prod | Published in staging | Notes |
|------|---------|-------------------|---------------------|-------|
| 8080 | API (public) | Yes (`0.0.0.0:8080`) | Yes (`0.0.0.0:8080`) | Must sit behind TLS proxy |
| 9091 | Admin (metrics/health) | **No** | `127.0.0.1:9091` only | See below |
| 5432 | PostgreSQL | **No** | **No** | Internal network only |
| 6379 | Redis | **No** | **No** | Internal network only |

### Admin port (:9091)

The admin server exposes:
- `GET /livez` — liveness check (process alive)
- `GET /readyz` — readiness check (dependencies reachable)
- `GET /metrics` — Prometheus metrics

**This port must never be reachable from the public internet.**

In production, the admin port is not published. Access it from monitoring
infrastructure via the private network, or temporarily via:

```bash
docker exec -it <api-container> wget -qO- http://localhost:9091/metrics
```

In staging, the admin port is bound to `127.0.0.1:9091` so engineers can
SSH-tunnel to access it:

```bash
ssh -L 9091:localhost:9091 staging-host
# Then locally:
curl http://localhost:9091/metrics
curl http://localhost:9091/readyz
```

---

## Reverse proxy requirement

The API binary serves plain HTTP. TLS must be terminated by a reverse proxy or
load balancer in front of port 8080.

Acceptable options: nginx, Caddy, AWS ALB, GCP Cloud Load Balancing, Cloudflare.

### Trusted proxy and real IP

The API uses `chimw.RealIP` middleware which reads `X-Forwarded-For` and
`X-Real-IP` headers to determine the client IP used for:
- rate limiting (Redis key prefix is client IP)
- access log `remote_ip` field

**This middleware trusts any `X-Forwarded-For` header by default.**

If your deployment places an untrusted network between clients and the API,
configure your reverse proxy to strip and re-set `X-Forwarded-For` to only
contain the real client IP. Do not allow clients to inject arbitrary
`X-Forwarded-For` values or rate-limit bypass becomes trivial.

---

## Security configuration checklist

Before deploying to staging or production, verify:

- [ ] `POSTGRES_SSL_MODE` is `require` or `verify-full` (not `disable`)
- [ ] `POSTGRES_PASSWORD` is a strong randomly generated value
- [ ] `REDIS_PASSWORD` is a strong randomly generated value
- [ ] `REDIS_TLS` is `true` if using a managed Redis with TLS support
- [ ] `JWT_SECRET` is at least 32 random bytes (`openssl rand -base64 48`)
- [ ] `JWT_SECRET` for staging is different from production
- [ ] `CORS_ALLOWED_ORIGINS` is set to the exact frontend origin(s)
- [ ] `ENVIRONMENT` is `staging` or `production` (not `local` or `test`)
- [ ] No secrets are present in version-controlled files
- [ ] `deploy/secrets/env.*` files exist only on the deployment host, not in git
- [ ] Reverse proxy is configured and TLS certificate is valid
- [ ] Reverse proxy strips/re-sets `X-Forwarded-For` before it reaches the API
- [ ] PostgreSQL and Redis ports are not reachable from the public internet
- [ ] Admin port (:9091) is not reachable from the public internet

---

## Environment variable reference

See `deploy/env.production.example` and `deploy/env.staging.example` for the
full variable reference with inline documentation.

The authoritative variable list and default values are in
`apps/backend/internal/config/config.go`.

---

## Deployment procedure (self-hosted Docker Compose)

```bash
# 1. Build the API image on the CI host or deployment host
export VERSION=$(git describe --tags --always)
export COMMIT=$(git rev-parse --short HEAD)
export BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
export IMAGE_TAG=${VERSION}

docker build -f apps/backend/Dockerfile apps/backend/ \
  --build-arg VERSION=${VERSION} \
  --build-arg COMMIT=${COMMIT} \
  --build-arg BUILD_TIME=${BUILD_TIME} \
  -t dzeroth-api:${IMAGE_TAG}

# 2. Transfer the image to the deployment host (or push/pull from a registry)
# docker tag dzeroth-api:${IMAGE_TAG} registry.example.com/dzeroth-api:${IMAGE_TAG}
# docker push registry.example.com/dzeroth-api:${IMAGE_TAG}

# 3. On the deployment host — production
IMAGE_TAG=${IMAGE_TAG} docker compose -f docker-compose.prod.yml pull   # if using a registry
IMAGE_TAG=${IMAGE_TAG} docker compose -f docker-compose.prod.yml up -d

# 3. On the deployment host — staging
IMAGE_TAG=${IMAGE_TAG} docker compose -f docker-compose.staging.yml up -d

# 4. Verify health
docker compose -f docker-compose.prod.yml ps
docker exec $(docker compose -f docker-compose.prod.yml ps -q api) \
  wget -qO- http://localhost:8080/health
```

---

## Session cleanup graceful shutdown

The API binary runs a background `SessionCleanupWorker` that periodically
deletes expired session rows from PostgreSQL. The worker receives OS signals
via a context derived from the main application context.

When the API container stops (`docker compose stop`, `SIGTERM`, or
`SIGINT`), the main context is cancelled, the cleanup worker's `Run` loop
exits on the next tick, and the HTTP server drains in-flight requests before
the process exits. No additional compose configuration is required.

If the container is hard-killed (`SIGKILL`), the cleanup worker may be mid-batch.
The cleanup SQL uses a `ctid`-based DELETE that is safe to interrupt and re-run —
it is idempotent within a given batch and leaves no orphaned data.

---

## Volumes

| Volume | Purpose | Backup required |
|--------|---------|-----------------|
| `pgdata_prod` | PostgreSQL data | **Yes** |
| `redisdata_prod` | Redis AOF/RDB persistence | Recommended |
| `pgdata_staging` | Staging PostgreSQL | No |
| `redisdata_staging` | Staging Redis | No |

Production PostgreSQL backups are outside the scope of this compose file.
Use `pg_dump`, a managed database, or a volume snapshot strategy appropriate
for your infrastructure.

---

## PostgreSQL TLS and the bundled Compose service

The env templates (`deploy/env.production.example`, `deploy/env.staging.example`) default to
`POSTGRES_SSL_MODE=require`. This setting is intended for **managed PostgreSQL** services
(AWS RDS, Google Cloud SQL, Supabase, Neon, etc.) that configure server-side TLS automatically.

**The bundled `postgres:` service in `docker-compose.prod.yml` / `docker-compose.staging.yml`
uses the standard `postgis/postgis:15-3.3` image which starts with `ssl=off` in
`postgresql.conf`. Connecting with `POSTGRES_SSL_MODE=require` against this service will
cause the API to fail on startup with an SSL connection error.**

| PostgreSQL deployment | Correct `POSTGRES_SSL_MODE` |
|----------------------|----------------------------|
| Managed service (RDS, Cloud SQL, etc.) | `require` or `verify-full` |
| Bundled Compose service (no TLS configured) | `disable` |
| Bundled Compose service with custom certs mounted | `require` or `verify-full` |

To enable TLS on the bundled service you must:
1. Generate or obtain server certificate + key files
2. Mount them into the postgres container (e.g. `/etc/ssl/server.crt`, `/etc/ssl/server.key`)
3. Supply a custom `postgresql.conf` with `ssl = on`, `ssl_cert_file`, `ssl_key_file`
4. Add the mount and config to the compose postgres service definition

This additional configuration is outside the scope of the starter compose files.
The recommended production path is to use a managed PostgreSQL service.

---

## Replacing compose services with managed infrastructure

For production, consider replacing the compose-defined PostgreSQL and Redis
services with managed equivalents:

- **PostgreSQL**: AWS RDS for PostgreSQL 15+, Google Cloud SQL, Supabase, Neon
  - Set `POSTGRES_HOST`, `POSTGRES_PORT`, credentials, and `POSTGRES_SSL_MODE=require` or `verify-full`
  - Remove the `postgres` service and `pgdata_prod` volume from the compose file
- **Redis**: AWS ElastiCache, Redis Cloud, Upstash
  - Set `REDIS_ADDR`, `REDIS_PASSWORD`, and `REDIS_TLS=true`
  - Remove the `redis` service and `redisdata_prod` volume from the compose file

When using managed services, the API service only needs to connect to the
managed endpoints — the private network topology does not change for the API
itself.
