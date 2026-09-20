# Dzeroth Roadmap

**Status:** `PHASE 1 COMPLETE — Phase 2 complete — Phase 3 complete — Phase 4 complete — Phase 5 complete — Phase 6 complete (ranking deferred) — Phase 7 next`

Dzeroth is a production-grade, text-first social platform with familiar X/Twitter-like functionality and independently implemented UI/UX. It must preserve Dzeroth's anti-addiction, privacy, and quality constraints.

## Phase 0 — Audit and architecture

- [x] Run `00-project-auditor`
- [x] Produce a repository inventory
- [x] Identify implemented, partial, broken, and missing functionality
- [x] Run `01-architecture-planner`
- [x] Confirm package/module boundaries
- [x] Confirm API and database ownership
- [x] Establish ID, timestamp, pagination, error, and configuration conventions
- [x] Establish feed architecture
- [ ] Establish UI design system and authoritative reusable post component — deferred to Phase 1
- [x] Record important decisions in ADRs (0002–0006)

**Exit gate:** architecture is understood and contradictions are resolved before feature implementation begins. ✓

## Phase 1 — Foundation, identity, and profiles

- [x] Application bootstrap and configuration
- [x] Authentication/session lifecycle
- [x] User/profile model
- [x] Avatar/header/bio/profile editing
- [x] Privacy and account settings
- [x] Block/mute foundations
- [x] Core navigation and responsive shell

**Exit gate:** a user can securely authenticate and manage a profile end-to-end. ✓

## Phase 2 — Posts and conversations

- [x] Create/edit/delete posts where supported
- [x] Replies
- [x] Threads
- [x] Mentions
- [x] Hashtags/topics
- [ ] Media handling
- [x] Authoritative post card/detail UI
- [x] Server-side validation

**Exit gate:** post creation and conversation flows work across frontend, API, and database with tests. ✓ (Media handling deferred pending media provider decision — OPEN-1)

## Phase 3 — Feeds

- [x] Home timeline (follower-based, DB-first JOIN, cursor-paginated, finite at 200 items)
- [ ] Inner Circle feed — deferred; requires separate design
- [ ] Discovery feed — deferred; requires content ranking logic
- [x] Pagination/cursors (reuses ADR 0006 FeedCursor contract)
- [x] Finite content loops (all feeds terminate server-side)
- [x] Hard feed termination (HomeFeedTerminated / PostFeedTerminated are terminal states)
- [x] “Go Touch Grass” boundary UX (GoTouchGrassWidget)
- [x] No public validation metrics (enforced at DTO level, tested via reflection)
- [x] Follow/follower system (migrations 0007, instant follow, private-account enforcement)
- [x] Block/mute enforcement (atomic block+unfollow transaction, feed/profile filtering)
- [x] Bookmarks (owner-scoped, private DTO, paginated list, terminated at 200)
- [x] Repost idempotency (partial unique index migration 0009)
- [x] Quote/repost 5-second countdown (PostComposeBloc, cancellable, no bypass path)

**Exit gate:** feeds are bounded, performant, secure, and consistent with Dzeroth rules. ✓ (Inner Circle and Discovery feeds deferred — require separate architecture design)

## Phase 4 — Interactions and discovery

- [x] Reactions (private toggle, no public count)
- [x] Repost/retweet-style sharing (implemented in Phase 2/3)
- [x] Quote posts (implemented in Phase 2/3)
- [x] Bookmarks (core in Phase 3; PostDetailScreen wiring fixed in Phase 4 Wave 0)
- [x] Search (posts and users)
- [x] Topics/trends (hashtag feed — Phase 6)
- [x] Notifications (follow, mention, reply, reaction events)
- [ ] Inner Circle (deferred)
- [ ] Discovery/algorithmic feed (deferred)

**Exit gate:** interaction state is consistent, idempotent, authorized, and covered by tests. ✓ (Topics/trends, Inner Circle, and Discovery deferred)

## Phase 5 — Safety and creator functionality

- [x] Reporting (report submission: posts + users; migration 0013; rate limited; idempotent)
- [ ] Moderation actions — deferred to Phase 7 (requires full security review)
- [x] Abuse/rate limiting (fail-closed rate limiting on report submission)
- [x] Creator Studio (private analytics: reactions, bookmarks, replies, quotes per post; owner-scoped; terminated at 50 items)
- [x] Private analytics only (PostAnalytics never in public DTO; enforced by struct design)
- [x] Privacy controls (real SettingsScreen: private-account toggle, account suspension)
- [x] Secure creator data access (Studio endpoint JWT callerID-scoped, no path param)
- [x] Self-suspension (DELETE /me/account; revokes sessions; is_suspended=TRUE)

**Exit gate:** safety controls and creator analytics do not leak public validation metrics. ✓ (Moderator queue deferred to Phase 7)

## Phase 6 — Localized rankings and weekly titles

- [x] Topics/trends — hashtag browsing (`GET /api/v1/hashtags/{tag}/posts`, Flutter `features/hashtag/`, tappable PostCard hashtags)
- [ ] Redis ZSET ranking pipelines — DEFERRED to dedicated future feature
- [ ] Zone/type/week key convention — DEFERRED
- [ ] Weekly snapshot — DEFERRED
- [ ] PostgreSQL `weekly_titles` persistence — DEFERRED
- [ ] Sunday-night rotation behavior — DEFERRED
- [ ] Failure recovery and idempotency — DEFERRED

