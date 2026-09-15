# Dzeroth API Contracts

This document is the API contract registry and convention guide.

## Contract status

- `IMPLEMENTED` — verified against the current backend.
- `PROPOSED` — design only; do not call from frontend code until implemented.
- `DEPRECATED` — still present but scheduled for removal.
- `REMOVED` — no longer valid.

## Core conventions

- API versioning follows the existing backend convention; do not invent a second versioning scheme.
- Inspect existing router registration before adding an endpoint.
- Search handlers, services, repositories, models, and tests before creating new ones.
- Authentication and authorization are enforced server-side.
- Request validation is enforced server-side even if Flutter validates first.
- Errors must use the established project error shape.
- Pagination must use the project's chosen cursor/page convention consistently.
- IDs and timestamps must use the existing project convention once established.
- Mutating operations should be idempotent where retries can occur.
- Do not expose private analytics or internal validation metrics through public endpoints.

## Contract registry

| Area | Endpoint | Status | Notes |
|---|---|---|---|
| Identity | `/api/v1/me` | IMPLEMENTED/VERIFY | Confirm against current router and handler code |
| Feed | `/api/v1/feeds/inner-circle` | IMPLEMENTED/VERIFY | Confirm response contract and pagination |
| Feed | `/api/v1/feeds/discovery` | IMPLEMENTED/VERIFY | Confirm response contract and pagination |
| Posts | Existing posts endpoint(s) | IMPLEMENTED/VERIFY | Search router before modifying/adding |
| Auth | Authentication endpoints | PROPOSED | Define only after audit |
| Profiles | Profile endpoints | PROPOSED | Define only after audit |
| Search | Search endpoints | PROPOSED | Define only after architecture review |
| Notifications | Notification endpoints | PROPOSED | Define only after architecture review |

## Contract change protocol

For any endpoint change:

1. Find the current route registration.
2. Find its handler/service/repository path.
3. Find all frontend consumers.
4. Find tests.
5. Determine backward-compatibility impact.
6. Update this registry.
7. Update tests.
8. Review security and authorization.
9. Review the diff for duplicate or orphaned endpoints.

Never create a second endpoint because an existing endpoint was hard to find.
