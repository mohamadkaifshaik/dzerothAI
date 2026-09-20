#!/usr/bin/env bash
# pg_backup.sh — Container-native PostgreSQL backup for Dzeroth on AWS EC2.
#
# Architecture:
#   pg_dump runs INSIDE the postgres container via `docker compose exec -T`.
#   Port 5432 is NOT exposed to the host and must remain that way.
#   PGPASSWORD is NOT handled by the host script — pg_dump reads it from the
#   container's own environment (loaded from the Compose env_file). The host
#   script does not need to know or pass the password.
#
# S3 retention strategy:
#   This script does NOT delete S3 objects. Local retention is handled here
#   (files older than RETAIN_DAILY days are removed after a successful S3
#   upload + verification). S3 retention is handled by an S3 Lifecycle rule
#   set on the bucket, which is the safer and more auditable approach:
#   - Script-based S3 deletion requires s3:DeleteObject in the IAM policy,
#     which is a broader permission than necessary for backup-only access.
#   - S3 Lifecycle configuration operates independently of the backup host,
#     so a host failure or script bug cannot accidentally delete the only copy.
#   - Lifecycle rules are auditable in the AWS console and version-controlled
#     via IaC if desired.
#   Recommended S3 lifecycle: transition to S3-IA after 30 days, expire after
#   90 days. Configure this on the bucket, not here.
#
# IAM requirements (EC2 instance role DzerothProductionBackupRole):
#   s3:PutObject      — upload backup
#   s3:GetObject      — head-object verification (head-object uses s3:GetObject)
#   s3:ListBucket     — needed by some AWS CLI versions for head-object
#   NO s3:DeleteObject needed — retention is via lifecycle, not this script.
#
# Usage:
#   ./scripts/backup/pg_backup.sh
#
# All variables are configurable via environment. Documented defaults below.
# Secrets come from the container environment, not the host script.
#
# Cron/systemd: see deploy/systemd/dzeroth-backup.{service,timer}

set -euo pipefail

# ── Derived paths ─────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ── Configuration (all overridable via environment) ───────────────────────────
COMPOSE_FILE="${COMPOSE_FILE:-${SCRIPT_DIR}/../../docker-compose.prod.yml}"
POSTGRES_SERVICE="${POSTGRES_SERVICE:-postgres}"
POSTGRES_DB="${POSTGRES_DB:-dzeroth}"
POSTGRES_USER="${POSTGRES_USER:-dzeroth}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/dzeroth}"
S3_BUCKET="${S3_BUCKET:-dzeroth-production-postgres-backups-2026}"
S3_PREFIX="${S3_PREFIX:-postgres}"
RETAIN_DAILY="${RETAIN_DAILY:-7}"

# Resolve the compose file to an absolute path for consistent log output.
COMPOSE_FILE="$(cd "$(dirname "${COMPOSE_FILE}")" && pwd)/$(basename "${COMPOSE_FILE}")"

# ── Logging ───────────────────────────────────────────────────────────────────
log_info() {
    echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] [INFO]  $*"
}

log_error() {
    echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] [ERROR] $*" >&2
}

# ── Secrets file detection ────────────────────────────────────────────────────
COMPOSE_DIR="$(dirname "${COMPOSE_FILE}")"
SECRETS_FILE="${COMPOSE_DIR}/deploy/secrets/env.production"

# The Compose file lives at <repo-root>/docker-compose.prod.yml.
# deploy/secrets/env.production is at <repo-root>/deploy/secrets/env.production.
# Resolve relative to the repo root (same directory as the compose file).
if [[ ! -f "${SECRETS_FILE}" ]]; then
    log_error "Secrets file not found: ${SECRETS_FILE}"
    log_error "Create deploy/secrets/env.production from deploy/env.production.example."
    log_error "See docs/DEPLOYMENT_TOPOLOGY.md — Secrets provisioning."
    exit 1
fi

