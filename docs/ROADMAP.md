# Dzeroth Roadmap

**Status:** `PHASE 1 COMPLETE — Phase 2 complete — Phase 3 next`

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

- [ ] Home timeline
- [ ] Inner Circle feed
- [ ] Discovery feed
- [ ] Pagination/cursors
- [ ] Finite content loops
- [ ] Hard feed termination
- [ ] “Go Touch Grass” boundary UX
- [ ] No public validation metrics

**Exit gate:** feeds are bounded, performant, secure, and consistent with Dzeroth rules.

## Phase 4 — Interactions and discovery

- [ ] Reactions where product design permits
- [ ] Repost/retweet-style sharing
- [ ] Quote posts
- [ ] Bookmarks
- [ ] Search
- [ ] Topics/trends where approved
- [ ] Notifications

**Exit gate:** interaction state is consistent, idempotent, authorized, and covered by tests.

## Phase 5 — Safety and creator functionality

- [ ] Reporting
- [ ] Moderation actions
- [ ] Abuse/rate limiting
- [ ] Creator Studio
- [ ] Private analytics only
- [ ] Privacy controls
- [ ] Secure creator data access

**Exit gate:** safety controls and creator analytics do not leak public validation metrics.

## Phase 6 — Localized rankings and weekly titles

- [ ] Redis ZSET ranking pipelines
- [ ] Zone/type/week key convention
- [ ] Weekly snapshot
- [ ] PostgreSQL `weekly_titles` persistence
- [ ] Sunday-night rotation behavior
- [ ] Failure recovery and idempotency

**Exit gate:** rankings are deterministic, recoverable, and operationally observable.

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

## Rule

Do not skip phases merely because a feature appears small. Cross-layer work must be traced from UI to API to persistence and back.
