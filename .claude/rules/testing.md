# Dzeroth Testing Rules

## General

Compilation is not proof of correctness.

Tests must verify important behavior.

Do not weaken production code to satisfy a test.

## Flutter

Use:

- unit tests
- BLoC tests
- widget tests
- integration tests where appropriate

Validate:

- state transitions
- feed termination
- countdown behavior
- quote validation
- privacy rendering
- error states

## Go

Use:

- unit tests
- integration tests
- race detection where applicable

Validate:

- authorization
- validation
- idempotency
- concurrency
- database transactions
- API contracts
- Redis behavior

## Critical Dzeroth Invariants

Tests must cover:

- finite feeds
- no infinite scrolling
- five-second share/quote friction
- five distinct quote words
- public metric lockdown
- Private Creator Studio authorization
- localized ranking
- weekly snapshot/rotation
- authentication
- authorization
- rate limiting
- concurrent writes
- duplicate requests
- Redis failure/recovery

## Test Selection

Prefer:

```text
unit → integration → E2E
```

Use E2E for critical user journeys rather than every implementation detail.

## Completion

Report exactly:

- commands executed
- tests passed
- tests failed
- tests not run
- remaining risk
