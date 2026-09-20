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
    docker compose -f "${COMPOSE_FILE}" \
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
# We decompress on the host and pipe into the container's pg_restore via stdin.
# Using process substitution to feed stdin from the decompressed dump.
# The `sh -c` wrapper reads from /dev/stdin inside the container.
if ! gunzip -c "${TMPFILE}" \
    | compose_exec sh -c 'pg_restore --list /dev/stdin' > /dev/null; then
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
