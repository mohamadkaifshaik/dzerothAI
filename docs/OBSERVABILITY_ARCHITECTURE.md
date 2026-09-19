# Observability Architecture

**Status:** `PLANNING — Phase 7C`
**Scope:** Backend only (`apps/backend/`). Frontend and ranking cron are out of scope for this phase.

---

## 1. Current Observability (What Actually Exists)

This section documents only what was confirmed by reading the source code.

### 1.1 Structured Logger

File: `apps/backend/cmd/api/main.go`, function `buildLogger`.

- Uses `go.uber.org/zap`.
- In `local` and `test` environments: `zap.NewDevelopmentConfig()` — colored console output, DEBUG-friendly.
- In all other environments: `zap.NewProductionConfig()` — JSON output suitable for log aggregation pipelines.
- Log level is read from `LOG_LEVEL` env var (default `info`). Parsed via `level.UnmarshalText`; invalid values silently fall back to INFO.
- The logger instance is passed explicitly via constructor injection to every service and handler. No global logger exists.

### 1.2 Startup Logging

Confirmed log events emitted at startup (all at INFO level):

| Event | Fields |
|---|---|
| `starting dzeroth api` | `environment`, `port` |
| `database connected` | — |
| `database migrations applied` | — |
| `redis connected` | `addr` |
| `redis unavailable at startup — rate limiting will fail closed` | `addr`, `error` (ERROR level) |
| `cors allowlist configured` | `origin_count` |
| `CORS_ALLOWED_ORIGINS is not set; all cross-origin requests will be blocked` | — (WARN level) |
| `http server listening` | `addr` |

Shutdown logging:

| Event | Fields |
|---|---|
| `shutdown signal received` | `signal` |
| `server stopped cleanly` | — |

No `version`, `git_commit`, or `build_time` fields are logged at startup.

### 1.3 HTTP Middleware Stack

File: `apps/backend/cmd/api/main.go`, router setup.

Middleware applied in order:

1. `chimw.RequestID` — generates or propagates `X-Request-ID` header and stores it in the request context under chi's own context key.
2. `chimw.RealIP` — populates `r.RemoteAddr` from `X-Real-IP` or `X-Forwarded-For`. Used by rate-limit IP extraction.
3. `chimw.Logger` — chi's built-in request logger. Emits one log line per request to `os.Stdout` using chi's own formatter, **not zap**. Fields include: method, path, status code, bytes written, elapsed time.
4. `chimw.Recoverer` — catches panics, logs a stack trace to `os.Stderr`, and responds HTTP 500.
5. `chimw.Timeout(30s)` — cancels the request context after 30 seconds.

**Critical gap:** `chimw.RequestID` stores the request ID in chi's context key, but no code propagates it into the zap logger or into downstream service log calls. Service errors logged via `s.log.Error(...)` have no request ID field.

**Critical gap:** `chimw.Logger` logs to stdout as plain text using chi's own format, not zap's JSON format. In production environments where `LOG_LEVEL` produces JSON zap output, the HTTP access log is plain text, which breaks log aggregation uniformity.

### 1.4 Health Endpoint

File: `apps/backend/cmd/api/main.go`, function `buildHealthHandler`.

Route: `GET /health` — unauthenticated, on the main port.

Behavior:
- Creates a 3-second timeout context.
- Calls `pool.Ping(ctx)` — if it fails, logs `health: database ping failed` at WARN with `error` field.
- Calls `platformRedis.IsAvailable(ctx, redisClient)` — performs a `PING` with a 1-second timeout. If the Redis client is `nil`, returns `false` immediately.
- When Redis is unavailable, no log is emitted from the health handler (the WARN log for Redis is only emitted at startup).

Response body:
```json
{"status":"ok|degraded","db":"ok|unavailable","redis":"ok|unavailable"}
```

HTTP status: 200 when all dependencies are healthy; 503 when any dependency is unavailable.

**No** `/livez` endpoint.
**No** `/readyz` endpoint.
**No** pgxpool connection statistics in the response.
**No** migration version in the response.

### 1.5 Rate-Limit Logging

Confirmed across `auth/middleware.go`, `post/handler.go`, `report/service.go`, `reaction/service.go`, `studio/service.go`:

| Situation | Level | Fields |
|---|---|---|
| Redis client nil (fail-closed path) | ERROR | `operation` or `user_id`, `reporter_id` |
| Redis SetNX failed (fail-closed) | ERROR | `operation`/`user_id`, `error` |
| Redis INCR failed (fail-closed) | ERROR | `operation`/`user_id`, `error` |
| Redis EXPIRE failed | ERROR | `key`, `error` |
| Redis check failed (fail-open path, reaction/studio) | WARN | `caller_id`, `error` |

**Notable:** auth rate limit middleware logs the client IP (`ip` field). No rate-limit-exceeded event is logged — the counter silently returns 429. There is no log message for "rate limit exceeded for this IP/user".

### 1.6 Auth Error Logging

File: `apps/backend/internal/auth/handler.go`.

- Validation errors, `UnauthorizedError`, `SuspendedError`, duplicate email/handle: **no log emitted** — these are mapped directly to 4xx responses.
- Unexpected internal errors: `auth: unexpected internal error` at ERROR with `error` field.

File: `apps/backend/internal/auth/middleware.go` (`JWTMiddleware`):

