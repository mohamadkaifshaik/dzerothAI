# Dzeroth Backend Rules

## Go

Write idiomatic Go.

Prefer:

- explicit error handling
- context propagation
- bounded resources
- small functions
- clear ownership
- dependency injection where useful
- structured zap logging

Avoid:

- swallowed errors
- giant functions
- global mutable state
- unnecessary interfaces
- panic-based normal error handling

## HTTP/API

Before creating or changing an endpoint:

1. search routes
2. search handlers
3. search request/response types
4. search consumers
5. search tests

Do not create duplicate routes.

## Security

Authentication and authorization are server-side responsibilities.

Never trust:

- client-supplied identity
- hidden UI controls
- client-only roles
- unverified claims

## Validation

Validate:

- type
- size
- format
- allowed values
- ownership
- authorization
- product invariants

## Product Rules

Backend must enforce security-sensitive versions of:

- finite feed boundaries
- five-second share/quote rule
- five-distinct-word quote rule
- public metric lockdown
- creator analytics privacy

## Errors

Never expose internal stack traces, database errors, secrets, or infrastructure
details.

## Reliability

Use appropriate:

- timeouts
- cancellation
- retries
- idempotency
- transaction boundaries
- concurrency controls

## Logging

Use existing zap conventions.

Never log:

- passwords
- tokens
- secrets
- unnecessary private content
