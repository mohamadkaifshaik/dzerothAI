# ADR 0002 — Flutter at Repository Root

**Status:** ACCEPTED

## Context

The architecture rules in `.claude/rules/architecture.md` document a target layout of
`apps/mobile/lib/` for the Flutter application. However, the repository was initialized
as a standard Flutter project with the package root at the repository root (`lib/`,
`pubspec.yaml`, and all platform shells directly under `/`).

All platform shells contain hardcoded relative paths that resolve to the repository root:

- `android/app/build.gradle.kts` — `flutter { source = "../.." }` resolves to the repo root
- iOS, macOS, Windows, and Linux shells follow the same Flutter toolchain convention

Moving Flutter to `apps/mobile/` would require updating:
- `source` paths in `android/app/build.gradle.kts`
- Xcode project settings and Podfiles for iOS and macOS
- CMakeLists.txt files for Linux and Windows
- All plugin symlinks and generated tool configuration

These changes are non-trivial, coordinated across six platform configurations, and carry a
risk of silent build breakage. No backend code exists yet that would create a conflict with
the current Flutter root layout.

## Decision

Flutter remains at the repository root. The `pubspec.yaml`, `lib/`, `test/`, and all
platform shell directories (`android/`, `ios/`, `macos/`, `linux/`, `windows/`, `web/`)
stay where they are.

The Go backend is introduced under `apps/backend/` alongside the existing root layout.
This provides a natural expansion point: if Flutter is migrated in the future, it would
move to `apps/mobile/` to sit alongside `apps/backend/`.

## Alternatives considered

**Move Flutter to `apps/mobile/` immediately.**
Rejected. The platform shell path updates are error-prone with no tooling to automate them
safely. The benefit (cleaner monorepo layout) is organizational only and provides no
immediate technical value while no backend exists yet.

## Consequences

- The `apps/` directory is introduced solely for `apps/backend/`.
- Platform shells must not be moved as part of any subsequent feature work.
- If Flutter is migrated to `apps/mobile/` in the future, a new ADR must record the
  migration plan, the platform-shell update steps, and the validation procedure.
- The architecture rules document (`architecture.md`) retains `apps/mobile/lib/` as the
  aspirational target; this ADR records why migration is deferred.

## Validation / follow-up

- Confirm `flutter analyze` and `flutter build` pass after `apps/backend/` is introduced.
- Revisit this decision when the project reaches Phase 8 (deployment), where the monorepo
  layout has production implications.
