# ADR 0003 — Backend Placed at apps/backend/

**Status:** ACCEPTED

## Context

The repository root is occupied by the Flutter application (per ADR 0002). The Go backend
needs a location that does not conflict with Flutter's `lib/`, `test/`, and platform shell
directories, and that signals the monorepo structure described in the architecture rules.

## Decision

The Go backend lives at `apps/backend/`.

Structure:

```
apps/backend/
├── go.mod                         module: github.com/mohamadkaifshaik/dzerothAI/apps/backend
├── go.sum
├── Makefile
├── cmd/
│   ├── api/
│   │   └── main.go                Phase 1: single HTTP binary
│   └── rank-cron/
│       └── main.go                Phase 6: ranking cron (stub in Phase 1)
├── internal/
│   ├── config/
│   ├── apierror/
│   ├── platform/
│   │   ├── db/
│   │   └── redis/
│   ├── auth/
│   └── user/
└── db/
    └── migrations/
```

The Go module path is: `github.com/mohamadkaifshaik/dzerothAI/apps/backend`

This is derived from the confirmed GitHub repository:
`https://github.com/mohamadkaifshaik/dzerothAI.git`

## Alternatives considered

**`backend/` at the repository root.**
Also acceptable, but `apps/backend/` better expresses the future monorepo intent (alongside
a potential `apps/mobile/` when Flutter migrates) and keeps the root less cluttered.

## Consequences

- All Go import paths are prefixed with `github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/...`
- The `go.mod` file is the authoritative Go module declaration; the module path must not
  be changed after packages are imported across the project without a coordinated rename.
- `docker-compose.yml` at the repo root references `./apps/backend` for the API service build context.

## Validation / follow-up

- Verify `go build ./...` passes from `apps/backend/` after initialization.
- Confirm the module path matches the GitHub repository URL before creating `go.mod`.
