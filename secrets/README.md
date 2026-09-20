# secrets/ — Docker Secrets files

This directory holds secret files for Docker Secrets support.

**These files must NEVER be committed to git.** The `secrets/` directory is
gitignored. The `.gitignore` entry must never be removed.

---

## How to create secret files before first deploy

```bash
# Create the secrets directory on the deployment host
mkdir -p secrets
chmod 700 secrets

# Generate each secret and write it to a file
openssl rand -base64 48 | tr -d '\n' > secrets/jwt_secret.txt
openssl rand -base64 32 | tr -d '\n' > secrets/postgres_password.txt
openssl rand -base64 32 | tr -d '\n' > secrets/redis_password.txt

# Restrict file permissions
chmod 600 secrets/*.txt
```

---

## Secret file formats

Each file contains the raw secret value with no trailing newline.

| File | Environment variable | Description |
|---|---|---|
| `secrets/jwt_secret.txt` | `JWT_SECRET` | HMAC-SHA256 signing key, minimum 32 bytes |
| `secrets/postgres_password.txt` | `POSTGRES_PASSWORD` | PostgreSQL superuser password |
| `secrets/redis_password.txt` | `REDIS_PASSWORD` | Redis AUTH password |

---

## How the application reads secrets

The Go config package supports the `_FILE` suffix convention (Docker Secrets
standard pattern):

- Set `JWT_SECRET_FILE=/run/secrets/jwt_secret` and the app reads the secret
  from that file path.
- If `JWT_SECRET_FILE` is set, it takes precedence over `JWT_SECRET`.
- For backward compatibility: if only `JWT_SECRET` (plain env var) is set, it
  is used directly.

This is supported for: `JWT_SECRET`, `POSTGRES_PASSWORD`, `REDIS_PASSWORD`.

---

## Current deployment model

The current deployment uses `deploy/secrets/env.*` files (loaded via Compose
`env_file:`). Docker Secrets file support in the Go application allows future
migration to Docker Swarm secrets or Kubernetes secrets without changing the
application code.

To use Docker Secrets with Docker Compose:

```yaml
# In docker-compose.prod.yml — add top-level secrets block:
secrets:
  jwt_secret:
    file: ./secrets/jwt_secret.txt
  postgres_password:
    file: ./secrets/postgres_password.txt
  redis_password:
    file: ./secrets/redis_password.txt

# In the api service — mount secrets and set _FILE env vars:
services:
  api:
    secrets:
      - jwt_secret
      - postgres_password
      - redis_password
    environment:
      JWT_SECRET_FILE: /run/secrets/jwt_secret
      POSTGRES_PASSWORD_FILE: /run/secrets/postgres_password
      REDIS_PASSWORD_FILE: /run/secrets/redis_password
```

This is NOT enabled by default — the current production topology uses
`deploy/secrets/env.production` (plain env file). See `docs/DEPLOYMENT_TOPOLOGY.md`.

---

## Security reminder

- Never commit these files. Git will reject them if `secrets/` is properly gitignored.
- Never print or log secret values.
- Use different secrets for staging and production.
- Rotate `JWT_SECRET` with care — rotation invalidates all active sessions.
