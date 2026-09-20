# Dzeroth — Backup and Recovery

**Status:** `PHASE 8D-4 — S3 backup with Docker-native pg_dump and systemd timer`

This document is the authoritative operational reference for Dzeroth's
PostgreSQL backup strategy on AWS EC2.

---

## Scope

PostgreSQL holds all durable application state. Redis holds rate-limit
counters (ephemeral; no durable application data). This document covers
PostgreSQL backup only.

---

## Limitations

This is a `pg_dump`-based backup strategy for single-instance self-hosted
deployments.

Known limitations:

- **No point-in-time recovery (PITR).** Recovery granularity is the last
  successful backup, typically 24 hours behind.
- **Single host.** All services run on one EC2 instance. A total host failure
  loses all data written since the last backup. S3 off-host storage mitigates
  this for the backup files themselves.
- **No cross-region replication.** Backups land in `ap-south-2`. An S3
  regional failure would lose access to them. For stronger durability, add
  cross-region replication in the S3 bucket configuration.
- **Not a substitute for managed PostgreSQL.** Production deployments with
  strict RTO/RPO requirements should use AWS RDS, Google Cloud SQL, Supabase,
  or Neon with built-in PITR.

---

## Architecture

```
EC2 host (dzeroth-production, ap-south-2)
  │
  ├── docker compose (docker-compose.prod.yml)
  │     ├── postgres  (port 5432 NOT host-exposed)
  │     ├── redis     (port 6379 NOT host-exposed)
  │     └── api       (127.0.0.1:8080)
  │
  ├── /home/ec2-user/dzerothAI/scripts/backup/pg_backup.sh
  │     │
  │     ├── docker compose exec -T postgres pg_dump ...
  │     │     └── pg_dump runs INSIDE the postgres container
  │     │         PGPASSWORD is read from container environment
  │     │         (never handled by the host script)
  │     │
  │     ├── gzip | /var/backups/dzeroth/<filename>.dump.gz
  │     ├── gzip -t (integrity)
  │     ├── pg_restore --list (archive validity)
  │     └── aws s3 cp + s3api head-object (upload + verify)
  │
  └── S3 bucket: dzeroth-production-postgres-backups-2026 (ap-south-2)
        IAM role: DzerothProductionBackupRole (IMDS — no stored keys)
        SSE: aws:s3
        Versioning: enabled
        Lifecycle: see "S3 retention" section below
```

Port 5432 is NOT exposed to the EC2 host network and must remain that way.
`pg_dump` and `pg_restore` run inside the `postgres` container. The host
does not need `psql` or `pg_dump` installed.

---

## Backup script

`/home/ec2-user/dzerothAI/scripts/backup/pg_backup.sh`

Make executable after cloning or pulling:

```bash
chmod +x /home/ec2-user/dzerothAI/scripts/backup/pg_backup.sh
chmod +x /home/ec2-user/dzerothAI/scripts/backup/pg_restore.sh
```

### Configuration variables (all have defaults)

| Variable | Default | Description |
|---|---|---|
| `COMPOSE_FILE` | `<repo>/docker-compose.prod.yml` | Path to the production Compose file |
| `POSTGRES_SERVICE` | `postgres` | Compose service name for PostgreSQL |
| `POSTGRES_DB` | `dzeroth` | Database name |
| `POSTGRES_USER` | `dzeroth` | PostgreSQL user |
| `BACKUP_DIR` | `/var/backups/dzeroth` | Local directory for backup files |
| `S3_BUCKET` | `dzeroth-production-postgres-backups-2026` | Target S3 bucket |
| `S3_PREFIX` | `postgres` | S3 key prefix |
| `RETAIN_DAILY` | `7` | Days to retain local backup files |

### What the backup script does (step by step)

