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
| F-001 | Identity | Authentication/session | PLANNED | Phase 1 |
| F-002 | Profile | User profiles | PLANNED | Phase 1 |
| F-003 | Profile | Avatar/header/bio | PLANNED | Phase 1 |
| F-004 | Social | Posts | PLANNED | Phase 2 |
| F-005 | Social | Replies | PLANNED | Phase 2 |
| F-006 | Social | Threads | PLANNED | Phase 2 |
| F-007 | Social | Mentions | PLANNED | Phase 2 |
| F-008 | Social | Hashtags/topics | PLANNED | Phase 2 |
| F-009 | Media | Post media | PLANNED | Phase 2 |
| F-010 | Feed | Home timeline | PLANNED | Phase 3 |
| F-011 | Feed | Inner Circle feed | PLANNED | Phase 3 |
| F-012 | Feed | Discovery feed | PLANNED | Phase 3 |
| F-013 | Feed | Finite content loops | PLANNED | Dzeroth invariant |
| F-014 | Feed | Hard termination / Go Touch Grass | PLANNED | Dzeroth invariant |
| F-015 | Privacy | Hide public validation metrics | PLANNED | Dzeroth invariant |
| F-016 | Social | Repost/retweet-style sharing | PLANNED | Phase 4 |
| F-017 | Social | Quote posts | PLANNED | Phase 4 |
| F-018 | Social | Bookmarks | PLANNED | Phase 4 |
| F-019 | Discovery | Search | PLANNED | Phase 4 |
| F-020 | Notifications | Notifications | PLANNED | Phase 4 |
| F-021 | Safety | Reporting | PLANNED | Phase 5 |
| F-022 | Safety | Block/mute | PLANNED | Phase 5 |
| F-023 | Creator | Private Creator Studio | PLANNED | Phase 5 |
| F-024 | Ranking | Localized weekly titles | PLANNED | Phase 6 |
| F-025 | Operations | Observability | PLANNED | Phase 7 |
| F-026 | Operations | CI/CD and deployment | PLANNED | Phase 8 |

## Update protocol

Before adding a feature:

1. Search this registry.
2. Search the repository for existing implementations.
3. Search API contracts and migrations.
4. Confirm the feature is not already implemented under another name.
5. Update the existing registry row rather than adding a duplicate.
6. Add an ADR if the feature introduces a significant architectural decision.

When implementation status changes, update this file as part of the same change.
