#!/usr/bin/env bash
# pg_restore.sh — Container-native PostgreSQL restore for Dzeroth.
#
# Architecture:
#   pg_restore runs INSIDE the postgres container via `docker compose exec -T`.
#   Port 5432 is NOT exposed to the host and must remain that way.
#   PGPASSWORD is NOT handled by the host script — pg_restore reads it from
#   the container's own environment (loaded from the Compose env_file).
#
# WARNING: This script is DESTRUCTIVE.
#   --clean --if-exists drops all existing objects before restoring.
#   All existing data in the target database is permanently replaced.
#   Stop the API container before running this script to prevent data races.
#
# Usage:
#   ./scripts/backup/pg_restore.sh [--confirm] <backup_file.dump.gz>
#
#   --confirm     Skip the interactive prompt (for automation / testing).
#                 Use with extreme care — this is a destructive operation.
#
# Example:
#   # Stop the API first
#   docker compose -f docker-compose.prod.yml stop api
#
#   # Restore
#   ./scripts/backup/pg_restore.sh /var/backups/dzeroth/dzeroth_20260920T020000Z.dump.gz
#
#   # Restart the API
#   IMAGE_TAG=<tag> docker compose -f docker-compose.prod.yml up -d api

set -euo pipefail

# ── Derived paths ─────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ── Configuration ─────────────────────────────────────────────────────────────
COMPOSE_FILE="${COMPOSE_FILE:-${SCRIPT_DIR}/../../docker-compose.prod.yml}"
POSTGRES_SERVICE="${POSTGRES_SERVICE:-postgres}"
POSTGRES_DB="${POSTGRES_DB:-dzeroth}"
POSTGRES_USER="${POSTGRES_USER:-dzeroth}"

COMPOSE_FILE="$(cd "$(dirname "${COMPOSE_FILE}")" && pwd)/$(basename "${COMPOSE_FILE}")"

# ── Logging ───────────────────────────────────────────────────────────────────
log_info() {
    echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] [INFO]  $*"
}

log_error() {
    echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] [ERROR] $*" >&2
}

# ── Argument parsing ──────────────────────────────────────────────────────────
CONFIRMED=false
BACKUP_FILE=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --confirm)
            CONFIRMED=true
            shift
            ;;
        -*)
            log_error "Unknown flag: $1"
            echo "Usage: $0 [--confirm] <backup_file.dump.gz>" >&2
            exit 1
            ;;
        *)
            if [[ -n "${BACKUP_FILE}" ]]; then
                log_error "Too many arguments."
                echo "Usage: $0 [--confirm] <backup_file.dump.gz>" >&2
                exit 1
            fi
            BACKUP_FILE="$1"
            shift
            ;;
    esac
done

if [[ -z "${BACKUP_FILE}" ]]; then
    log_error "No backup file specified."
    echo "Usage: $0 [--confirm] <backup_file.dump.gz>" >&2
    exit 1
fi

if [[ ! -f "${BACKUP_FILE}" ]]; then
    log_error "Backup file not found: ${BACKUP_FILE}"
    exit 1
fi

# ── Secrets file detection ────────────────────────────────────────────────────
COMPOSE_DIR="$(dirname "${COMPOSE_FILE}")"
SECRETS_FILE="${COMPOSE_DIR}/deploy/secrets/env.production"

if [[ ! -f "${SECRETS_FILE}" ]]; then
    log_error "Secrets file not found: ${SECRETS_FILE}"
    log_error "Create deploy/secrets/env.production from deploy/env.production.example."
    exit 1
fi

# ── Compose exec helper ───────────────────────────────────────────────────────
# Runs a command inside the already-running postgres container.
# --env-file is intentionally NOT passed — see pg_backup.sh for full rationale.
# Summary: exec operates on an already-running container; PGPASSWORD is already
# in the container's environment from startup. --env-file is not supported by
# older Docker Compose v2 versions installed at the system level.
compose_exec() {
    docker compose -f "${COMPOSE_FILE}" \
        exec -T "${POSTGRES_SERVICE}" "$@"
}