1. Resolves the Compose file path and verifies `deploy/secrets/env.production` exists.
2. Creates `$BACKUP_DIR` (`/var/backups/dzeroth`) with mode 0700 if absent.
3. Generates filename: `dzeroth_<YYYYMMDDTHHMMSSZ>.dump.gz` (UTC timestamp).
4. Creates a `.tmp` file with an `EXIT` trap — the trap removes it on any failure so no partial file survives.
5. Verifies the `postgres` container is ready (`pg_isready`).
6. Runs `docker compose exec -T postgres pg_dump -U dzeroth -d dzeroth -Fc` piped through `gzip -c` into the `.tmp` file. `PGPASSWORD` is read from the container's own environment — the host script never handles the password.
7. Integrity check 1: file is non-empty.
8. Integrity check 2: `gzip -t` (compressed file is not corrupt).
9. Integrity check 3: `gunzip -c | docker compose exec -T postgres pg_restore --list /dev/stdin` (archive is a valid pg_dump custom-format archive).
10. Atomically renames `.tmp` to the final filename. Sets `chmod 600` on the file.
11. Uploads to S3: `aws s3 cp <file> s3://<bucket>/postgres/<filename> --sse aws:s3`. AWS credentials come from the EC2 IAM role via IMDS — no stored keys.
12. Verifies the upload: `aws s3api head-object --bucket <bucket> --key postgres/<filename>`. Fails the script if the object is not found.
13. Applies local retention: `find $BACKUP_DIR -name "*.dump.gz" -mtime +7 -delete`. Only runs after successful S3 upload and verification.
14. Logs success and exits 0.

### Local backup location and permissions

- Directory: `/var/backups/dzeroth/` (mode 0700, owned by root)
- Files: mode 0600, owned by root
- Naming: `dzeroth_<YYYYMMDDTHHMMSSZ>.dump.gz`

### S3 upload and IAM authentication

The EC2 instance runs with the `DzerothProductionBackupRole` IAM role. The AWS
CLI picks up credentials automatically from the instance metadata service
(IMDS, `169.254.169.254`). No `AWS_ACCESS_KEY_ID` or `AWS_SECRET_ACCESS_KEY`
are needed or used.

Required IAM permissions on the role:

```json
{
  "Effect": "Allow",
  "Action": [
    "s3:PutObject",
    "s3:GetObject",
    "s3:ListBucket"
  ],
  "Resource": [
    "arn:aws:s3:::dzeroth-production-postgres-backups-2026",
    "arn:aws:s3:::dzeroth-production-postgres-backups-2026/*"
  ]
}
```

`s3:DeleteObject` is NOT required — S3 retention is managed by a Lifecycle
rule, not by the backup script.

---

## Retention strategy

### Local retention

The backup script deletes local files older than `RETAIN_DAILY` (default: 7)
days using `find ... -mtime +7 -delete`. This runs only after the S3 upload
and head-object verification succeed, so local deletion never removes the only
copy of a backup.

### S3 retention

S3 retention is handled by an S3 Lifecycle rule — not by the backup script.

**Rationale:** Script-based S3 deletion requires `s3:DeleteObject` in the IAM
policy. If the backup host is compromised, `s3:DeleteObject` becomes an attack
surface for destroying backup history. S3 Lifecycle configuration operates
independently of the backup host and requires no additional IAM permissions on
the backup role. It is more auditable (visible in the AWS console and version-
controllable via IaC) and cannot be disabled by a script bug or host failure.

**Recommended S3 Lifecycle rule:**

- Transition to S3 Standard-IA: after 30 days (cost reduction)
- Expire objects: after 90 days

#### Configure via AWS console

1. Open S3 in the AWS console, region `ap-south-2`.
2. Navigate to bucket `dzeroth-production-postgres-backups-2026`.
3. Click "Management" tab, then "Create lifecycle rule".
4. Rule name: `dzeroth-postgres-backup-retention`
5. Filter: prefix `postgres/`
6. Lifecycle rule actions:
   - Transition current versions to Standard-IA after 30 days.
   - Expire current versions after 90 days.
7. Save the rule.

#### Configure via AWS CLI

