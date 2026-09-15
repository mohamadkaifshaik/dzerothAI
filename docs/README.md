# Dzeroth Engineering Documentation

This directory is the durable project knowledge base for Dzeroth.

## Source-of-truth order

1. Existing source code and tests
2. Database migrations and API contracts
3. `CLAUDE.md`
4. `.claude/rules/`
5. `.claude/agents/`
6. This documentation
7. New proposals and assumptions

If documentation conflicts with implemented behavior, inspect the implementation and tests before changing either.

## Core documents

- `ROADMAP.md` — phased product and engineering plan.
- `FEATURE_REGISTRY.md` — one authoritative inventory of features and their implementation status.
- `API_CONTRACTS.md` — API contract conventions and registry.
- `DO_NOT_BUILD.md` — explicit boundaries and anti-scope-creep rules.
- `adr/` — architecture decision records.

## Documentation rules

- Do not create duplicate documentation for the same subject.
- Update the authoritative document when a decision changes.
- Never document an API, schema, environment variable, service, or feature that does not actually exist unless it is clearly marked as proposed.
- Mark proposals as `PROPOSED` and implemented behavior as `IMPLEMENTED`.
- Keep documentation concise enough to remain useful to engineers and Claude Code agents.
