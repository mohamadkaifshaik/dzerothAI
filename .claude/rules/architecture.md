# Dzeroth Architecture Rules

## Stack

Frontend:
- Flutter / Dart
- flutter_bloc / bloc
- feature-based Clean Architecture

Backend:
- Go
- idiomatic Go
- `cmd/` + `internal/`
- chi where already established
- zap structured logging

Persistence:
- PostgreSQL 15+
- PostGIS
- Redis 7+

## Flutter Structure

```text
apps/mobile/lib/
├── core/
└── features/<feature>/
    ├── presentation/
    │   ├── bloc/
    │   └── screens/
    ├── domain/
    └── data/
```

Reuse existing feature boundaries.

Do not create parallel architectures.

## Backend Structure

```text
apps/backend/
├── cmd/
│   ├── api/
│   └── rank-cron/
├── internal/
└── db/
    └── migrations/
```

Responsibilities must remain explicit.

## Dependency Direction

Prefer:

```text
presentation → domain → data/infrastructure
```

Business rules must not leak into UI widgets.

Do not create abstractions without a real responsibility.

## Architecture Changes

Before changing architecture:

1. inspect current architecture
2. identify the problem
3. identify affected consumers
4. plan migration
5. preserve compatibility where required
6. test the change

Do not perform architecture rewrites as part of unrelated feature work.
