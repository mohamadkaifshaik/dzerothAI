# Dzeroth — Backup and Recovery

**Status:** `PHASE 8D-4 — Backup strategy implemented`

This document describes the PostgreSQL backup strategy for Dzeroth, including
how to run backups manually, the recommended cron schedule, retention policy,
restoration procedure, and testing expectations.

---

## Scope

PostgreSQL holds all durable application state. Redis holds rate-limit counters
(ephemeral; no durable application data). This document covers PostgreSQL backup.

---

## Limitation

**This is a basic `pg_dump` backup strategy suitable for single-instance
self-hosted deployments.** Production deployments with strict RTO/RPO
requirements should use managed PostgreSQL (AWS RDS, Google Cloud SQL, Supabase,
Neon) with built-in point-in-time recovery (PITR). The scripts in this document
are not a substitute for managed database backup with PITR.

---

## Backup script

`scripts/backup/pg_backup.sh` performs a `pg_dump` and stores the compressed
output as `<db>_<timestamp>.dump.gz`.

Make the script executable before use:

```bash
chmod +x scripts/backup/pg_backup.sh
chmod +x scripts/backup/pg_restore.sh
```

### Required environment variables

| Variable | Description |
|---|---|
| `POSTGRES_HOST` | PostgreSQL host |
| `POSTGRES_PORT` | PostgreSQL port (default: 5432) |
| `POSTGRES_DB` | Database name |
| `POSTGRES_USER` | PostgreSQL user |
| `PGPASSWORD` | PostgreSQL password (read by `pg_dump` automatically) |
| `BACKUP_DIR` | Directory to store backups (default: `/var/backups/dzeroth`) |

**Secrets:** never embed `PGPASSWORD` in the script. Set it in the environment
before running. Never commit it to version control.

### Manual run example

```bash
export POSTGRES_HOST=localhost
export POSTGRES_PORT=5432
export POSTGRES_DB=dzeroth
export POSTGRES_USER=dzeroth
export PGPASSWORD=<your-postgres-password>
export BACKUP_DIR=/var/backups/dzeroth

bash scripts/backup/pg_backup.sh
```

When running against the Docker Compose stack, execute inside the api container
or directly via docker exec on the postgres container:

```bash
# From the host, via docker exec on the postgres container:
PGPASSWORD=<password> docker exec -i <postgres-container> \
  pg_dump -U dzeroth -d dzeroth -Fc \
  | gzip > /var/backups/dzeroth/dzeroth_$(date +%Y%m%dT%H%M%SZ).dump.gz
```

---

## Cron schedule

### Recommended schedule

| Frequency | Cron expression | Description |
|---|---|---|
| Daily | `0 2 * * *` | Run at 02:00 UTC every day |
| Weekly | `0 1 * * 0` | Run at 01:00 UTC every Sunday (in addition to daily) |

Example crontab entry (replace placeholders):

```cron
0 2 * * * POSTGRES_HOST=localhost POSTGRES_PORT=5432 POSTGRES_DB=dzeroth \
  POSTGRES_USER=dzeroth PGPASSWORD=<secret> BACKUP_DIR=/var/backups/dzeroth \
  /opt/dzeroth/scripts/backup/pg_backup.sh >> /var/log/dzeroth_backup.log 2>&1
```

---

## Storage and retention

### Local path

Backups are stored at `$BACKUP_DIR` (default: `/var/backups/dzeroth`). The host
volume must have sufficient free space — estimate ~2–5× the uncompressed database
size for a day's backup.

### Off-host storage (required for production)

Local backup files protect only against database corruption, not against host
failure. **Copy backup files off-host after each successful run**, for example:

```bash
# rsync to a separate storage host
rsync -az /var/backups/dzeroth/ backup-host:/backups/dzeroth/

# or sync to object storage (aws s3 example):
aws s3 sync /var/backups/dzeroth/ s3://your-backup-bucket/dzeroth/
```

Automating off-host copy as part of the cron job is strongly recommended.

### Retention recommendation

| Backup type | Keep | Total |
|---|---|---|
| Daily | 7 most recent | 7 days of daily backups |
| Weekly | 4 most recent | 4 weeks of weekly backups |

Example retention cleanup (add to cron after backup):

```bash
# Keep the 7 most recent daily backups (modify path/pattern as needed)
ls -t /var/backups/dzeroth/dzeroth_*.dump.gz | tail -n +8 | xargs rm -f
```

---

## Restoration procedure

### Step-by-step restore

**Warning: restoration drops and recreates the target database. All existing data
is lost. Perform this only with explicit operator intent.**

1. Stop the API container to prevent new writes during restore:
   ```bash
   docker compose -f docker-compose.prod.yml stop api
   ```

2. Identify the backup file to restore:
   ```bash
   ls -lh /var/backups/dzeroth/
   # Example: dzeroth_dzeroth_20260920T020000Z.dump.gz
   ```

3. Set environment variables:
   ```bash
   export POSTGRES_HOST=localhost
   export POSTGRES_PORT=5432
   export POSTGRES_DB=dzeroth
   export POSTGRES_USER=dzeroth
   export PGPASSWORD=<your-postgres-password>
   ```

4. Run the restore script:
   ```bash
   bash scripts/backup/pg_restore.sh /var/backups/dzeroth/dzeroth_dzeroth_<timestamp>.dump.gz
   ```
   The script will prompt: `Type 'yes' to proceed` before dropping the database.

5. Restart the API:
   ```bash
   IMAGE_TAG=<tag> docker compose -f docker-compose.prod.yml up -d api
   ```

6. Verify health:
   ```bash
   docker exec $(docker compose -f docker-compose.prod.yml ps -q api) \
     wget -qO- http://localhost:9091/readyz
   # Expected: {"status":"ready"}
   ```

7. Verify migration state:
   ```bash
   docker exec $(docker compose -f docker-compose.prod.yml ps -q postgres) \
     psql -U dzeroth -d dzeroth -c \
     "SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1;"
   ```

8. Document the incident: record what happened, which backup was used, and what
   data loss occurred (if any).

---

## Restore testing expectations

**Backup files that are never tested are unreliable.** The backup-restore path
should be tested before any production release (see `RELEASE_CHECKLIST.md`).

### Monthly restore drill

Once per month, on a staging/test database:

1. Copy a recent production backup file to a test host.
2. Run the full restore procedure against the staging database.
3. Start the API against the restored database.
4. Run the post-deploy smoke test (`/health`, `/readyz`).
5. Verify that a known data record (e.g., a test user) is present.
6. Record the test date and result.

---

## Backup integrity check

After each backup, verify the file is non-empty and not corrupt:

```bash
# Verify the file can be decompressed (does not need to fully decompress)
gunzip --test /var/backups/dzeroth/dzeroth_<timestamp>.dump.gz && echo "OK"

# List the table of contents (proves the archive is a valid pg_dump)
pg_restore --list /var/backups/dzeroth/dzeroth_<timestamp>.dump.gz | head -20
```

---

## Relationship to RELEASE_CHECKLIST.md

The release checklist item "PostgreSQL backup strategy confirmed" is satisfied
by the existence of these scripts and this document. Before a production release
is considered ready:

- [ ] `scripts/backup/pg_backup.sh` has been run manually and produces a backup.
- [ ] `scripts/backup/pg_restore.sh` has been run on a staging database.
- [ ] Off-host backup copy is configured.
- [ ] Cron schedule is installed on the production host.