```bash
aws s3api put-bucket-lifecycle-configuration \
  --bucket dzeroth-production-postgres-backups-2026 \
  --lifecycle-configuration '{
    "Rules": [{
      "ID": "dzeroth-postgres-backup-retention",
      "Filter": {"Prefix": "postgres/"},
      "Status": "Enabled",
      "Transitions": [{
        "Days": 30,
        "StorageClass": "STANDARD_IA"
      }],
      "Expiration": {
        "Days": 90
      }
    }]
  }'
```

---

## Scheduled execution — systemd timer

The backup runs daily at 02:00 UTC via a systemd timer. This is preferred
over a user crontab because:

- `systemctl status dzeroth-backup.timer` shows the last run time, next run
  time, and last exit code.
- All output goes to journald and is queryable with `journalctl`.
- `Persistent=true` on the timer catches missed runs after system downtime.
- `After=docker.service Requires=docker.service` ensures Docker is running
  before the backup starts.

### Installation

```bash
sudo cp /home/ec2-user/dzerothAI/deploy/systemd/dzeroth-backup.service /etc/systemd/system/
sudo cp /home/ec2-user/dzerothAI/deploy/systemd/dzeroth-backup.timer   /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now dzeroth-backup.timer
```

### Checking status and logs

```bash
# Timer status and next scheduled run
sudo systemctl status dzeroth-backup.timer

# Service exit status from last run
sudo systemctl status dzeroth-backup.service

# Backup logs — live tail
sudo journalctl -u dzeroth-backup.service -f

# Backup logs — most recent first
sudo journalctl -u dzeroth-backup.service -r

# Backup logs — last 24 hours
sudo journalctl -u dzeroth-backup.service --since "24 hours ago"
```

See `deploy/systemd/README.md` for complete installation and update instructions.

---

## Manual backup run

```bash
# Trigger immediately via systemd (recommended — captures logs in journald)
sudo systemctl start dzeroth-backup.service
sudo journalctl -u dzeroth-backup.service -f

# Or run the script directly
sudo /home/ec2-user/dzerothAI/scripts/backup/pg_backup.sh
```

---

## Restore procedure

### Stop the API first

The API must be stopped before restoring to prevent data races:

```bash
docker compose -f /home/ec2-user/dzerothAI/docker-compose.prod.yml stop api
```

### Identify the backup file to restore

```bash
# List local backups
ls -lh /var/backups/dzeroth/

# List S3 backups (most recent first)
aws s3 ls s3://dzeroth-production-postgres-backups-2026/postgres/ \
  --recursive | sort -k1,2r | head -20

# Download a specific backup from S3 (if not available locally)
aws s3 cp \
  s3://dzeroth-production-postgres-backups-2026/postgres/dzeroth_<timestamp>.dump.gz \
  /var/backups/dzeroth/dzeroth_<timestamp>.dump.gz
```

### Run the restore script

```bash
# Interactive (prompts "Type 'yes' to proceed")
sudo /home/ec2-user/dzerothAI/scripts/backup/pg_restore.sh \
  /var/backups/dzeroth/dzeroth_<timestamp>.dump.gz

# Non-interactive (automation, staging drills)
sudo /home/ec2-user/dzerothAI/scripts/backup/pg_restore.sh --confirm \
  /var/backups/dzeroth/dzeroth_<timestamp>.dump.gz
```

The restore script:
1. Verifies gzip integrity of the backup file before restoring.
2. Prints a destructive-operation warning with the target database name.
3. Requires `--confirm` flag or interactive "yes" confirmation.
4. Decompresses on the host and pipes through `docker compose exec -T postgres pg_restore`.
5. Uses `--clean --if-exists` (drops existing objects before restoring), `--no-owner`, `--no-privileges`, `--exit-on-error`.

### After restore