- Missing or malformed Authorization header: **no log emitted** — returns 401 directly.
- Invalid/expired token: **no log emitted** — returns 401 directly.
- Invalid UUID in subject: **no log emitted** — returns 401 directly.

**Gap:** Token forgery attempts (invalid signatures) are silently mapped to 401 with no log. This means anomalous auth behavior is invisible in production logs.

### 1.7 Feed Termination

File: `apps/backend/internal/feed/service.go`.

The constant `maxHomeFeedDepth = 200` is enforced at the repository level. When the feed is terminated, `PostPage.Terminated = true` is returned. No log or counter is emitted for feed termination events.

### 1.8 Notification Publish

File: `apps/backend/internal/notification/service.go`.

- Repository errors: `notification: publish` at ERROR with `error` field.
- Notification failures from reaction service: `reaction: publish notification failed` at WARN with `caller_id`, `post_id`, `error`.

### 1.9 Database Connection Pool

File: `apps/backend/internal/platform/db/db.go`.

Pool configuration: `MaxConns=25`, `MinConns=2`, `MaxConnLifetime=1h`, `MaxConnIdleTime=30m`.

`pgxpool.Pool` exposes `pool.Stat()` which returns `*pgxpool.Stat` with fields:
- `AcquireCount` (int64)
- `AcquiredConns` (int32) — connections currently in use
- `IdleConns` (int32) — idle connections
- `TotalConns` (int32) — total open connections
- `MaxConns` (int32) — configured max

None of these are currently logged or exposed.

### 1.10 Redis Client

File: `apps/backend/internal/platform/redis/redis.go`.

`IsAvailable` performs a `PING` with a 1-second timeout and returns bool. No pooling statistics are accessible from the `redis.Client` without additional instrumentation.

### 1.11 Background Jobs

`apps/backend/cmd/rank-cron/main.go` contains only `func main() { /* Phase 6 */ }` — an empty stub. No telemetry is needed.

`apps/backend/cmd/api/main.go` launches exactly one goroutine beyond the main startup goroutine: the HTTP server goroutine (`srv.ListenAndServe()`). No other background goroutines exist.

### 1.12 Migration Observability

`runMigrations` logs `database migrations applied` at INFO on success. On failure, the error propagates to `run()` and the process exits with a fatal log to stderr. The applied migration version number is not logged.

---

## 2. Gaps

Prioritized by impact.

### P0 — Critical (blocks production diagnoseability)

1. **No request ID propagation into service logs.** `chimw.RequestID` sets a request ID, but service-layer errors logged with `s.log.Error(...)` contain no request ID field. Correlating a user-reported error with a specific log event is impossible in production.

2. **HTTP access log is not structured JSON.** `chimw.Logger` writes plain text to stdout. In staging/production where zap emits JSON, the HTTP access log is unstructured and cannot be parsed by standard log aggregation pipelines.

3. **No metrics endpoint.** There is no way to scrape Dzeroth backend metrics. Request rate, error rate, and latency are completely invisible to any monitoring system without parsing log text.

4. **No `/livez` or `/readyz` separation.** A single `/health` endpoint conflates liveness (is the process alive) with readiness (are dependencies reachable). Kubernetes and load balancers require these to be separate: restarting a container because Redis is temporarily unreachable is incorrect behavior.

### P1 — High (significant observability gaps)

5. **Rate-limit-exceeded events are not logged.** When a caller exceeds a rate limit, a 429 is returned but no log event is emitted. Rate-limit abuse patterns are invisible.

6. **Auth token validation failures are not logged.** Clients presenting invalid, forged, or replayed JWTs receive a silent 401. Anomalous patterns (potential token enumeration or forgery) cannot be detected.

7. **Feed terminations are not counted.** The Dzeroth §2.1 finite-feed guarantee is a core product invariant. There is no counter confirming that terminations are actually occurring at the expected rate.

8. **No pgxpool stats exposure.** ~~Connection pool exhaustion is a common production failure mode. The pool's `Stat()` is never sampled or logged.~~ **RESOLVED in Phase 7C-4:** Four Prometheus gauges (`dzeroth_db_pool_connections_{total,acquired,idle,max}`) are now updated on every `/health` and `/readyz` call.

9. **No build/version identity at startup.** There is no `version` or `git_commit` logged at startup, making it impossible to determine which deployed artifact is running from logs alone.

### P2 — Medium

10. **Redis health check in `/health` has no log when Redis is unavailable at health-check time.** Only a startup log is emitted. Repeated calls to `/health` that show Redis unavailable are silent in the log.

11. **Notification publish failures are WARN in reaction but ERROR in notification.** Inconsistent classification makes alert rules harder to write.

12. **Studio rate-limit check failure is WARN** (fail-open path). This is correct behavior but the inconsistency between packages (some use `logger`, some use `log`) is a minor cleanup item.

---

## 3. Goals

1. Every production request must be traceable from HTTP layer through service layer using a single correlation ID.
2. Request error rate and latency must be scrapable by a metrics system without parsing log text.
3. Dependency health (DB, Redis) must be independently queryable for liveness vs readiness purposes.
4. Auth anomalies (repeated JWT forgery, unusual token failures) must appear in logs at WARN or ERROR level.
5. Rate-limit events must be counted and logged.
6. Feed terminations must be counted (confirms Dzeroth §2.1 invariant is operating).
7. DB pool health must be periodically sampled.
8. Redis availability changes (up → down, down → up) must be logged at WARN.
9. Build identity (version, git commit) must appear in startup logs.
10. All telemetry must be safe: no passwords, tokens, post content, or private user data.

