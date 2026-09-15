# Dzeroth Security and Privacy Rules

## Threat Model

Consider at minimum:

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
- race conditions
- resource exhaustion

## Authentication

Authentication must be verified by the backend.

Never trust a client-provided user identity.

## Authorization

Check authorization at the resource/action boundary.

UI hiding is not authorization.

## Input

Validate all untrusted input.

Enforce:

- length limits
- payload limits
- valid formats
- allowed values
- ownership
- authorization
- business rules

## Secrets

Never commit or log:

- passwords
- API keys
- access tokens
- private keys
- production credentials

Never place production secrets in Flutter bundles.

## Privacy

Use explicit public DTOs.

Never serialize database models directly into public responses when doing so
could expose private fields.

Creator analytics are private.

Public feeds must not expose social-validation metrics.

## Rate Limiting

Use backend rate limiting for abuse-sensitive operations.

Do not rely only on Flutter.

## Errors

Never expose:

- stack traces
- SQL errors
- internal service names
- credentials
- sensitive identifiers

## Security Changes

Security-sensitive changes require tests and final review.
