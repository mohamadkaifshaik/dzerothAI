# Dzeroth Performance Baseline

> Phase 11C hardening artifact.
>
> All measurements marked **NOT YET EXECUTED** require an operator to run the
> commands against a live staging environment. Do not invent numbers.

---

## 1. Go micro-benchmarks

### Feed cursor encode/decode

These benchmarks measure the CPU and allocation cost of the opaque pagination
cursor cycle. They are deterministic and require no database connection.

**Command:**

```bash
cd apps/backend
go test -bench=. -benchmem ./internal/feed/...
```

**Benchmarks defined in** `apps/backend/internal/feed/benchmark_test.go`:

| Benchmark | Description |
|---|---|
| `BenchmarkFeedCursorEncode` | base64url encoding of a FeedCursor to opaque string |
| `BenchmarkFeedCursorDecode` | base64url decoding of an opaque string to FeedCursor |
| `BenchmarkFeedCursorRoundTrip` | combined encode + decode cycle |

**Results:** NOT YET EXECUTED — operator must run the command above.

---

## 2. k6 staging load test

**Script:** `scripts/load/k6_staging.js`

**Prerequisites:**
- k6 installed (`brew install k6` or https://k6.io/docs/get-started/installation/)
- A live staging environment
- A valid staging JWT access token for a seeded user

**Command:**

```bash
k6 run \
  -e BASE_URL=https://staging.example.com \
  -e API_TOKEN=<valid_staging_jwt> \
  scripts/load/k6_staging.js
```

**Scenarios:**

| Scenario | VUs | Duration | Description |
|---|---|---|---|
| `auth_login` | 5 | 30s | Auth error-path latency (invalid credentials) |
| `create_post` | 10 | 30s | Create original posts (authenticated) |
| `home_feed` | 10 | 30s | GET home feed (authenticated) |

**Thresholds (staging — not production SLOs):**

| Metric | Threshold |
|---|---|
| `http_req_duration p(95)` per scenario | < 2000ms |
| `http_req_failed` | < 5% |
| `feed_metric_leakage_violations` | == 0 (public metrics must not appear in responses) |

**Results:** EXECUTED — staging baseline, 2026-09-23.

> **Staging baseline only.** This is not a production performance benchmark
> and must not be used to infer throughput, capacity limits, or production
> capacity.

**Environment:** temporary AWS staging EC2.

**Workload:** `auth_login` 5 looping VUs, `create_post` 10 looping VUs,
`home_feed` 10 looping VUs, each for 30s (maximum 25 VUs).

**Run context:**
- First successful real-token staging run after correcting the `create_post`
  `responseCallback` in `scripts/load/k6_staging.js`.
- The `create_post` functional check accepts `201` or rate-limit `429`.
- The `create_post` `responseCallback` prevents expected `429` rate-limit
  responses from inflating `http_req_failed`.
- The 739 HTTP requests are **not** 739 successful `201` posts; the
  `create_post` workload can receive `429` responses due to rate limiting.

**Measurements:**

| Metric | Value |
|---|---|
| iterations | 739 complete, 0 interrupted |
| HTTP requests | 739 |
| `http_req_failed` | 0.00% (0/739) |
| checks | 1908/1908 passed (100%), 0 failed |
| `feed_metric_leakage_violations` | 0 |
| `feed_terminated` | 290 |
| `http_req_duration` p(95), overall | 39.46 ms |
| `http_req_duration` avg, overall | 25.35 ms |
| `http_req_duration` max, overall | 59.19 ms |
| `http_req_duration` p(95), `auth_login` | 40.19 ms |
| `http_req_duration` p(95), `create_post` | 37.92 ms |
| `http_req_duration` p(95), `home_feed` | 40.18 ms |
| `iteration_duration` p(95) | 1.06 s |
| data received | 1.4 MB |
| data sent | 325 kB |

**Threshold results:**

| Threshold | Result |
|---|---|
| `feed_metric_leakage_violations` count==0 | PASS |
| `http_req_duration{scenario:auth_login}` p(95) < 2000ms | PASS |
| `http_req_duration{scenario:create_post}` p(95) < 2000ms | PASS |
| `http_req_duration{scenario:home_feed}` p(95) < 2000ms | PASS |
| `http_req_failed` rate < 5% | PASS |

---

## 3. Baseline summary

| Component | Measurement | Value | Date |
|---|---|---|---|
| `BenchmarkFeedCursorEncode` | ns/op | NOT YET EXECUTED | — |
| `BenchmarkFeedCursorEncode` | B/op | NOT YET EXECUTED | — |
| `BenchmarkFeedCursorDecode` | ns/op | NOT YET EXECUTED | — |
| `BenchmarkFeedCursorDecode` | B/op | NOT YET EXECUTED | — |
| `BenchmarkFeedCursorRoundTrip` | ns/op | NOT YET EXECUTED | — |
| `create_post p(95) latency` | ms | 37.92 (staging) | 2026-09-23 |
| `home_feed p(95) latency` | ms | 40.18 (staging) | 2026-09-23 |
| `auth_login p(95) latency` | ms | 40.19 (staging) | 2026-09-23 |
| `http_req_failed rate` | % | 0.00 (0/739, staging) | 2026-09-23 |

---

## 4. Human operator steps required

1. Provision staging environment with the current backend image.
2. Seed at least one staging user with followers (so home feed returns posts).
3. Mint a staging JWT access token for the seeded user.
4. Run the Go benchmarks: `go test -bench=. -benchmem ./internal/feed/...`
5. Run the k6 script with real `BASE_URL` and `API_TOKEN`.
6. Record results in the table above, commit the update, and tag the baseline.

---

## 5. Notes

- These are **staging** benchmarks for baseline capture. Production SLOs are
  a separate concern and require production traffic profiling.
- The `feed_metric_leakage_violations` counter in the k6 script enforces the
  CLAUDE.md §2.3 invariant (no public social-validation metrics) under load.
- Do not disable thresholds to "make the test pass". Investigate failures.