---

## 4. Non-Goals

- **No ranking metrics.** `cmd/rank-cron` is an empty stub. Ranking observability is permanently deferred until Phase 6.
- **No distributed tracing infrastructure.** OpenTelemetry trace propagation across service boundaries is out of scope for Phase 7C. A single request ID field in logs is the tracing primitive for this phase.
- **No external observability platform dependency at compile time.** The backend must not require a specific SaaS platform (Datadog, New Relic, etc.) to build or run.
- **No per-query database logging.** Per-row query logs would be prohibitively noisy and are not planned.
- **No Flutter / mobile observability.** This document covers the Go backend only.

---

## 5. Logging Architecture

### 5.1 Existing Zap Fields (Confirmed)

These fields are currently in use:

| Package | Logger field | Type | Notes |
|---|---|---|---|
| startup | `environment` | string | `local`, `test`, `staging`, `production` |
| startup | `port` | string | API port |
| startup | `addr` | string | Redis addr, HTTP addr |
| startup | `origin_count` | int | CORS origins count |
| startup | `signal` | string | SIGTERM / SIGINT |
| health | `error` | error | DB ping failure |
| rate limit | `operation` | string | auth middleware |
| rate limit | `ip` | string | auth middleware only |
| rate limit | `user_id` | string | post handler |
| rate limit | `reporter_id` | string | report service |
| rate limit | `caller_id` | string | reaction/studio |
| rate limit | `key` | string | Redis key (no value) |
| rate limit | `error` | error | Redis errors |
| feed | `caller_id` | Stringer | UUID |
| feed | `error` | error | |
| post | `post_id` | string | UUID |
| post | `caller_id` | string | UUID |
| notification | `error` | error | |
| studio | `caller_id` | string | UUID |
| studio | `key` | string | Redis key |
| auth | `error` | error | internal errors only |

### 5.2 Recommended Additional Fields

The following fields should be added as part of Phase 7C implementation work:

**Per-request context fields (via request-scoped logger or middleware-injected context):**

| Field | Value | Where |
|---|---|---|
| `request_id` | chi's `chimw.GetReqID(ctx)` | Every service log call that processes a request |
| `route` | chi's normalized route pattern (e.g., `/posts/{postID}`) | HTTP access log |
| `method` | HTTP method | HTTP access log |
| `status` | HTTP response status code | HTTP access log |
| `duration_ms` | Request duration in milliseconds | HTTP access log |

**Startup fields:**

| Field | Value |
|---|---|
| `version` | Binary version string (from `-ldflags` or env) |
| `git_commit` | Git commit SHA |

**Auth-specific fields (WARN level, no sensitive data):**

| Field | Value | Notes |
|---|---|---|
| `auth_result` | `success` / `failure` | Login/refresh outcome |
| `failure_reason` | `invalid_credentials` / `expired_token` / `token_not_found` / `suspended` | Category only, no user content |

### 5.3 Log Level Assignment Per Error Class

| Error class | Level | Rationale |
|---|---|---|
| `CodeValidation` — user input errors | INFO | Expected user behavior, high volume, not actionable for ops |
| `CodeNotFound` — resource not found | INFO | Expected, not actionable |
| `CodeUnauthorized` — missing/expired token | INFO | Expected (session expiry is normal) |
| Auth failure — wrong password, token not found | WARN | Elevated: single events are normal, patterns are suspicious |
| Auth anomaly — malformed JWT, invalid signature | WARN | Should appear in logs for anomaly detection |
| `CodeForbidden` — authorization check failure | WARN | Access control boundary crossed |
| Rate-limit exceeded | INFO | With counter increment; individual events are normal |
| Redis errors (fail-open path: reaction, studio) | WARN | Service degraded but continues |
| Redis errors (fail-closed path: auth, post, report) | ERROR | Service unavailable response issued |
| DB errors | ERROR | Unexpected infrastructure failure |
| `CodeInternal` / unhandled errors | ERROR | All unexpected internal errors |
| Panic (chimw.Recoverer) | ERROR | Already handled by chi; results in 500 |

### 5.4 Fields That Must NOT Appear in Logs

- Passwords (in any form, including after encoding)
- JWT tokens (access or refresh)
- Session token hashes
- User email addresses
- Post content body
- JWT secret key material
- Database DSN (contains password)
- Redis connection strings if they contain auth tokens
- Private IP addresses of internal services (use service names)

The `ip` field in `auth/middleware.go` rate-limit logs contains the client IP address. This is currently acceptable because: (a) it is only logged for Redis infrastructure errors, not for every request, and (b) the rate-limit key itself uses the IP. Production deployments should ensure log access is restricted to authorized personnel.

### 5.5 Request ID Propagation

The intended propagation chain:

```
HTTP request → chimw.RequestID sets X-Request-ID
             → chimw.GetReqID(r.Context()) extracts it
             → Middleware creates a request-scoped zap.Logger with zap.String("request_id", reqID)
             → Request-scoped logger is stored in context
             → Handlers and services extract logger from context for log calls
```

Implementation note: this requires either:
- A helper function `LoggerFromContext(ctx) *zap.Logger` that falls back to the global logger if none is set.
- Or passing the request ID as a field when constructing the per-handler logger at the start of each handler method.

The second approach (no context-stored logger) is simpler and preferred for Phase 7C to avoid touching every service signature.

---

