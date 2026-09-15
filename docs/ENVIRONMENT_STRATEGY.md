# Environment Strategy

**Status:** `BASELINE — VERIFY DURING AUDIT`

Do not invent environment variables. First inspect the repository, deployment configuration, Docker files, CI files, and application configuration.

## Required environment separation

At minimum, distinguish:

- local development
- test/CI
- staging
- production

## Rules

- Secrets must never be committed.
- Production secrets must come from an approved secret-management mechanism.
- Local `.env` files must be ignored by Git if used.
- Configuration names must have one authoritative definition.
- Avoid environment-specific code paths when configuration can solve the difference safely.
- Document required variables only after verifying them in code.
- Never paste real credentials into documentation or agent prompts.

## Audit checklist

- [ ] Find all environment variable reads.
- [ ] Find all `.env`/configuration files.
- [ ] Find Docker/Compose configuration.
- [ ] Find CI/CD configuration.
- [ ] Identify secrets and credential references.
- [ ] Identify missing validation for required configuration.
- [ ] Establish staging/production configuration separately.
