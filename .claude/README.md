# Claude Code Instructions

Dzeroth uses a layered instruction model.

- `../CLAUDE.md` — global product constitution and non-negotiable rules.
- `agents/` — specialized engineering agents.
- `rules/` — detailed technology/discipline-specific rules.

Agents should read the global constitution first and then the rule files relevant
to their responsibility.

The repository is always the source of truth. Documentation never licenses an
agent to invent missing code, APIs, schema, infrastructure, or dependencies.

## Recommended execution order

For a new implementation cycle, use this order unless repository evidence requires another sequence:

1. `00-project-auditor`
2. `01-architecture-planner`
3. foundation / frontend / backend / database agents for the approved slice
4. `06-api-security-agent` for security-sensitive work
5. `07-test-quality-agent`
6. `08-performance-reliability-agent`
7. `09-observability-agent`
8. `10-release-ci-agent`
9. `11-production-deployment-agent`
10. `12-production-reviewer`

The roadmap, feature registry, API contract registry, ADRs, and do-not-build rules are durable coordination artifacts. Agents should update them when their work changes the corresponding truth.

## Mandatory pre-edit check

Before editing:

- inspect the current tree;
- search the feature registry;
- search existing code and routes;
- inspect relevant migrations;
- inspect relevant tests;
- read only the specialized rule files needed for the task;
- state the intended files and why they are authoritative.

If ambiguity remains, stop and ask rather than inventing architecture.
