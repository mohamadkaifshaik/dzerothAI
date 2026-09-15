# Dzeroth Production Rules

## Quality Gates

Before production release, verify applicable:

- formatting
- static analysis
- unit tests
- integration tests
- security checks
- dependency checks
- builds
- migrations
- configuration
- artifact integrity

## Observability

Production must provide useful visibility into:

- latency
- errors
- dependency failures
- database health
- Redis health
- authentication failures
- authorization failures
- rate limits
- background jobs
- ranking snapshots
- startup/shutdown

Reuse the existing telemetry stack.

Do not create duplicate logging/metrics systems.

## Configuration

Do not hard-code environment-specific values or secrets.

Before adding an environment variable:

1. search for an equivalent
2. follow naming conventions
3. document purpose
4. validate required values

## CI/CD

Reuse existing workflows.

Do not create duplicate pipelines.

CI should fail clearly when required checks fail.

## Artifacts

Production artifacts must be:

- reproducible
- versioned
- identifiable
- traceable to a commit/build

## Deployment Preconditions

Do not deploy unless:

- tests pass
- static checks pass
- security review is acceptable
- migration plan is known
- required configuration exists
- secrets are correctly managed
- health/readiness checks exist
- observability exists
- artifact version is known
- rollback/roll-forward strategy exists

## Deployment

Use the safest available rollout strategy.

Verify:

- health
- readiness
- critical API paths
- authentication
- authorization
- database connectivity
- Redis connectivity
- background jobs
- frontend integrity

Never invent provider-specific deployment resources or commands.

## Rollback

When a material regression occurs:

1. stop rollout
2. preserve evidence
3. determine rollback vs roll-forward
4. execute approved procedure
5. verify recovery
6. document incident
7. add regression coverage where appropriate

## Database Releases

Consider:

- backward compatibility
- deployment ordering
- migration locks
- existing data
- expand/contract strategies
- rollback/roll-forward

Never assume a technically reversible migration is operationally safe.
