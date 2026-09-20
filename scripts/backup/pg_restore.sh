#!/usr/bin/env bash
# pg_restore.sh — restore a Dzeroth PostgreSQL backup produced by pg_backup.sh.
#
# WARNING: This script is DESTRUCTIVE. It drops and recreates the target database.
# Run it only with explicit operator intent and after verifying the backup file.
#
# Usage:
#   ./scripts/backup/pg_restore.sh <backup_file>
#
#   <backup_file> must be a .dump.gz file produced by pg_backup.sh.
#
# Required environment variables:
#   POSTGRES_HOST      — PostgreSQL host
#   POSTGRES_PORT      — PostgreSQL port (default: 5432)
#   POSTGRES_DB        — Target database name (will be dropped and recreated)
#   POSTGRES_USER      — PostgreSQL user (must have CREATEDB privilege)
#   PGPASSWORD         — PostgreSQL password
#
# Make this script executable before use:
#   chmod +x scripts/backup/pg_restore.sh

set -euo pipefail

# ── Arguments ─────────────────────────────────────────────────────────────────
if [ $# -ne 1 ]; then
    echo "Usage: $0 <backup_file.dump.gz>" >&2
    exit 1
fi

BACKUP_FILE="$1"

if [ ! -f "${BACKUP_FILE}" ]; then
    echo "ERROR: Backup file not found: ${BACKUP_FILE}" >&2
    exit 1
fi

# ── Configuration ─────────────────────────────────────────────────────────────
PG_HOST="${POSTGRES_HOST:?POSTGRES_HOST is required}"
PG_PORT="${POSTGRES_PORT:-5432}"
PG_DB="${POSTGRES_DB:?POSTGRES_DB is required}"
PG_USER="${POSTGRES_USER:?POSTGRES_USER is required}"

if [ -z "${PGPASSWORD:-}" ]; then
    echo "ERROR: PGPASSWORD is not set." >&2
    exit 1
fi

# ── Destructive warning ───────────────────────────────────────────────────────
echo ""
echo "WARNING: This operation will DROP and RECREATE the database '${PG_DB}' on ${PG_HOST}."
echo "All existing data in '${PG_DB}' will be permanently deleted."
echo "Backup file: ${BACKUP_FILE}"
echo ""
read -r -p "Type 'yes' to proceed: " CONFIRM
if [ "${CONFIRM}" != "yes" ]; then
    echo "Restore cancelled."
    exit 1
fi

# ── Restore ───────────────────────────────────────────────────────────────────
echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] Dropping and recreating database '${PG_DB}'..."

psql \
    --host="${PG_HOST}" \
    --port="${PG_PORT}" \
    --username="${PG_USER}" \
    --dbname="postgres" \
    --no-password \
    -c "DROP DATABASE IF EXISTS \"${PG_DB}\";"

psql \
    --host="${PG_HOST}" \
    --port="${PG_PORT}" \
    --username="${PG_USER}" \
    --dbname="postgres" \
    --no-password \
    -c "CREATE DATABASE \"${PG_DB}\" OWNER \"${PG_USER}\";"

echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] Restoring from ${BACKUP_FILE}..."

# Decompress and pipe into pg_restore. The -Fc format requires pg_restore (not psql).
# --no-owner avoids ownership mismatches when restoring to a different user.
# --no-privileges avoids GRANT/REVOKE errors in environments with different roles.
gunzip --stdout "${BACKUP_FILE}" | pg_restore \
    --host="${PG_HOST}" \
    --port="${PG_PORT}" \
    --username="${PG_USER}" \
    --dbname="${PG_DB}" \
    --no-password \
    --no-owner \
    --no-privileges \
    --exit-on-error

echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] Restore complete."