## 6. Health and Readiness Architecture

### 6.1 `GET /health` (Exists — Improve)

Current behavior (confirmed in `buildHealthHandler`):
- 3-second context timeout.
- Pings PostgreSQL with `pool.Ping(ctx)`.
- Checks Redis with `platformRedis.IsAvailable(ctx, redisClient)` (1-second inner timeout).
- Returns `{"status":"ok|degraded","db":"ok|unavailable","redis":"ok|unavailable"}`.
- HTTP 200 on healthy; 503 on any degraded dependency.

Recommended improvements:
- Add optional `db_pool` object when pool stats are available: `{"total": 5, "idle": 3, "in_use": 2, "max": 25}`. This is read from `pool.Stat()` and costs nothing (no I/O).
- Log a WARN when Redis is unavailable at health-check time (currently only logged at startup).
- Keep the 3-second timeout. Do not increase it.
- Do not expose: DSN, hostnames, passwords, Redis keyspace data, migration SQL.

Revised response shape:
```json
{
  "status": "ok|degraded",
  "db": "ok|unavailable",
  "redis": "ok|unavailable",
  "db_pool": {
    "total": 5,
    "idle": 3,
    "in_use": 2,
    "max": 25
  }
}
```

`db_pool` is omitted if the pool ping fails (stats would be misleading).

### 6.2 `GET /livez` (New)

Purpose: Kubernetes liveness probe. If this fails, Kubernetes restarts the container.

Behavior:
- Returns HTTP 200 immediately with `{"status":"alive"}`.
- No dependency checks.
- No timeout required.

Rationale: Liveness must not check dependencies. If Redis is down, the process is still alive and should not be restarted. Only check that the HTTP server goroutine is running (which is implicit if this endpoint responds).

### 6.3 `GET /readyz` (New)

Purpose: Kubernetes readiness probe and load-balancer health gate. If this fails, the load balancer stops sending traffic to this instance.

Behavior:
- 3-second timeout context.
- Checks `pool.Ping(ctx)` — PostgreSQL must be reachable.
- Checks `platformRedis.IsAvailable(ctx, redisClient)` — Redis must be reachable.
- Returns HTTP 200 with `{"status":"ready"}` only if both pass.
- Returns HTTP 503 with `{"status":"not_ready","db":"unavailable"|"ok","redis":"unavailable"|"ok"}` otherwise.

Rationale: A new instance should not receive production traffic until its dependencies are confirmed reachable.

Do not expose: migration version, pool stats, internal hostnames.

### 6.4 Authorization for Health Endpoints

All three endpoints (`/health`, `/livez`, `/readyz`) must remain unauthenticated. Ops tooling, load balancers, and Kubernetes probes cannot present JWT tokens.

Production network policy should restrict `/readyz` and `/livez` to the internal network or load balancer IP range where possible, but this is an infrastructure concern, not an application concern.

---

## 7. Metrics Architecture

### 7.1 Metrics Library Decision (OPEN)

See Section 17 (Open Decisions). The candidate evaluation is documented here; the final choice requires team confirmation.

**Option A: `prometheus/client_golang`**
- Industry standard; integrates with Grafana, alerting systems, Kubernetes operator ecosystem.
- `~500KB` additional binary size.
- Prometheus exposition format is a de facto standard; other systems (Datadog Agent, OpenTelemetry Collector) can scrape it.
- Recommendation: preferred for Phase 7C.

**Option B: Lightweight custom counters with JSON export**
- No external dependency.
- Minimal binary impact.
- Not compatible with standard monitoring infrastructure without custom work.
- Only appropriate if zero external dependencies is a hard constraint.

**Option C: OpenTelemetry SDK**
- Future-proof: supports metrics, traces, and logs under one API.
- Significantly more complex setup; requires an OTLP collector or compatible backend.
- Deferred to a future phase.

The remainder of this section assumes Option A (Prometheus client).

### 7.2 HTTP Metrics

**`http_requests_total`** — Counter

| Label | Values | Cardinality note |
|---|---|---|
| `method` | GET, POST, PUT, DELETE, OPTIONS | 5 values |
| `route` | Normalized chi route pattern: `/posts/{postID}`, `/api/v1/auth/login` | ~30 values (bounded by route count) |
| `status_code` | 200, 201, 204, 400, 401, 403, 404, 422, 429, 500, 503 | ~11 values |

Total cardinality: approximately 5 × 30 × 11 = 1,650 series. Acceptable.

**Important:** Route labels must use chi's pattern (`/posts/{postID}`), NOT the raw URL path (`/posts/abc-123-def`). Raw paths produce unbounded cardinality. Use `chi.RouteContext(r.Context()).RoutePattern()` to extract the normalized pattern.

**`http_request_duration_seconds`** — Histogram

Same labels as above.

Recommended buckets: `[0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5]` seconds.

These buckets cover the range from cache hits (~5ms) through acceptable API latency (250ms) through degraded (1s) to critically slow (2.5s). Requests that exceed 2.5s fall into the `+Inf` bucket.

### 7.3 Auth Metrics

**`auth_login_attempts_total`** — Counter

| Label | Values |
|---|---|
| `result` | `success`, `invalid_credentials`, `suspended`, `internal_error` |

No user_id, no email, no IP. `result` has bounded values.

**`auth_token_refresh_total`** — Counter

| Label | Values |
|---|---|
| `result` | `success`, `not_found`, `expired`, `suspended`, `internal_error` |

