# Dzeroth Feature Registry

This is the single authoritative feature inventory.

## Status vocabulary

- `IMPLEMENTED` — verified in code/tests.
- `PARTIAL` — some layers exist but the feature is incomplete.
- `PLANNED` — approved direction, not implemented.
- `BLOCKED` — cannot proceed until a dependency/decision is resolved.
- `DEFERRED` — intentionally postponed.
- `REMOVED` — no longer part of the product.

Agents must search this registry before creating a feature, module, endpoint, model, screen, service, or reusable UI component.

## Registry

| ID | Area | Feature | Status | Source / Owner |
|---|---|---|---|---|
| F-001 | Identity | Authentication/session | IMPLEMENTED | Phase 1 — `internal/auth/`, JWT HS256, opaque refresh tokens, sessions table |
| F-002 | Profile | User profiles | IMPLEMENTED | Phase 1 — `internal/user/`, `GET /api/v1/users/:id`, `GET/PUT /api/v1/me` |
| F-003 | Profile | Avatar/header/bio | IMPLEMENTED | Phase 1 — profile edit endpoints; avatar/header upload blocked on OPEN-1 |
| F-004 | Social | Posts | IMPLEMENTED | Phase 2 — `internal/post/`, migrations 0004–0006, Flutter `features/post/` |
| F-005 | Social | Replies | IMPLEMENTED | Phase 2 — `post_type=reply`, thread endpoint, PostThreadView widget |
| F-006 | Social | Threads | IMPLEMENTED | Phase 2 — `thread_root_id` denormalization, `GET /posts/{postId}/thread`, finite depth |
| F-007 | Social | Mentions | IMPLEMENTED | Phase 2 — mention extraction into `post_mentions` join table (migration 0005) |
| F-008 | Social | Hashtags/topics | IMPLEMENTED | Phase 2 — normalized lowercase extraction into `post_hashtags` join table (migration 0006) |
| F-009 | Media | Post media | BLOCKED | Blocked on media storage provider decision (OPEN-1) |
| F-010 | Feed | Home timeline | IMPLEMENTED | Phase 3 — `internal/feed/`, `GET /api/v1/feeds/home`, DB-first JOIN, terminated at 200, Flutter `features/feed/` |
| F-011 | Feed | Inner Circle feed | DEFERRED | Requires separate architecture design; not in Phase 3 scope |
| F-012 | Feed | Discovery feed | DEFERRED | Requires content ranking logic; not in Phase 3 scope |
| F-013 | Feed | Finite content loops | IMPLEMENTED | Phase 3 — all feeds terminate server-side; HomeFeedTerminated / PostFeedTerminated are terminal BLoC states |
| F-014 | Feed | Hard termination / Go Touch Grass | IMPLEMENTED | Phase 3 — GoTouchGrassWidget rendered on all terminated feed states; no bypass path |
| F-015 | Privacy | Hide public validation metrics | IMPLEMENTED | Phase 2/3 — zero metric fields in PostDTO, FollowUserDTO, BookmarkDTO; enforced by reflection tests |
| F-016 | Social | Repost/retweet-style sharing | IMPLEMENTED | Phase 2 (`post_type=repost`) + Phase 3 (idempotency via migration 0009 partial unique index, 5-second countdown) |
| F-017 | Social | Quote posts | IMPLEMENTED | Phase 2 (`post_type=quote`, 5-distinct-word rule) + Phase 3 (5-second countdown enforced in PostComposeBloc) |
| F-018 | Social | Bookmarks | IMPLEMENTED | Phase 3 — `internal/bookmark/`, migration 0008, owner-scoped, Flutter `features/bookmark/`; PostDetailScreen bookmark wiring fixed in Phase 4 Wave 0 |
| F-019 | Discovery | Search | IMPLEMENTED | Phase 4 — `internal/search/`, migration 0012, Flutter `features/search/` |
| F-020 | Notifications | Notifications | IMPLEMENTED | Phase 4 — `internal/notification/`, migration 0011, Flutter `features/notification/` |
| F-021 | Safety | Reporting | IMPLEMENTED | Phase 5 — `internal/report/`, migration 0013, Flutter `features/report/`; `POST /api/v1/posts/{postId}/report` + `POST /api/v1/users/{userId}/report`; moderator queue deferred to Phase 7 |
| F-022 | Safety | Block/mute | IMPLEMENTED | Phase 1 (schema: blocks/mutes tables, migration 0003) + Phase 3 (full enforcement: `internal/block/`, atomic block+unfollow transaction, feed/profile filtering, Flutter `features/block/`) |
| F-023 | Creator | Private Creator Studio | IMPLEMENTED | Phase 5 — `internal/studio/`, Flutter `features/studio/`; `GET /api/v1/me/studio/analytics`; current-state aggregates only; owner-scoped; terminated at 50 items |
| F-024 | Ranking | Localized weekly titles | DEFERRED | Phase 6 — ranking deferred to dedicated future feature |
| F-033 | Discovery | Topics/Trends (hashtag feed) | IMPLEMENTED | Phase 6 — `GET /api/v1/hashtags/{tag}/posts` (`internal/post/` extension); Flutter `features/hashtag/`; tappable `#hashtag` tokens in PostCard; terminated at 200 items; block-filtered for authenticated callers |
| F-025 | Operations | Observability | PLANNED | Phase 7 |
| F-026 | Operations | CI/CD and deployment | PLANNED | Phase 8 |
| F-027 | Social | Follow/follower system | IMPLEMENTED | Phase 3 — `internal/follow/`, migration 0007, instant follow, `IsBlockedBy` check, private-account enforcement, Flutter `features/follow/` |
| F-028 | Social | Reactions/likes | IMPLEMENTED | Phase 4 — `internal/reaction/`, migration 0010, Flutter `features/reaction/` |
| F-029 | Notifications | Notifications | IMPLEMENTED | Phase 4 — `internal/notification/`, migration 0011, Flutter `features/notification/` |
| F-030 | Discovery | Search (posts + users) | IMPLEMENTED | Phase 4 — `internal/search/`, migration 0012, Flutter `features/search/` |
| F-031 | Settings | Account settings and privacy | IMPLEMENTED | Phase 5 — `GET/PUT /api/v1/me/settings` (Phase 1 backend), Flutter `features/settings/`; real SettingsScreen replaces placeholder |
| F-032 | Safety | Self-suspension | IMPLEMENTED | Phase 5 — `DELETE /api/v1/me/account` (`internal/user/`), `auth.RevokeAllSessions`, Flutter SettingsScreen confirmation dialog; `is_suspended=TRUE` soft-suspend |

## Update protocol

Before adding a feature:

1. Search this registry.
2. Search the repository for existing implementations.
3. Search API contracts and migrations.
4. Confirm the feature is not already implemented under another name.
5. Update the existing registry row rather than adding a duplicate.
6. Add an ADR if the feature introduces a significant architectural decision.

When implementation status changes, update this file as part of the same change.
