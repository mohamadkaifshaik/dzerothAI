# Dzeroth — Global Engineering Constitution

> This file contains only the rules that every agent must know.
> Detailed technology-specific rules live in `.claude/rules/`.
>
> **Dzeroth is a social-network application inspired by and functionally
> comparable to X (formerly Twitter).** The goal is to provide familiar,
> production-grade social-network functionality and a polished, familiar
> social-media UX while applying Dzeroth's own product rules and identity.
>
> Do not copy X's proprietary source code, assets, trademarks, private APIs,
> or proprietary implementation. Recreate required behavior independently
> using Dzeroth's architecture and design system.

---

# 1. Product Identity

Dzeroth is a text-first social network with the familiar interaction model
users expect from modern microblogging/social platforms.

Where applicable, Dzeroth should provide the core functionality users expect
from X/Twitter-like products, including:

- account creation and authentication
- profiles
- avatars and profile headers
- bios
- posts
- replies
- conversations/threads
- repost/retweet-style sharing
- quote posts
- likes/reactions where appropriate internally
- bookmarks
- following/followers where appropriate internally
- home timeline
- discovery/explore
- search
- notifications
- mentions
- hashtags/topics where supported
- media attachments where supported
- profile timelines
- settings
- moderation/reporting
- blocking/muting where supported
- creator functionality
- responsive, polished social-network UI/UX

These capabilities must not violate Dzeroth's product constraints below.

---

# 2. Dzeroth Product Constitution

Dzeroth intentionally differs from conventional social networks in important
ways.

## 2.1 No Infinite Scrolling

There must be **NO infinite scrolling**.

Feeds are finite.

When the current feed reaches its configured boundary:

- stop fetching additional content
- expose an explicit termination/boundary state
- present the "Go Touch Grass" experience where applicable

Do not implement hidden endless pagination.

The backend must not provide an effectively infinite feed that the client merely
chooses not to consume.

## 2.2 Share/Quote Friction

All share and quote actions require a mandatory **five-second countdown**.

The client must display the countdown.

The backend must enforce the underlying rule so bypassing the client cannot
remove the intended friction.

Text quotes require at least **5 distinct words**.

The normalization/counting strategy must be consistent between client and
backend.

## 2.3 Public Metric Lockdown

Public feed layers must expose **zero social-validation metrics**.

Do not expose publicly:

- likes
- impressions
- bookmark counts
- follower counts
- equivalent popularity metrics

Internal persistence may contain required data, but public DTOs must not
serialize these metrics.

Analytics belong to the secure **Private Creator Studio**.

## 2.4 Localized Weekly Titles

Localized ranking uses Redis `ZSET`s.

Weekly keys:

```text
leaderboard:zone:type:YYYY-wWW
```

Weekly snapshot/rotation occurs Sunday night through the ranking cron process.

Snapshots are persisted to PostgreSQL `weekly_titles`.

The implementation must be:

- idempotent
- retry-safe
- timezone-aware
- observable
- resilient to Redis/PostgreSQL partial failure

---

# 3. Absolute Engineering Rules

These rules apply to every agent.

### 1. Do not guess.

### 2. Inspect before editing.

### 3. Search before creating.

### 4. Reuse before duplicating.

### 5. One responsibility has one authoritative implementation.

### 6. Backend enforces security and security-sensitive product invariants.

### 7. Tests are evidence.

### 8. Do not claim completion without validation.

### 9. Do not make unrelated refactors.

### 10. When uncertain, investigate or stop instead of hallucinating.

---

# 4. Source-of-Truth Hierarchy

When information conflicts, use this order:

1. Existing working source code
2. Existing database schema/migrations
3. Existing automated tests
4. Existing verified API contracts/consumers
5. Existing configuration/deployment files
6. This `CLAUDE.md`
7. `.claude/rules/*.md`
8. Other documentation
9. Agent assumptions

Agent assumptions are never project truth.

If something cannot be verified, mark it `UNKNOWN` rather than inventing it.

---

# 5. Anti-Hallucination Protocol

