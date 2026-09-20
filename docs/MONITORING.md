# Dzeroth — Monitoring

**Status:** `PHASE 8D-4 — Monitoring documentation`

This document describes the Prometheus metrics endpoint, key metrics to alert on,
minimum alerting recommendations, and the reference Prometheus scrape configuration.

Grafana dashboards and alert rules are operational infrastructure and are not
provided in this repository. Operators should configure Prometheus alertmanager
and Grafana dashboards against the metrics described below.

---

## Prometheus metrics endpoint

The API exposes Prometheus metrics on the **admin listener** (default `:9091`).

```
GET http://<host>:9091/metrics
```

**This endpoint must never be exposed on the public API listener (port 8080).**
It is served only on the admin listener. Access it from monitoring infrastructure
on the private network, or via SSH tunnel from a staging host:

```bash
# Via docker exec (production — admin port not published):
docker exec <api-container> wget -qO- http://localhost:9091/metrics

# Via SSH tunnel (staging):
ssh -L 9091:localhost:9091 staging-host
curl http://localhost:9091/metrics
```

See `docs/DEPLOYMENT_TOPOLOGY.md` — Admin port (:9091).

---

## Registered metrics

Derived from `apps/backend/internal/platform/metrics/`.

### HTTP metrics

| Metric | Type | Labels | Description |
|---|---|---|---|
| `http_requests_total` | Counter | `method`, `route`, `status_code` | Total HTTP requests by method, normalized chi route pattern, and status code |
| `http_request_duration_seconds` | Histogram | `method`, `route`, `status_code` | HTTP request latency in seconds |

Route labels use normalized chi route patterns (e.g. `/posts/{postID}`), never raw URL paths.

### Authentication and rate-limit events

| Metric | Type | Labels | Description |
|---|---|---|---|
| `dzeroth_auth_events_total` | Counter | `event` | Auth events: `login_success`, `login_failure`, `token_refresh_success`, `token_refresh_failure`, `logout` |
| `dzeroth_rate_limit_events_total` | Counter | `category`, `result` | Rate-limit decisions (`allowed`/`rejected`) by category: `auth_login`, `auth_register`, `auth_refresh`, `post_write`, `follow`, `block`, `bookmark`, `search`, `reaction`, `report`, `studio` |

### Feed

| Metric | Type | Labels | Description |
|---|---|---|---|
| `dzeroth_feed_terminations_total` | Counter | `feed_type` | Feed termination events (finite feed boundary reached). `feed_type=home` |

### Infrastructure health

| Metric | Type | Labels | Description |
|---|---|---|---|
| `dzeroth_redis_up` | Gauge | — | Redis availability: 1 = available, 0 = unavailable (updated on every `/health` and `/readyz` call) |
| `dzeroth_redis_errors_total` | Counter | — | Redis infrastructure error counter. Registered; currently dormant pending a central Redis abstraction. See below. |
| `dzeroth_db_pool_connections_total` | Gauge | — | Total PostgreSQL pool connections (idle + acquired + constructing) |
| `dzeroth_db_pool_connections_acquired` | Gauge | — | PostgreSQL connections currently in use |
| `dzeroth_db_pool_connections_idle` | Gauge | — | Idle PostgreSQL connections available in the pool |
| `dzeroth_db_pool_connections_max` | Gauge | — | Configured maximum connections for the PostgreSQL pool |

### Build identity

| Metric | Type | Labels | Description |
|---|---|---|---|
| `dzeroth_build_info` | Gauge | `version`, `commit`, `build_time` | Build identity gauge; value is always 1. Use labels to identify the running version. |

---

## `dzeroth_redis_errors_total` — dormant counter

`dzeroth_redis_errors_total` is registered and exported but currently receives
no increments. Redis errors are logged at ERROR level by the rate-limit middleware
but are not yet wired to increment this counter. Wiring requires adding
`*InfraMetrics` to the `RateLimitMiddleware` signature or introducing a central
Redis error boundary — deferred to a future phase.

In the interim, use `dzeroth_redis_up` (gauge) and the Prometheus `up` metric
to detect Redis unavailability.

---

## Key metrics to alert on

These are prose recommendations. Configure Prometheus alertmanager rules to match
your operational SLA.

### Error rate

```promql
rate(http_requests_total{status_code=~"5.."}[5m]) > 0
```

Alert when the 5xx error rate is non-zero for a sustained period (e.g. 2 minutes).
A brief spike may be acceptable during deployment; sustained 5xx is a regression.

### Latency

```promql
histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m])) > 2.0
```

Alert when the p99 latency exceeds 2 seconds (the 2.5s histogram bucket boundary).
The Go API's handler timeout is 30 seconds; requests at 2s p99 are approaching
degraded territory.

### Process availability

```promql
up{job="dzeroth_api"} == 0
```

Alert immediately when the API process stops responding to Prometheus scrapes.

### Redis unavailability

```promql
dzeroth_redis_up == 0
```

Alert when Redis is unavailable. Auth rate-limiting fails closed (HTTP 503) when
Redis is down — this directly affects users.

### PostgreSQL pool saturation

```promql
dzeroth_db_pool_connections_acquired / dzeroth_db_pool_connections_max > 0.9
```

Alert when more than 90% of the PostgreSQL connection pool is in use. Pool
exhaustion causes request queuing and latency spikes.

### Auth failure rate

```promql
rate(dzeroth_auth_events_total{event="login_failure"}[5m]) > 10
```

Alert on sustained high login failure rate — may indicate a credential stuffing
attack or misconfigured client.

### Feed termination rate (informational)

```promql
rate(dzeroth_feed_terminations_total[5m])
```

Not an alert condition — use as an operational signal showing how often users
reach the finite feed boundary (the "Go Touch Grass" experience).

---

## Reference Prometheus scrape configuration

`monitoring/prometheus.yml` contains a reference Prometheus scrape config.

The scrape target `api:9091` resolves to the dzeroth-api container on the
shared Docker backend network. Prometheus must be on the same Docker network
to reach the admin port.

Adapt the configuration to your Prometheus deployment:
- If using a managed Prometheus (Grafana Cloud, AWS Managed Prometheus), configure
  the remote scrape endpoint or install the Prometheus agent on the host.
- If deploying Prometheus as a container, add it to the `docker-compose.prod.yml`
  on the same `backend` network.

---

## Prometheus scrape interval

The reference configuration uses `scrape_interval: 30s`. This is appropriate
for the current workload. The `/metrics` endpoint is served from in-memory
counters — no database or Redis I/O involved. Reducing to 15s is safe if higher
resolution alerting is required.

---

## Operational note

No Grafana dashboards, alert rule files, or alertmanager configuration are
provided in this repository. These are deployment-specific operational artifacts.

Recommended starting dashboards for operators:
- Import the "Go Processes" dashboard from Grafana.com (ID 6671) for process metrics.
- Build a custom dashboard for the `dzeroth_*` metrics listed above.
- Configure alertmanager with the alerting rules described in this document.