```bash
# Verify migration state
docker compose -f /home/ec2-user/dzerothAI/docker-compose.prod.yml exec postgres \
  psql -U dzeroth -d dzeroth \
  -c "SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1;"
# Expected: latest version, dirty = f

# Restart the API
IMAGE_TAG=<tag> docker compose -f /home/ec2-user/dzerothAI/docker-compose.prod.yml up -d api

# Verify readiness
docker exec $(docker compose -f /home/ec2-user/dzerothAI/docker-compose.prod.yml ps -q api) \
  wget -qO- http://localhost:9091/readyz
# Expected: {"status":"ready"}

# Verify health
curl -s http://localhost:8080/health
# Expected: {"status":"ok","db":"ok","redis":"ok",...}
```

---

## Staging restore drill

Perform this drill monthly to confirm backups are restorable.

1. Download a recent production backup to the staging host:
   ```bash
   aws s3 cp \
     s3://dzeroth-production-postgres-backups-2026/postgres/<latest>.dump.gz \
     /tmp/restore-drill.dump.gz
   ```

2. Verify gzip integrity:
   ```bash
   gzip -t /tmp/restore-drill.dump.gz && echo "PASS"
   ```

3. Stop the staging API:
   ```bash
   docker compose -f /home/ec2-user/dzerothAI/docker-compose.staging.yml stop api
   ```

4. Run the restore against the staging database:
   ```bash
   COMPOSE_FILE=/home/ec2-user/dzerothAI/docker-compose.staging.yml \
   sudo /home/ec2-user/dzerothAI/scripts/backup/pg_restore.sh --confirm \
     /tmp/restore-drill.dump.gz
   ```
   Note: You must also point `POSTGRES_DB`/`POSTGRES_USER` at the staging values
   if they differ from the defaults.

5. Restart the staging API:
   ```bash
   IMAGE_TAG=<tag> docker compose -f /home/ec2-user/dzerothAI/docker-compose.staging.yml up -d api
   ```

6. Verify health and a known data record.

7. Record the drill date, backup file used, and result.

---

## Failure handling

### Backup fails — partial .tmp file

The backup script writes to a `.tmp` file and registers an `EXIT` trap that
removes it on any error. No partial file survives a failed run.

### Backup fails — gzip or pg_restore integrity check

The script exits before the S3 upload and before the local retention step.
The corrupted backup is not uploaded to S3. Investigate the postgres container
logs for the root cause.

### Backup fails — S3 upload

The script exits after the upload error. The local file is preserved (the
local retention step runs only after successful upload + verification). The
backup remains locally available until the next successful run creates a new
file and local retention deletes older ones.

### S3 head-object verification fails

The upload may have partially completed. The script exits non-zero. The local
file is preserved. Investigate S3 connectivity and IAM role permissions.

### Backup fails — notification

The systemd service exit code is non-zero on failure. The journald log contains
`[ERROR]` entries. Configure Prometheus Alertmanager (via the existing
Prometheus setup in `monitoring/`) to alert on `dzeroth-backup.service` failures,
or use AWS CloudWatch to monitor S3 put operations.

---

## Test suite

```bash
bash /home/ec2-user/dzerothAI/scripts/backup/test_backup.sh
```

Tests shell syntax, argument validation, trap behavior, secrets file detection,
logging format, filename format, and the `--confirm` flag. Does not require
Docker, AWS, or a live database. See the script for details.

---

## Relationship to RELEASE_CHECKLIST.md

The release checklist item "PostgreSQL backup strategy confirmed" is satisfied
when:

- [ ] `scripts/backup/pg_backup.sh` has been run manually on the production host and produces a backup.
- [ ] The backup was uploaded to S3 and verified (`aws s3api head-object` returned 200).
- [ ] The systemd timer is installed and enabled (`systemctl status dzeroth-backup.timer`).
- [ ] S3 Lifecycle rule is configured on the bucket.
- [ ] `scripts/backup/pg_restore.sh` has been run on a staging database with a production backup (restore drill performed and recorded).
- [ ] `scripts/backup/test_backup.sh` passes on the production host.