**`auth_registration_total`** — Counter

| Label | Values |
|---|---|
| `result` | `success`, `duplicate_email`, `duplicate_handle`, `validation_error`, `internal_error` |

### 7.4 Rate-Limit Metrics

**`rate_limit_blocks_total`** — Counter

| Label | Values |
|---|---|
| `endpoint` | Normalized route pattern (same as HTTP metric) |

No user_id, no IP. Cardinality bounded by route count.

### 7.5 Database Metrics

**`db_pool_connections`** — Gauge

| Label | Values | Source |
|---|---|---|
| `state` | `idle`, `in_use`, `total` | `pool.Stat().IdleConns`, `AcquiredConns`, `TotalConns` |

Collected from `pool.Stat()` which is a non-blocking in-memory read.

**`db_errors_total`** — Counter

| Label | Values |
|---|---|
| `operation` | `query`, `exec`, `begin`, `scan` |

This requires instrumentation at the pgxpool level (query tracer) or at individual repository call sites. A pgxpool `QueryTracer` is the preferred approach as it avoids modifying every repository.

### 7.6 Redis Metrics

**`redis_errors_total`** — Counter

| Label | Values |
|---|---|
| `command` | `set_nx`, `incr`, `expire`, `ping`, `zadd`, `zrange` |

**`dzeroth_redis_up`** — Gauge **[IMPLEMENTED in Phase 7C-4]**

Value: `1.0` when `platformRedis.IsAvailable` returns true; `0.0` otherwise. Updated during `/health` and `/readyz` calls (piggybacked — no separate goroutine).

### 7.7 Feed Metrics

**`feed_terminations_total`** — Counter

| Label | Values |
|---|---|
| `feed_type` | `home`, `hashtag`, `author`, `thread` |

This counter is incremented when `PostPage.Terminated == true` is returned to a caller. It is a direct observable signal confirming Dzeroth §2.1 (finite feed) is functioning.

### 7.8 Notification Metrics

**`notification_publish_total`** — Counter

| Label | Values |
|---|---|
| `event_type` | `reply`, `mention`, `follow`, `reaction` |
| `result` | `success`, `suppressed_self`, `error` |

`suppressed_self` counts the self-notification suppression path in `notification/service.go`.

### 7.9 Metrics Endpoint

Route: `GET /metrics` — Prometheus text exposition format.

**Port decision (OPEN — see Section 17):**

Option A (same port as API): simplest, no new listener. Production must restrict access via network policy or load-balancer rule.

Option B (separate admin port, e.g., `:9090`): cleaner security boundary; `/metrics` never reaches the public load balancer. Requires a second `http.Server` in `main.go`.

For Phase 7C: same port. Document that production infrastructure must prevent public exposure of `/metrics`.

The `/metrics` endpoint must not require authentication (Prometheus scrapers typically cannot present JWT tokens). Access control is a network-layer concern.

---

## 8. Error Observability

Mapping from `apierror` codes to log levels and counter behavior:

| Code | HTTP | Log level | Counter action |
|---|---|---|---|
| `CodeValidation` | 400 | None (expected user error) | Counted in `http_requests_total{status_code="400"}` |
| `CodeUnauthorized` | 401 | INFO for normal token expiry; WARN for malformed/forged token | `auth_*` counter |
| `CodeForbidden` | 403 | WARN | `http_requests_total{status_code="403"}` |
| `CodeNotFound` | 404 | None | `http_requests_total{status_code="404"}` |
| `CodeConflict` | 409 | None | `http_requests_total{status_code="409"}` |
| `CodeRateLimit` | 429 | INFO with endpoint field | `rate_limit_blocks_total` counter |
| `CodeInternal` | 500 | ERROR | `http_requests_total{status_code="500"}` |
| `CodeServiceUnavailable` | 503 | ERROR | `http_requests_total{status_code="503"}` |

The HTTP metrics middleware handles the status code counters automatically. The auth and rate-limit counters require instrumentation at the handler or service layer.

---

## 9. Database Observability

### 9.1 Pool Statistics

`pgxpool.Pool.Stat()` returns `*pgxpool.Stat`. Confirmed fields relevant to production monitoring:

```go
type Stat struct {
    AcquireCount            int64   // total successful acquires
    AcquireDuration         time.Duration
    AcquiredConns           int32   // currently in use
    CanceledAcquireCount    int64   // acquires canceled due to context
    ConstructingConns        int32  // connections being established
    EmptyAcquireCount       int64   // acquires that waited for an available connection
    IdleConns               int32   // idle (available) connections
    MaxConns                int32   // configured maximum
    TotalConns              int32   // idle + acquired + constructing
}
```

Key signals:
- `AcquiredConns / MaxConns` approaching 1.0 indicates pool pressure.
- `EmptyAcquireCount` increasing over time indicates pool is a bottleneck.
- `CanceledAcquireCount` increasing indicates request context timeouts before DB connection acquired.

### 9.2 Periodic Pool Logging

Recommended: emit pool stats at DEBUG level every 5 minutes in all environments. At INFO level only when `AcquiredConns / MaxConns > 0.8` (connection pressure threshold).

This requires a background goroutine in `cmd/api/main.go`. It should be bounded by a context that is canceled on shutdown.

In Phase 7C: implement as `db_pool_connections` Prometheus gauge sampled from a periodic goroutine, rather than a logging approach, to keep the log stream clean.

### 9.3 Timeouts