# ── Docker Compose binary resolution ─────────────────────────────────────────
# Docker Compose v2 is a CLI plugin that may only be installed per-user
# (e.g. ~/.docker/cli-plugins/docker-compose). When this script runs as root
# under systemd, root's Docker CLI has no plugin path and `docker compose`
# fails with "is not a docker command".
#
# Resolution order:
#   1. DOCKER_COMPOSE_BIN env override (explicit / testable)
#   2. System-wide plugin locations (preferred — accessible to root)
#   3. /home/ec2-user/.docker/cli-plugins/docker-compose (user-space fallback)
#
# Deployment prerequisite — create the system-wide symlink once on the host:
#   sudo mkdir -p /usr/local/lib/docker/cli-plugins
#   sudo ln -sf /home/ec2-user/.docker/cli-plugins/docker-compose \
#       /usr/local/lib/docker/cli-plugins/docker-compose
find_docker_compose() {
    # 1. Explicit override — highest priority, useful for testing and overrides.
    if [[ -n "${DOCKER_COMPOSE_BIN:-}" ]]; then
        if [[ -x "${DOCKER_COMPOSE_BIN}" ]]; then
            DOCKER_COMPOSE_CMD="${DOCKER_COMPOSE_BIN}"
            log_info "Docker Compose binary (override): ${DOCKER_COMPOSE_CMD}"
            return 0
        else
            log_error "DOCKER_COMPOSE_BIN is set but not executable: ${DOCKER_COMPOSE_BIN}"
            exit 1
        fi
    fi

    # 2. System-wide plugin locations — accessible to all users including root.
    local _dc_paths=(
        /usr/local/lib/docker/cli-plugins/docker-compose
        /usr/libexec/docker/cli-plugins/docker-compose
        /usr/lib/docker/cli-plugins/docker-compose
        /usr/local/libexec/docker/cli-plugins/docker-compose
        /usr/local/bin/docker-compose
        /usr/bin/docker-compose
    )
    local _p
    for _p in "${_dc_paths[@]}"; do
        if [[ -x "${_p}" ]]; then
            DOCKER_COMPOSE_CMD="${_p}"
            log_info "Docker Compose binary (system): ${DOCKER_COMPOSE_CMD}"
            return 0
        fi
    done

    # 3. User-space fallback — ec2-user's plugin directory.
    # Root can execute binaries owned by other users.
    # This is where the official Docker install script places the plugin on
    # Amazon Linux 2023 when installed as a non-root user.
    local _user_path="/home/ec2-user/.docker/cli-plugins/docker-compose"
    if [[ -x "${_user_path}" ]]; then
        DOCKER_COMPOSE_CMD="${_user_path}"
        log_info "Docker Compose binary (user-space fallback): ${DOCKER_COMPOSE_CMD}"
        return 0
    fi

    log_error "Docker Compose binary not found."
    log_error "Tried system paths: ${_dc_paths[*]}"
    log_error "Tried user-space: ${_user_path}"
    log_error "To fix, create a system-wide symlink:"
    log_error "  sudo mkdir -p /usr/local/lib/docker/cli-plugins"
    log_error "  sudo ln -sf /home/ec2-user/.docker/cli-plugins/docker-compose \\"
    log_error "      /usr/local/lib/docker/cli-plugins/docker-compose"
    log_error "Or set DOCKER_COMPOSE_BIN=/path/to/docker-compose."
    exit 1
}

find_docker_compose

# ── Setup ─────────────────────────────────────────────────────────────────────
if [[ ! -d "${BACKUP_DIR}" ]]; then
    mkdir -p "${BACKUP_DIR}"
    chmod 700 "${BACKUP_DIR}"
    log_info "Created backup directory: ${BACKUP_DIR}"
fi

# ── Filename ──────────────────────────────────────────────────────────────────
TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"
FILENAME="dzeroth_${TIMESTAMP}.dump.gz"
TMPFILE="${BACKUP_DIR}/${FILENAME}.tmp"
FINALFILE="${BACKUP_DIR}/${FILENAME}"

# ── Trap: remove temp file on error or signal ─────────────────────────────────
cleanup() {
    local exit_code=$?
    if [[ -f "${TMPFILE}" ]]; then
        log_error "Removing incomplete temp file: ${TMPFILE}"
        rm -f "${TMPFILE}"
    fi
    if [[ ${exit_code} -ne 0 ]]; then
        log_error "Backup failed (exit code ${exit_code}). No partial file remains."
    fi
}
trap cleanup EXIT

# ── Compose exec helper ───────────────────────────────────────────────────────
# Runs a command inside the already-running postgres container.
#
# --env-file is intentionally NOT passed here. That flag controls Compose
# variable substitution at service-startup time; it is not needed for exec
# because the container is already running with its full environment injected
# by the `env_file:` directive in docker-compose.prod.yml. pg_dump reads
# PGPASSWORD from the container's own running environment — the host script
# never handles the database password.
#
# Excluding --env-file also ensures compatibility across Docker Compose v2
# versions: --env-file as a global compose flag was added in v2.2.x and is
# absent from older system-installed versions. Passing it to those versions
# causes the Docker CLI itself to reject it with "unknown flag: --env-file".
compose_exec() {
    "${DOCKER_COMPOSE_CMD}" -f "${COMPOSE_FILE}" \
        exec -T "${POSTGRES_SERVICE}" "$@"
}

