# Dzeroth — Do Not Build

This document prevents scope creep and accidental violation of product invariants.

## Never copy proprietary X implementation

Dzeroth may recreate familiar X/Twitter-like functionality and interaction patterns independently, but must not copy:

- X proprietary source code
- proprietary internal APIs
- private endpoints
- proprietary assets
- private algorithms or implementation details
- credentials, secrets, or internal infrastructure
- copyrighted assets without permission

The goal is an independent implementation with Dzeroth-specific product rules.

## Never reintroduce infinite engagement loops

Do not build:

- endless feed pagination
- infinite auto-loading content
- feed mechanics designed to keep a user scrolling indefinitely
- hidden continuation after a hard termination boundary

Feeds must remain finite and visibly terminate.

## Never expose public validation metrics

Do not expose public:

- like counts
- impression/view counts
- bookmark counts
- follower counts
- other engagement metrics whose primary purpose is public social validation

If analytics are approved, keep them inside secure Private Creator Studio surfaces.

## Never bypass backend invariants

Do not rely on Flutter-only enforcement for:

- share countdown
- quote minimum-word rule
- authorization
- privacy controls
- rate limits
- moderation restrictions
- feed termination policy

## Never create duplicates

Do not add a second:

- feature folder
- model
- repository
- service
- endpoint
- database table
- migration for the same change
- reusable post component
- state-management implementation
- configuration key

Search first and extend the existing source of truth.

## Never invent infrastructure

Do not assume:

- a database exists
- Redis exists
- an environment variable exists
- a deployment target exists
- an external service is configured
- an API endpoint exists

Inspect the repository and configuration first.

## Never silently change architecture

Significant architecture changes require:

1. evidence from the current codebase,
2. a plan,
3. an ADR,
4. validation,
5. review.

## Never mark work complete without evidence

A feature is not complete because files compile or a screen renders.

Completion requires appropriate tests, security review, integration validation, and diff review for the affected scope.