The 30-second HTTP server `WriteTimeout` is the outer bound for any query. The 30-second router `chimw.Timeout(30s)` cancels the request context. These already exist.

Connection acquisition timeouts from `pgxpool` are surfaced as context deadline errors and should be logged at WARN level with the `request_id` field.

---

## 10. Redis Observability

### 10.1 Current Behavior

`platformRedis.IsAvailable(ctx, client)` in `apps/backend/internal/platform/redis/redis.go`:
- Returns `false` if `client == nil`.
- Performs `client.Ping(pingCtx)` with a 1-second timeout.
- Returns `true` only if `Ping` returns no error.

No state is tracked between calls. Every call is a fresh Ping.

### 10.2 Recommended Improvements

**Startup:** Already logged correctly. Confirmed: `redis connected` at INFO, `redis unavailable at startup` at ERROR.

**Health check:** When `/health` or `/readyz` finds Redis unavailable, log `health: redis unavailable` at WARN. Currently no log is emitted for Redis unavailability in the health handler (only the DB ping failure is logged).

**State change detection:** Track last-known Redis availability state in a package-level atomic bool within `platformRedis`. On first failure: log WARN `redis: became unavailable`. On first recovery: log INFO `redis: became available`. This avoids log noise from repeated health checks finding Redis consistently in the same state.

This state-change tracking is a Phase 7C-5 item and requires a small addition to `platform/redis`.

**Redis error counter:** Increment `redis_errors_total{command=...}` at each Redis error site in rate-limit code. These currently only log at WARN/ERROR; the counter makes them alertable.

---

## 11. Background Job Observability

**`cmd/rank-cron/main.go`** is confirmed to be an empty stub (`func main() { /* Phase 6 */ }`). No telemetry is needed or planned for it in this phase.

**`cmd/api/main.go`** background goroutines:
1. The HTTP server goroutine: `go func() { srv.ListenAndServe() }()` — this is the only goroutine beyond the main startup goroutine. It is not a background job; it is the main serving path.
2. No other goroutines are launched in `cmd/api/main.go`.

If a periodic pool-stat goroutine is added as part of Phase 7C-5, it should:
- Receive a `context.Context` that is canceled on shutdown.
- Log startup and shutdown at DEBUG level.
- Not emit any sensitive data.

---

## 12. Privacy and Security Requirements for Telemetry

Each telemetry data point is classified below.

| Data point | Safe? | Notes |
|---|---|---|
| `http_requests_total{method, route, status_code}` | Yes | Route is a pattern, not raw URL |
| `http_request_duration_seconds{method, route, status_code}` | Yes | Same as above |
| `auth_login_attempts_total{result}` | Yes | Result is categorical; no email or IP |
| `auth_token_refresh_total{result}` | Yes | Categorical only |
| `rate_limit_blocks_total{endpoint}` | Yes | Endpoint is a normalized pattern |
| `db_pool_connections{state}` | Yes | Infrastructure metric only |
| `db_errors_total{operation}` | Yes | Operation type only |
| `redis_errors_total{command}` | Yes | Command type only |
| `redis_available` | Yes | Boolean gauge |
| `feed_terminations_total{feed_type}` | Yes | Feed type is categorical |
| `notification_publish_total{event_type, result}` | Yes | Event type and result are categorical |
| Log: `ip` in auth rate-limit | Acceptable with restrictions | Client IP; only logged on Redis errors, not per-request. Production log access must be restricted. |
| Log: `caller_id` / `user_id` | Acceptable | Internal UUID; not PII equivalent in the same way as email |
| Log: `error` field | Acceptable | Error messages must never include passwords, tokens, or post content |
| Log: `addr` (Redis address) | Acceptable | Does not include auth credentials |
| Log: DB ping error | Acceptable | Must not include DSN; `pool.Ping` errors expose only connectivity status |

**Explicit prohibitions confirmed for this codebase:**
- The `PostgresDSN` (which contains `POSTGRES_PASSWORD`) is constructed in `config.Load()` and passed to `platformDB.Connect()` and `runMigrations()`. It must never be logged. Currently it is not logged.
- `JWTSecret` is passed as `[]byte` through the wire and never logged. Confirmed correct.
- `body.Password` in `auth/handler.go` `login()` is passed to `bcrypt.CompareHashAndPassword` and never logged. Confirmed correct.
- `rawRefreshToken` in `auth/service.go` `Refresh()` is immediately hashed; the raw token is never logged. Confirmed correct.

---

## 13. Performance Considerations

**`chimw.Logger`:** Per-request I/O write to stdout. Acceptable overhead for typical social API throughput. The switch to structured logging (replacing `chimw.Logger` with a zap middleware) may slightly increase allocation per request due to field construction but reduces downstream parsing cost.

**Prometheus histogram:** Each observation allocates a lock acquisition on the histogram's mutex. For a histogram with ~1,650 series and typical request rates of hundreds per second, this is well within acceptable bounds. Prometheus client_golang's implementation is optimized for this pattern.

**Health check (3-second timeout):** The DB ping sends a lightweight query to PostgreSQL. At typical health check intervals (every 10–30 seconds), this is negligible. Do not reduce the timeout below 1 second; network hiccups can cause false failures.

**`pool.Stat()`:** In-memory read with a mutex. No I/O. Zero meaningful overhead.

