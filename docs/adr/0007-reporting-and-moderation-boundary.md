# ADR 0007 — Reporting and Moderation Boundary

**Status:** ACCEPTED

## Context

Phase 5 introduces user-facing safety controls. Two design questions required resolution:

1. **What Phase 5 actually ships**: Full moderation (queue, moderator role, moderation actions)
   was initially considered alongside report submission.
2. **Whether blocks prevent reporting**: A blocked user attempting to report their blocker
   is a common adversarial scenario.

## Decision

### Phase 5 scope for reporting

Phase 5 ships **report submission only** — two endpoints:

- `POST /api/v1/posts/{postId}/report`
- `POST /api/v1/users/{userId}/report`

These insert rows into the `reports` table with `status = 'pending'`.

**Moderator role, moderation queue, and moderation actions are deferred to Phase 7.**
No `role` column exists in `users`. No moderation endpoints are registered. No admin UI
is built.

This avoids premature access-control complexity before Phase 7 establishes the full
security and testing framework.

### Block relationships do not prevent reporting

A user can report another user regardless of whether a block relationship exists in
either direction.

**Rationale:** Blocking is a privacy and feed-filtering mechanism. Reporting is a
safety escalation to moderators. Preventing reports across block boundaries would
allow a bad actor to immunize themselves from reports by pre-emptively blocking
potential reporters.

The server never reveals the block relationship to the reporting party.

### Self-report prevention

The backend enforces that a user cannot report their own post or their own account.
This returns `CodeValidation` (400) rather than a silent no-op to surface the client
error clearly.

### Idempotency

A duplicate pending report from the same reporter against the same target is silently
accepted (204) via `ON CONFLICT DO NOTHING`. This prevents exposing whether a prior
report exists.

## Consequences

- Moderator tooling ships in Phase 7 alongside the full security review.
- The `reports` table `reviewed_by` and `reviewed_at` columns exist but remain NULL
  until Phase 7 implements the moderation workflow.
- The `status` column defaults to `'pending'`; only the Phase 7 moderation agent
  will transition it to `'reviewed'` or `'dismissed'`.