Never invent:

- files
- folders
- endpoints
- request fields
- response fields
- database tables
- database columns
- indexes
- constraints
- Redis keys
- environment variables
- authentication claims
- roles
- external APIs
- cloud resources
- deployment resources
- dependencies
- credentials

Before relying on any of these, inspect the repository.

Important implementation decisions must be traceable to:

- source code
- schema/migrations
- tests
- configuration
- this constitution
- explicit user requirements

---

# 6. Anti-Duplication Protocol

Before creating any new:

- file
- folder
- class
- function
- service
- repository
- handler
- route
- model
- entity
- DTO
- validator
- BLoC
- middleware
- migration
- database object
- Redis key scheme
- configuration system
- CI/CD workflow

the agent must:

1. Search the repository.
2. Search equivalent names/responsibilities.
3. Inspect references/imports/usages.
4. Identify the authoritative implementation.
5. Extend it when possible.

Never create artificial variants such as:

```text
*_new
*_old
*_copy
*_backup
*_final
*_updated
*_v2
feature2
service2
repository2
```

unless explicitly required.

---

# 7. Minimal Change Principle

Do not refactor unrelated code during feature work.

Do not unnecessarily:

- rename files
- reorganize directories
- replace libraries
- rewrite services
- upgrade dependencies
- introduce abstractions
- change architecture

Prefer the smallest safe, reviewable, reversible change.

---

# 8. Development Lifecycle

For substantial work:

```text
UNDERSTAND
    ↓
INSPECT
    ↓
SEARCH
    ↓
PLAN
    ↓
IMPLEMENT
    ↓
FORMAT
    ↓
TEST
    ↓
SECURITY REVIEW
    ↓
DIFF REVIEW
    ↓
DONE
```

Never skip inspection simply because the requested feature sounds familiar.

---

# 9. Agent Coordination

Agents are specialized workers, not independent project owners.

Every agent must:

1. Read this file.
2. Read applicable `.claude/rules/*.md`.
3. Inspect the current repository.
4. Search for existing implementations.
5. Review relevant previous agent work when available.
6. Define its change scope.
7. Modify only necessary areas.
8. Preserve valid work from other agents.
9. Validate changes.
10. Report actual evidence.

The repository is the source of truth, not an agent's previous assumption.

---

# 10. X/Twitter-Like UX Direction

Dzeroth should feel familiar to users of X/Twitter-like applications.

Use the familiar mental model for:

- navigation
- timelines
- profile pages
- post composition
- reply threads
- quote posts
- repost/share interactions
- notifications
- search/discovery
- settings
- responsive layouts
- mobile interactions

The implementation should be polished and production-grade rather than a
minimal prototype.

However, Dzeroth's product rules always take precedence.

For example:

- familiar timeline UX is allowed, but **not infinite scrolling**
- familiar engagement actions are allowed, but **share/quote requires 5 seconds**
- internal engagement data may exist, but **public metrics remain locked down**
- creator analytics may exist, but they remain **private**

Do not blindly reproduce every conventional social-media mechanic if it
conflicts with Dzeroth's product constitution.

---

# 11. UI/UX Quality Standard

The UI must be:

- responsive
- accessible
- consistent
- visually polished
- fast
- predictable
- touch-friendly
- keyboard-friendly where applicable
- resilient to loading/error/empty states

Do not accept placeholder-quality UI as production UI.

Every major screen should account for:

- loading
- success
- empty state
- error state
- offline/network failure where applicable
- permission/authorization state
- finite-feed termination where applicable

Detailed Flutter rules are in:

```text
.claude/rules/frontend.md
```

---

# 12. Security and Privacy

Security and privacy are release requirements.

At minimum protect against:

- broken access control
- injection
- authentication bypass
- authorization bypass
- replay
- abuse
- rate-limit bypass
- privilege escalation
- information leakage
- unsafe input
- cache poisoning
- concurrency/race issues

Never expose:

- passwords
- tokens
- secrets
- private analytics
- unnecessary private user data

Never rely solely on client-side security.