**Avoid:**
- Per-row query logging at INFO or above.
- Calling `pool.Stat()` per request (only in the periodic goroutine or health handler).
- Histogram label values that include request IDs or user IDs (unbounded cardinality).
- Any telemetry operation in the hot path that makes a network call (exception: health check endpoint, which is called infrequently).

---

## 14. Test Strategy

### 14.1 Health Endpoint Tests

Test file to create: `apps/backend/cmd/api/health_test.go`

Required test cases:
- `TestHealth_AllHealthy`: mock pool Ping returns nil, Redis Ping returns nil → HTTP 200, `{"status":"ok","db":"ok","redis":"ok"}`.
- `TestHealth_DBUnavailable`: mock pool Ping returns error → HTTP 503, `{"status":"degraded","db":"unavailable","redis":"ok"}`.
- `TestHealth_RedisUnavailable`: Redis client is nil → HTTP 503, `{"status":"degraded","db":"ok","redis":"unavailable"}`.
- `TestHealth_BothUnavailable`: both fail → HTTP 503, `{"status":"degraded","db":"unavailable","redis":"unavailable"}`.

### 14.2 Liveness and Readiness Tests

- `TestLivez_AlwaysOK`: `/livez` returns HTTP 200 regardless of dependency state.
- `TestReadyz_AllHealthy`: HTTP 200.
- `TestReadyz_DBDown`: HTTP 503 with `db:unavailable`.
- `TestReadyz_RedisDown`: HTTP 503 with `redis:unavailable`.

### 14.3 Metrics Tests

- Route normalization: a request to `/posts/some-uuid-value` must result in the metric label `route=/posts/{postID}`, not `/posts/some-uuid-value`. This is the highest-cardinality risk.
- Counter increment: confirm `http_requests_total` increments correctly after a request.
- Sensitive field exclusion: confirm no metric label contains a user ID or raw path parameter value.

### 14.4 Logging Sensitive Field Tests

- Confirm that a failed login attempt does not log the supplied password.
- Confirm that a rate-limit Redis error does not log the JWT secret or token value.
- These can be implemented as unit tests using a `zaptest.NewLogger` observer and asserting that logged fields do not contain known sensitive strings.

### 14.5 Existing Tests

No existing test covers `/health`, `zap` logger fields, or metrics. The 23 test files found cover handler and service logic, not observability components.

---

## 15. Environment Strategy

Configuration for telemetry per environment, consistent with `ENVIRONMENT_STRATEGY.md` and `config.go`:

| Concern | `local` / `test` | `staging` | `production` |
|---|---|---|---|
| Log format | Development (colored console, zap dev config) | JSON (zap production config) | JSON (zap production config) |
| Log level | DEBUG (set via `LOG_LEVEL=debug`) | INFO | INFO |
| HTTP access log | chi's plain-text logger (acceptable locally) | Structured zap middleware | Structured zap middleware |
| Health endpoints | `/health`, `/livez`, `/readyz` all on main port | Same | Same |
| Metrics endpoint | `/metrics` on main port | `/metrics` on main port | `/metrics` on main port; **restrict to internal network via infrastructure policy** |
| Pool stat goroutine | Disabled or very verbose | Enabled | Enabled |
| Build identity in logs | Optional (dev convenience) | Required | Required |

No new environment variables are required by the logging or health architecture beyond `LOG_LEVEL` (already exists) and `ENVIRONMENT` (already exists).

If a build version is added, recommended env var: `BUILD_VERSION` (optional, defaults to `unknown`). This follows the existing naming convention in `config.go`.

---

## 16. Implementation Phases

Work is broken into discrete, independently reviewable tasks. Each phase produces a testable increment.

### Phase 7C-1: Improve `GET /health`

Files: `apps/backend/cmd/api/main.go`

- Add `db_pool` field to health response using `pool.Stat()` (only when DB ping succeeds).
- Add WARN log in health handler when Redis is unavailable.
- Add test: `apps/backend/cmd/api/health_test.go`.

### Phase 7C-2: Add `/livez` and `/readyz`

Files: `apps/backend/cmd/api/main.go`

- Register `GET /livez` — immediate 200.
- Register `GET /readyz` — DB + Redis checks with 3s timeout, same logic as `/health` but separate response shape.
- Add tests for all status combinations.

### Phase 7C-3: Structured HTTP Access Log (Replace `chimw.Logger`)

Files: `apps/backend/cmd/api/main.go`

- Remove `chimw.Logger` from middleware stack.
- Implement a zap-based HTTP logging middleware that emits a single structured log per request at INFO level with fields: `request_id`, `method`, `route`, `status`, `duration_ms`, `bytes`.
- `route` must use `chi.RouteContext(r.Context()).RoutePattern()` to get the normalized pattern.
- This is a prerequisite for Phase 7C-4 (metrics route normalization uses the same pattern extraction).

### Phase 7C-4: Prometheus HTTP Metrics Middleware

**IMPLEMENTED** — `apps/backend/cmd/api/main.go`, `apps/backend/internal/platform/metrics/metrics.go`

- `http_requests_total` counter and `http_request_duration_seconds` histogram registered.
- Middleware records both metrics per request using normalized chi route patterns as labels.
- `GET /metrics` served on admin listener only (`:9091`), never on the public API port.

### Phase 7C-4 (extension): DB Pool and Redis Gauges

**IMPLEMENTED** — `apps/backend/internal/platform/metrics/infra.go`, `apps/backend/internal/platform/metrics/infra_test.go`, `apps/backend/cmd/api/main.go`