**Exit gate:** rankings are deterministic, recoverable, and operationally observable. ✓ (partial — topics/trends complete; ranking permanently deferred to dedicated feature)

## Phase 7 — Hardening

- [ ] Unit tests
- [ ] Integration tests
- [ ] API contract tests
- [ ] Database migration tests
- [ ] Security review
- [ ] Dependency/static analysis
- [ ] Performance/load testing
- [ ] Observability validation
- [ ] Backup/restore validation
- [ ] Production reviewer approval

**Exit gate:** no critical release blockers remain.

## Phase 8 — Release and deployment

- [ ] Environment-specific configuration
- [ ] CI/CD
- [ ] Build artifacts
- [ ] Database release safety
- [ ] Deployment procedure
- [ ] Rollback procedure
- [ ] Post-deploy smoke tests
- [ ] Operational runbook

**Exit gate:** a repeatable production release can be performed and rolled back safely.

### Phase 8B — CI Foundation

- [x] GitHub Actions workflow: go test, go vet, flutter analyze, flutter test, Docker build
- [x] Migration smoke test (Phase 8B-2)
- [x] Race-detector CI on Linux (Phase 8B-3)
- [ ] Deployment pipeline

### Phase 8C — Deployment Foundation

- [x] Production/staging Docker Compose topology (Phase 8C-1)
- [x] Private admin/DB/Redis ports, secrets provisioning, topology documentation (Phase 8C-1)
- [x] Staging runtime validation end-to-end (Phase 8C-2)
- [x] Operational runbook and pre-production security checklist (Phase 8C-3)

### Phase 8D — Live Integration Test Foundation

- [x] Live PostgreSQL integration tests: connectivity, migrations idempotency, user CRUD, session CRUD, constraint enforcement (Phase 8D-1)
- [x] Live Redis integration tests: connectivity, auth rejection, rate-limit key behavior (SetNX/Incr/Expire), key isolation, window expiry (Phase 8D-1)
- [x] `integration` job added to CI (bitnami/redis with requirepass, postgis:15-3.3) (Phase 8D-1)
- [x] API-level integration tests against real httptest.Server: register/login/GET-me flow, logout invalidates refresh, refresh valid/invalid, refresh rate-limit 429 + Retry-After, health/livez/readyz, request-ID, JWT failure WARN log, CORS allow-all and allowlist, error envelope format, PostgreSQL+HTTP chain, Redis rate-limit state created (Phase 8D-2)

### Phase 8D-4 — Production Hardening

- [x] PostgreSQL backup scripts: `scripts/backup/pg_backup.sh` (pg_dump + gzip, env-var credentials, timestamped output), `scripts/backup/pg_restore.sh` (destructive restore with confirmation prompt). Documentation: `docs/BACKUP_RECOVERY.md` (manual run, cron schedule, retention, step-by-step restore, monthly drill)
- [x] Nginx reference reverse proxy config: `nginx/nginx.prod.conf` — TLS termination, `proxy_set_header X-Real-IP $remote_addr` (no client header trust), HTTP→HTTPS redirect, proxy timeouts matching API handler timeout, HSTS at proxy layer. Trusted-proxy topology documented in `docs/DEPLOYMENT_TOPOLOGY.md`
- [x] Docker Secrets `_FILE` convention: config.go supports `JWT_SECRET_FILE`, `POSTGRES_PASSWORD_FILE`, `REDIS_PASSWORD_FILE` — reads secret from file when `_FILE` variant is set; plain env var used when `_FILE` is absent (backward compatible). `secrets/` directory gitignored. `secrets/README.md` documents secret file creation and format
- [x] Monitoring documentation: `docs/MONITORING.md` — all registered metrics listed, key alert recommendations (error rate, p99 latency, `dzeroth_redis_up`, DB pool saturation, auth failure rate), reference `monitoring/prometheus.yml` scrape config
- [x] Migration ownership policy: `docs/DATABASE_MIGRATION_POLICY.md` — current single-instance behavior, advisory lock semantics, horizontal scaling mitigation options (dedicated job, golang-migrate CLI, expand/contract)
- [x] Dockerfile healthcheck port: `ARG API_PORT=8080` + `ENV API_PORT` added; HEALTHCHECK uses `${API_PORT}` variable. Documents the runtime override limitation
- [x] Compose graceful shutdown: `stop_grace_period: 20s` added to API service in both `docker-compose.prod.yml` and `docker-compose.staging.yml` (buffer above Go API's 15s shutdown timeout)
- [x] Redis error counter assessment: `dzeroth_redis_errors_total` left dormant — wiring requires adding `*InfraMetrics` to `RateLimitMiddleware` signature (non-trivial signature change). Documented in `docs/MONITORING.md`
- [x] Invalid optional env var warnings: `optionalBoolWarn` and `optionalDurationWarn` emit WARN to stderr when a variable is set but unparseable. `REDIS_TLS` and `SESSION_CLEANUP_INTERVAL` use warn variants. Tests added in `config_test.go`
- [x] Security response headers middleware: `internal/platform/middleware/security_headers.go` — `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`. Wired into `cmd/api/main.go` after Recoverer. Unit tests in `security_headers_test.go`. Integration test assertion in `api_test.go`

## Rule

Do not skip phases merely because a feature appears small. Cross-layer work must be traced from UI to API to persistence and back.