# ── Destructive operation warning ─────────────────────────────────────────────
echo ""
echo "========================================================================"
echo "  DESTRUCTIVE OPERATION WARNING"
echo "========================================================================"
echo ""
echo "  This will RESTORE the database '${POSTGRES_DB}' from:"
echo "  ${BACKUP_FILE}"
echo ""
echo "  The restore uses --clean --if-exists, which drops all existing"
echo "  objects in '${POSTGRES_DB}' before restoring from the backup."
echo "  ALL CURRENT DATA IN '${POSTGRES_DB}' WILL BE PERMANENTLY REPLACED."
echo ""
echo "  Prerequisites:"
echo "    1. The API container must be stopped before restoring."
echo "       docker compose -f docker-compose.prod.yml stop api"
echo "    2. Verify this is the correct backup file."
echo "    3. Ensure you have a separate copy of the current database if needed."
echo ""
echo "========================================================================"
echo ""

if [[ "${CONFIRMED}" == "false" ]]; then
    read -r -p "Type 'yes' to proceed with the restore: " CONFIRM_INPUT
    if [[ "${CONFIRM_INPUT}" != "yes" ]]; then
        echo "Restore cancelled."
        exit 1
    fi
else
    log_info "--confirm flag provided. Proceeding without interactive prompt."
fi

# ── Pre-restore integrity check ───────────────────────────────────────────────
log_info "Verifying backup file integrity before restore..."

if ! gzip -t "${BACKUP_FILE}"; then
    log_error "gzip integrity check failed on backup file: ${BACKUP_FILE}"
    exit 1
fi
log_info "Backup file gzip integrity — PASS"

# ── Verify the postgres container is running ──────────────────────────────────
log_info "Verifying postgres container is reachable..."
if ! compose_exec pg_isready -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -q 2>/dev/null; then
    log_error "postgres container is not ready. Is it running?"
    exit 1
fi

# ── Restore ───────────────────────────────────────────────────────────────────
log_info "Restoring '${POSTGRES_DB}' from ${BACKUP_FILE}..."
log_info "Using: --clean --if-exists (drops existing objects before restoring)"

# Decompress on the host and pipe into the container's pg_restore via stdin.
# PGPASSWORD is read from the container environment — not passed on the command line.
# -Fc — custom format (matches pg_backup.sh which produces -Fc dumps)
# --clean --if-exists — drop existing objects before creating them
# --no-owner — skip ownership assignments (avoids role mismatches)
# --no-privileges — skip GRANT/REVOKE (avoids role mismatches)
# --exit-on-error — stop on first error rather than continuing with a partial restore
# - (final arg) — read archive from stdin
gunzip -c "${BACKUP_FILE}" \
    | compose_exec pg_restore \
        -U "${POSTGRES_USER}" \
        -d "${POSTGRES_DB}" \
        -Fc \
        --clean \
        --if-exists \
        --no-owner \
        --no-privileges \
        --exit-on-error \
        -

log_info "Restore complete: database '${POSTGRES_DB}' restored from ${BACKUP_FILE}"
echo ""
echo "Next steps:"
echo "  1. Verify migration state:"
echo "     docker compose -f docker-compose.prod.yml exec postgres \\"
echo "       psql -U ${POSTGRES_USER} -d ${POSTGRES_DB} \\"
echo "       -c \"SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1;\""
echo ""
echo "  2. Restart the API:"
echo "     IMAGE_TAG=<tag> docker compose -f docker-compose.prod.yml up -d api"
echo ""
echo "  3. Verify readiness:"
echo "     docker exec \$(docker compose -f docker-compose.prod.yml ps -q api) \\"
echo "       wget -qO- http://localhost:9091/readyz"