# ── Verify the postgres container is running ──────────────────────────────────
log_info "Verifying postgres container is reachable..."
if ! compose_exec pg_isready -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -q; then
    log_error "postgres container is not ready. Aborting."
    exit 1
fi

# ── Dump ──────────────────────────────────────────────────────────────────────
log_info "Starting pg_dump: service=${POSTGRES_SERVICE} db=${POSTGRES_DB} -> ${TMPFILE}"

# PGPASSWORD is NOT passed from the host. pg_dump reads it from the container's
# own environment, which is loaded from the Compose env_file. The host script
# never sees or handles the database password.
#
# The pipeline uses `pipefail` (set at the top). A non-zero exit from pg_dump
# propagates through the pipe and causes the script to fail, leaving only the
# .tmp file which the EXIT trap removes.
compose_exec pg_dump \
    -U "${POSTGRES_USER}" \
    -d "${POSTGRES_DB}" \
    -Fc \
    | gzip -c > "${TMPFILE}"

log_info "pg_dump + gzip complete."

# ── Integrity checks ──────────────────────────────────────────────────────────
log_info "Running integrity checks..."

# Check 1: file is non-empty
if [[ ! -s "${TMPFILE}" ]]; then
    log_error "Backup file is empty: ${TMPFILE}"
    exit 1
fi
log_info "Check 1/3: file is non-empty — PASS"

# Check 2: gzip integrity
if ! gzip -t "${TMPFILE}"; then
    log_error "gzip integrity check failed: ${TMPFILE}"
    exit 1
fi
log_info "Check 2/3: gzip integrity — PASS"

# Check 3: pg_restore can list the archive contents.
# Decompress on the host and pipe the raw PostgreSQL custom-format archive
# into the container's pg_restore via stdin.
#
# Use `-` (dash) as the archive argument — this tells pg_restore to read from
# fd 0 (its actual stdin), which docker compose exec -T connects directly to
# the host pipe. Do NOT use /dev/stdin: in the official postgres Docker image
# /dev/stdin is a char device node (not a symlink to /proc/self/fd/0), so
# pg_restore opening it receives no data and fails with "did not find magic
# string in file header".
#
# No `sh -c` wrapper is needed — compose_exec exec's pg_restore directly so
# its fd 0 IS the forwarded pipe. This matches how pg_restore.sh restores.
if ! gunzip -c "${TMPFILE}" \
    | compose_exec pg_restore --list - > /dev/null; then
    log_error "pg_restore --list check failed: archive may be corrupt."
    exit 1
fi
log_info "Check 3/3: pg_restore --list — PASS"

# ── Atomic rename ─────────────────────────────────────────────────────────────
mv "${TMPFILE}" "${FINALFILE}"
chmod 600 "${FINALFILE}"
log_info "Backup finalised: ${FINALFILE}"

BACKUP_SIZE="$(du -h "${FINALFILE}" | cut -f1)"
log_info "Backup size: ${BACKUP_SIZE}"

# Disarm the trap now that the final file exists and the temp file is gone.
trap - EXIT

# ── S3 upload ─────────────────────────────────────────────────────────────────
S3_KEY="${S3_PREFIX}/${FILENAME}"
S3_URI="s3://${S3_BUCKET}/${S3_KEY}"

log_info "Uploading to S3: ${S3_URI}"
aws s3 cp "${FINALFILE}" "${S3_URI}" --sse aws:s3
log_info "S3 upload complete."

# ── S3 verification ───────────────────────────────────────────────────────────
log_info "Verifying S3 object exists: bucket=${S3_BUCKET} key=${S3_KEY}"
if ! aws s3api head-object --bucket "${S3_BUCKET}" --key "${S3_KEY}" > /dev/null; then
    log_error "S3 head-object verification failed. The upload may not have persisted."
    exit 1
fi
log_info "S3 verification — PASS"

# ── Local retention ───────────────────────────────────────────────────────────
# Delete local files older than RETAIN_DAILY days. Only runs after S3 upload
# and verification succeed, so we never delete the only copy.
# We do NOT delete S3 objects here — use an S3 Lifecycle rule instead.
# See the script header for the rationale.
log_info "Applying local retention: removing files older than ${RETAIN_DAILY} days..."
find "${BACKUP_DIR}" -maxdepth 1 -name "*.dump.gz" -mtime "+${RETAIN_DAILY}" -delete
log_info "Local retention complete."

# ── Done ──────────────────────────────────────────────────────────────────────
log_info "Backup successful: ${FINALFILE} (${BACKUP_SIZE}) -> ${S3_URI}"