Detailed rules:

```text
.claude/rules/security.md
```

---

# 13. Testing and Quality

Code is not complete merely because it compiles.

Important product behavior must be tested.

Critical Dzeroth behavior includes:

- finite feed termination
- no infinite scrolling
- five-second share/quote friction
- five-distinct-word quote rule
- public metric lockdown
- Private Creator Studio authorization
- localized ranking
- weekly snapshot/rotation
- authentication
- authorization
- rate limiting
- database constraints
- Redis failure handling
- concurrent operations
- idempotency

Detailed rules:

```text
.claude/rules/testing.md
```

---

# 14. Production Standard

Dzeroth must be engineered for real production use.

Production concerns include:

- correctness
- security
- privacy
- reliability
- performance
- observability
- graceful failure
- migrations
- reproducible builds
- CI/CD
- deployment safety
- rollback/roll-forward

Detailed rules:

```text
.claude/rules/production.md
```

---

# 15. Technology Rules

The project architecture is:

```text
Flutter
   ↓
Go API
   ↓
PostgreSQL/PostGIS + Redis
```

Technology-specific rules are intentionally separated:

```text
.claude/rules/architecture.md
.claude/rules/frontend.md
.claude/rules/backend.md
.claude/rules/database.md
.claude/rules/security.md
.claude/rules/testing.md
.claude/rules/production.md
```

Do not introduce another framework, database, cache, queue, or architecture
without explicit justification.

---

# 16. Definition of Done

A task is DONE only when applicable conditions are satisfied:

```text
[ ] Requirement understood
[ ] Repository inspected
[ ] Existing implementation searched
[ ] No duplicate implementation introduced
[ ] No unnecessary folder created
[ ] No unnecessary dependency introduced
[ ] Architecture preserved
[ ] Implementation complete
[ ] No production placeholder remains
[ ] Product invariants enforced
[ ] Security requirements satisfied
[ ] Error handling implemented
[ ] Tests added/updated where required
[ ] Relevant tests pass
[ ] Formatter passes
[ ] Static analysis passes
[ ] Database changes validated
[ ] Redis changes validated
[ ] Observability considered
[ ] Final diff reviewed
[ ] No unrelated changes introduced
[ ] Deployment implications considered
```

If an applicable item is false, do not claim the task is complete.

---

# 17. Final Agent Report

Every implementation agent must report:

## Changed

Actual files changed and why.

## Reused

Existing components reused instead of duplicated.

## Validated

Commands/tests/builds actually executed and their results.

## Risks

Known risks and unverified areas.

## Remaining

Genuinely unfinished work.

Never say:

- "everything is perfect"
- "production-ready" without evidence
- "tests should pass"
- "this should work"

without actual verification.

---

# 18. Absolute Rules

1. **Do not guess.**
2. **Inspect before editing.**
3. **Search before creating.**
4. **Reuse before duplicating.**
5. **Preserve Dzeroth's product constitution.**
6. **Backend enforcement is mandatory for security-sensitive rules.**
7. **Do not expose public social-validation metrics.**
8. **Do not introduce infinite scrolling.**
9. **Do not bypass the five-second share/quote friction.**
10. **Do not weaken the five-distinct-word quote requirement.**
11. **Do not make unrelated changes.**
12. **Do not claim success without evidence.**

Dzeroth must remain one coherent production system, not a collection of
independently generated agent projects.

## Durable Project Knowledge

The following files are authoritative coordination artifacts:

- `docs/ROADMAP.md`
- `docs/FEATURE_REGISTRY.md`
- `docs/API_CONTRACTS.md`
- `docs/DO_NOT_BUILD.md`
- `docs/ENVIRONMENT_STRATEGY.md`
- `docs/DATABASE_SEED_STRATEGY.md`
- `docs/IDENTIFIERS_AND_TIME.md`
- `docs/RELEASE_CHECKLIST.md`
- `docs/adr/`

Before starting substantial feature work, inspect the relevant documents. Update them when implementation changes the corresponding source of truth.
