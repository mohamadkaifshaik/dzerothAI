#!/usr/bin/env bash
# pg_backup.sh — pg_dump backup script for Dzeroth PostgreSQL.
#
# Usage:
#   ./scripts/backup/pg_backup.sh
#
# Required environment variables:
#   POSTGRES_HOST      — PostgreSQL host
#   POSTGRES_PORT      — PostgreSQL port (default: 5432)
#   POSTGRES_DB        — Database name
#   POSTGRES_USER      — PostgreSQL user
#   PGPASSWORD         — PostgreSQL password (pg_dump reads this automatically)
#                        Set this to the value of POSTGRES_PASSWORD before running.
#
# Optional environment variables:
#   BACKUP_DIR         — Directory to store backups (default: /var/backups/dzeroth)
#
# Example cron (daily at 02:00):
#   0 2 * * * POSTGRES_HOST=localhost POSTGRES_PORT=5432 POSTGRES_DB=dzeroth \
#     POSTGRES_USER=dzeroth PGPASSWORD=<secret> BACKUP_DIR=/var/backups/dzeroth \
#     /path/to/scripts/backup/pg_backup.sh >> /var/log/dzeroth_backup.log 2>&1
#
# Make this script executable before use:
#   chmod +x scripts/backup/pg_backup.sh

set -euo pipefail

# ── Configuration ─────────────────────────────────────────────────────────────
BACKUP_DIR="${BACKUP_DIR:-/var/backups/dzeroth}"
PG_HOST="${POSTGRES_HOST:?POSTGRES_HOST is required}"
PG_PORT="${POSTGRES_PORT:-5432}"
PG_DB="${POSTGRES_DB:?POSTGRES_DB is required}"
PG_USER="${POSTGRES_USER:?POSTGRES_USER is required}"

# PGPASSWORD must be set in the environment before running this script.
# pg_dump reads it automatically. Never pass the password as a command-line flag.
if [ -z "${PGPASSWORD:-}" ]; then
    echo "ERROR: PGPASSWORD is not set. Set PGPASSWORD to the PostgreSQL password before running." >&2
    exit 1
fi

# ── Backup ────────────────────────────────────────────────────────────────────
TIMESTAMP=$(date -u +"%Y%m%dT%H%M%SZ")
BACKUP_FILE="${BACKUP_DIR}/dzeroth_${PG_DB}_${TIMESTAMP}.dump.gz"

echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] Starting backup: host=${PG_HOST} db=${PG_DB} → ${BACKUP_FILE}"

# Create backup directory if it does not exist.
mkdir -p "${BACKUP_DIR}"

# pg_dump with custom format (-Fc) compressed and piped through gzip for maximum
# compatibility. The custom format supports selective restore with pg_restore.
# -Fc produces a binary archive that is more efficient than SQL text dumps for
# large databases and supports parallel restore.
pg_dump \
    --host="${PG_HOST}" \
    --port="${PG_PORT}" \
    --username="${PG_USER}" \
    --dbname="${PG_DB}" \
    --format=custom \
    --no-password \
    | gzip > "${BACKUP_FILE}"

BACKUP_SIZE=$(du -h "${BACKUP_FILE}" | cut -f1)
echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] Backup complete: ${BACKUP_FILE} (${BACKUP_SIZE})"