DB pool gauges (no labels — zero cardinality):

| Metric name | pgxpool.Stat field | Semantics |
|---|---|---|
| `dzeroth_db_pool_connections_total` | `TotalConns()` | idle + acquired + constructing |
| `dzeroth_db_pool_connections_acquired` | `AcquiredConns()` | connections actively in use |
| `dzeroth_db_pool_connections_idle` | `IdleConns()` | connections available in the pool |
| `dzeroth_db_pool_connections_max` | `MaxConns()` | configured pool maximum |

Redis gauge (no labels — zero cardinality):

| Metric name | Semantics |
|---|---|
| `dzeroth_redis_up` | 1.0 = available (last health check succeeded), 0.0 = unavailable |

Update strategy: gauges are updated by piggybacking on `/health` (both public and admin) and `/readyz` (admin) handler calls. No separate background goroutine is introduced. The gauge reflects the pool state and Redis availability at the last health check invocation.

Failure behavior:
- If the DB ping fails, pool gauges are NOT updated (stats from a closed or unhealthy pool would be misleading). The last known good values are retained.
- If Redis is unavailable, `dzeroth_redis_up` is set to 0.0 on every `/health` or `/readyz` call.
- If `*InfraMetrics` is nil (e.g., in tests that do not exercise metrics), all update calls are no-ops — no panic.

11 tests in `apps/backend/internal/platform/metrics/infra_test.go` cover all four DB pool gauges, Redis up/down/transition, nil-safety, and zero-label cardinality.

### Phase 7C-5: Auth, Rate-Limit, Feed, and Notification Counters

Files: relevant handler and service files (targeted edits only)

- `auth_login_attempts_total`, `auth_token_refresh_total`, `auth_registration_total` — increment in `auth/handler.go` `handleServiceError`.
- `rate_limit_blocks_total` — increment in `auth/middleware.go` and `post/handler.go` when 429 is issued.
- `feed_terminations_total` — increment in `feed/service.go` when `Terminated == true`.
- `notification_publish_total` — increment in `notification/service.go` `Publish`.
- Auth WARN logs for malformed/forged JWTs — add to `auth/middleware.go` `JWTMiddleware`.
- Rate-limit exceeded INFO log — add at the `count > maxAttempts` branch in each rate-limit implementation.

### Phase 7C-6: DB Pool and Redis Metrics

Files: `apps/backend/cmd/api/main.go`, `apps/backend/internal/platform/redis/redis.go`

- Add `db_pool_connections` gauge populated from `pool.Stat()` in a periodic goroutine (5-minute interval).
- Add `redis_available` gauge updated in health check and `/readyz` handler.
- Add `redis_errors_total` counter at Redis error sites in rate-limit code.
- Add Redis state-change detection (first failure / first recovery WARN logs) to `platformRedis`.

### Phase 7C-7: Request ID Propagation and Build Identity

Files: `apps/backend/cmd/api/main.go`

- At startup, log `version` and `git_commit` fields (read from `BUILD_VERSION` env var and embedded via `-ldflags`).
- In the zap HTTP logging middleware (from Phase 7C-3), extract `chimw.GetReqID(r.Context())` and include it in the access log.
- Optionally: thread the request ID into service-layer logs for the most critical paths (auth, feed) without changing service signatures — pass it as a context-extracted field at the handler level.

### Phase 7C-8: Tests for All New Components

- Complete test coverage for all phases above.
- Sensitive field exclusion tests using `zaptest`.
- Metrics cardinality test (route normalization).

---

## 17. Open Decisions

These are genuinely unresolved questions that require team input before implementation begins.

### OD-1: Metrics Library Choice

Three options evaluated in Section 7.1. Recommendation is Option A (Prometheus `client_golang`), but this introduces a new dependency. Requires team confirmation.

**Decision needed:** Which metrics library?

### OD-2: `/metrics` Port

Should `/metrics` be served on the same port as the API (`:8080`) or on a separate admin port (e.g., `:9090`)?

Same port: simpler, no second listener.
Separate port: cleaner security — the metrics endpoint never reaches the public load balancer by design.

For Phase 7C, same port is proposed with production network policy restricting access. The separate admin port design is a Phase 8 infrastructure concern.

**Decision needed:** Same port acceptable for Phase 7C?

### OD-3: Monitoring Backend

The backend is not required to run or integrate with a specific monitoring system. But the implementation choices depend on what will eventually scrape `/metrics`:

- A Prometheus server scraping on schedule.
- A push gateway (for short-lived processes; not relevant for a long-running API).
- A vendor agent (Datadog, Grafana Alloy, etc.) that can scrape Prometheus format.
- No external system yet (metrics endpoint exists but nothing reads it).

**Decision needed:** What is the intended metrics consumer for staging and production?

### OD-4: Distributed Tracing Timing

OpenTelemetry trace context propagation across requests was explicitly excluded from Phase 7C. Should it be planned for Phase 8, or deferred indefinitely until the system is deployed and the need is confirmed?

**Decision needed:** Plan OTel tracing for Phase 8 or defer to confirmed need?

### OD-5: Log Aggregation Platform

The zap JSON log format is compatible with most log aggregation systems (Elasticsearch/Kibana, Loki/Grafana, Splunk, CloudWatch). The implementation has no dependency on a specific platform. But the intended platform affects decisions about log field naming conventions.

**Decision needed:** What log aggregation system is targeted for staging and production?
