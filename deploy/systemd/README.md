# Dzeroth Backup — systemd Timer Installation

This directory contains the systemd unit files for the daily Dzeroth
PostgreSQL backup job.

## Why systemd timer instead of user crontab

| Concern | User crontab | systemd timer |
|---|---|---|
| Status visibility | No built-in status | `systemctl status dzeroth-backup.timer` |
| Logging | Requires manual log rotation | Journald integration, queryable with `journalctl` |
| Session persistence | Depends on user session | System service, survives all sessions |
| Missed-run recovery | Silent miss | `Persistent=true` catches missed runs after downtime |
| Dependency ordering | None | `After=docker.service Requires=docker.service` |
| Failure alerting | Requires external tooling | Exit code captured by journald; can feed Prometheus alertmanager |

## Files

| File | Purpose |
|---|---|
| `dzeroth-backup.service` | Oneshot service that runs `pg_backup.sh` |
| `dzeroth-backup.timer` | Daily trigger at 02:00 UTC with `Persistent=true` |

## Installation on the EC2 instance

```bash
# Ensure the repository is at /home/ec2-user/dzerothAI
ls /home/ec2-user/dzerothAI/scripts/backup/pg_backup.sh

# Make the backup script executable
chmod +x /home/ec2-user/dzerothAI/scripts/backup/pg_backup.sh

# Copy unit files to systemd
sudo cp /home/ec2-user/dzerothAI/deploy/systemd/dzeroth-backup.service /etc/systemd/system/
sudo cp /home/ec2-user/dzerothAI/deploy/systemd/dzeroth-backup.timer   /etc/systemd/system/

# Reload systemd unit definitions
sudo systemctl daemon-reload

# Enable and start the timer (starts on boot and fires at the next scheduled time)
sudo systemctl enable --now dzeroth-backup.timer
```

## Verification

```bash
# Check the timer is active and shows the next trigger time
sudo systemctl status dzeroth-backup.timer

# List all timers and confirm dzeroth-backup.timer appears
sudo systemctl list-timers dzeroth-backup.timer

# Run a manual backup immediately (useful for first-run verification)
sudo systemctl start dzeroth-backup.service

# Follow the backup logs in real time
sudo journalctl -u dzeroth-backup.service -f

# View all backup logs (most recent first)
sudo journalctl -u dzeroth-backup.service -r

# Check exit status of the last run
sudo systemctl status dzeroth-backup.service
```

## Updating after a repository pull

If the unit files change (e.g. after a `git pull`):

```bash
sudo cp /home/ec2-user/dzerothAI/deploy/systemd/dzeroth-backup.service /etc/systemd/system/
sudo cp /home/ec2-user/dzerothAI/deploy/systemd/dzeroth-backup.timer   /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl restart dzeroth-backup.timer
```

## Disabling the timer

```bash
sudo systemctl disable --now dzeroth-backup.timer
```

## Notes

- The service runs as `root` because `docker compose exec` requires access to
  the Docker daemon socket (`/var/run/docker.sock`).
- No AWS credentials are configured in the service file. The EC2 IAM role
  (`DzerothProductionBackupRole`) provides them automatically via the instance
  metadata service (IMDS). The AWS CLI on the host picks them up without any
  additional configuration.
- The database password is never present on the host. `pg_dump` reads
  `PGPASSWORD` from inside the `postgres` container's environment, where it
  was injected at container startup via the Compose `env_file`.
