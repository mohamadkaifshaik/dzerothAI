# Dzeroth — Production Disaster Recovery Runbook

**Purpose:** Step-by-step procedure to rebuild the Dzeroth production
environment on a fresh EC2 instance after complete loss of the current host.

**Last verified:** 2026-09-21 (restore drill completed — database recovered
from S3 backup; 14 tables; schema_migrations v13, dirty=false).

---

## Scope

This runbook covers full recovery from scratch: a new EC2 instance with no
pre-installed software, no secrets, and no running containers.

**Prerequisites:**

- AWS account access with permission to launch EC2 instances, attach IAM
  roles, and associate Elastic IPs.
- Access to the GitHub repository `mohamadkaifshaik/dzerothAI`.
- Access to S3 bucket `dzeroth-production-postgres-backups-2026` (ap-south-2).
- The DNS for `dzeroth.com` is under your control.
- The production EC2 key pair is available for SSH.

**This runbook does NOT cover:**

- Routine deployments → `docs/DEPLOYMENT_TOPOLOGY.md`
- Backup configuration and S3/IAM setup → `docs/BACKUP_RECOVERY.md`
- Nginx and TLS setup details → `docs/NGINX_TLS.md`
- EC2 provisioning details → `docs/EC2_PROVISIONING.md`

---

## Current production architecture

```
Domain:        dzeroth.com
Elastic IP:    98.130.17.78
AWS Region:    ap-south-2 (Hyderabad)
EC2:           t3.small, Amazon Linux 2023 x86_64
EC2 name:      dzeroth-production
Repo path:     /home/ec2-user/dzerothAI

Internet (HTTPS :443 / HTTP :80)
    │
    ▼
Nginx (system service on EC2 host)
    │  127.0.0.1:8080
    ▼
┌──────────── docker-compose.prod.yml (Docker bridge network) ──────────────┐
│  dzeroth-api  127.0.0.1:8080:8080  (loopback only — not public)           │
│  postgres     no host port          (Docker bridge network only)           │
│  redis        no host port          (Docker bridge network only)           │
└───────────────────────────────────────────────────────────────────────────┘
    │
    ▼
S3: dzeroth-production-postgres-backups-2026 (daily pg_dump at 02:00 UTC)
IAM role: DzerothProductionBackupRole (IMDS — no stored credentials)
```

---

## Estimated recovery time

| Phase | Estimated time |
|-------|---------------|
| EC2 provisioning + Docker install | 15 minutes |
| Repository clone + image build | 10 minutes |
| Secrets recreation + stack startup | 5 minutes |
| PostgreSQL restore (depends on backup size) | 5–30 minutes |
| Nginx install + TLS certificate | 10 minutes |
| Elastic IP reassignment + verification | 5 minutes |
| **Total** | **50–75 minutes** |

---

## Step 1 — Launch a replacement EC2 instance

See `docs/EC2_PROVISIONING.md` for the full provisioning procedure.

Required specification:

| Parameter | Value |
|-----------|-------|
| AMI | Amazon Linux 2023 (x86_64) |
| Instance type | t3.small |
| Storage | 20 GiB gp3 |
| Region | ap-south-2 |
| IAM role | DzerothProductionBackupRole |
| Security group | inbound: 22 (restricted IPs only), 80 (0.0.0.0/0), 443 (0.0.0.0/0) |
| Public IP | **Disable** — Elastic IP will be associated in a later step |

Do not associate the Elastic IP yet. Keep the old instance running if it is
still accessible — reassign the IP only after the new instance is confirmed
healthy.

---

## Step 2 — Install Docker and Docker Compose v2

See `docs/EC2_PROVISIONING.md` for the detailed procedure. Quick reference:

```bash
# Docker Engine
sudo yum install -y docker
sudo systemctl enable --now docker
sudo usermod -aG docker ec2-user
newgrp docker

# Docker Compose v2 plugin
DOCKER_CONFIG=${DOCKER_CONFIG:-$HOME/.docker}
mkdir -p "$DOCKER_CONFIG/cli-plugins"
curl -SL https://github.com/docker/compose/releases/latest/download/docker-compose-linux-x86_64 \
  -o "$DOCKER_CONFIG/cli-plugins/docker-compose"
chmod +x "$DOCKER_CONFIG/cli-plugins/docker-compose"

# System-wide symlink (required for root/systemd backup service)
sudo mkdir -p /usr/local/lib/docker/cli-plugins
sudo ln -sf "$DOCKER_CONFIG/cli-plugins/docker-compose" \
  /usr/local/lib/docker/cli-plugins/docker-compose

# Verify
docker compose version
sudo docker compose version
```

---

## Step 3 — Clone the repository

```bash
cd /home/ec2-user
git clone https://github.com/mohamadkaifshaik/dzerothAI.git dzerothAI
```

> **The repository must be cloned to `/home/ec2-user/dzerothAI` exactly.**
> The systemd unit files and backup scripts reference this path directly.
> Cloning to any other path requires updating both systemd units before the
> backup timer will work.

Make backup scripts executable:

```bash
chmod +x /home/ec2-user/dzerothAI/scripts/backup/pg_backup.sh
chmod +x /home/ec2-user/dzerothAI/scripts/backup/pg_restore.sh
```

---

## Step 4 — Recreate production secrets

```bash
mkdir -p /home/ec2-user/dzerothAI/deploy/secrets
chmod 700 /home/ec2-user/dzerothAI/deploy/secrets

cp /home/ec2-user/dzerothAI/deploy/env.production.example \
   /home/ec2-user/dzerothAI/deploy/secrets/env.production
```

Edit `deploy/secrets/env.production`. Generate new values for each secret:

```bash
openssl rand -base64 32   # POSTGRES_PASSWORD
openssl rand -base64 32   # REDIS_PASSWORD
openssl rand -base64 48   # JWT_SECRET (minimum 32 bytes)
```

**Mandatory values to set:**

| Variable | Value |
|----------|-------|
| `POSTGRES_PASSWORD` | Generate with `openssl rand -base64 32` |
| `REDIS_PASSWORD` | Generate with `openssl rand -base64 32` |
| `JWT_SECRET` | Generate with `openssl rand -base64 48` |
| `CORS_ALLOWED_ORIGINS` | `https://dzeroth.com` |
| `POSTGRES_SSL_MODE` | **`disable`** — see critical note below |

---

> ### CRITICAL: POSTGRES_SSL_MODE must be `disable`
>
> The template (`deploy/env.production.example`) defaults to
> `POSTGRES_SSL_MODE=require`. This default is correct for **managed
> PostgreSQL** (AWS RDS, Cloud SQL, etc.).
>
> The bundled Compose PostgreSQL service (`postgis/postgis:15-3.3`) starts
> with `ssl=off`. It has no TLS configured. Connecting with `require` will
> cause the API to fail at startup with an SSL connection error.
>
> **You must explicitly set:**
> ```
> POSTGRES_SSL_MODE=disable
> ```
> in `deploy/secrets/env.production` when using the bundled Compose service.

---

Verify no `REQUIRED_` placeholder remains before proceeding:

```bash
grep 'REQUIRED_' /home/ec2-user/dzerothAI/deploy/secrets/env.production \
  && echo "INCOMPLETE — fix all placeholders before deploying" \
  || echo "OK — no REQUIRED_ placeholders remain"
```

> **Security rule:** Never commit `deploy/secrets/env.production` to git.
> Never paste secret values into documentation, issue trackers, or chat.
> Rotating `JWT_SECRET` invalidates all active user sessions — all users
> will be required to re-authenticate.

---

## Step 5 — Build the production Docker image

There is no image registry. The image must be built locally on the new instance.

```bash
cd /home/ec2-user/dzerothAI

export VERSION=$(git describe --tags --always)
export COMMIT=$(git rev-parse --short HEAD)
export BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
export IMAGE_TAG="${VERSION}"

docker build -f apps/backend/Dockerfile apps/backend/ \
  --build-arg VERSION="${VERSION}" \
  --build-arg COMMIT="${COMMIT}" \
  --build-arg BUILD_TIME="${BUILD_TIME}" \
  -t "dzeroth-api:${IMAGE_TAG}"
```

The multi-stage Dockerfile (`golang:1.26-alpine` → `alpine:3.20`) includes all
build tooling. No Go toolchain needs to be installed on the host.

---

## Step 6 — Start the application stack

```bash
cd /home/ec2-user/dzerothAI

IMAGE_TAG="${IMAGE_TAG}" docker compose -f docker-compose.prod.yml up -d

# Tail logs until "http server listening" appears
docker compose -f docker-compose.prod.yml logs api --follow
```

At this point the API is listening on `127.0.0.1:8080` (loopback only — not
yet publicly reachable). Internet traffic reaches the application only after
Nginx and DNS are configured in Steps 8–9.

Verify all services are healthy:

```bash
docker compose -f docker-compose.prod.yml ps
```

Expected: `api`, `postgres`, and `redis` all `Up` and `(healthy)`.

---

## Step 7 — Restore PostgreSQL from backup

> ### Production restore vs isolated drill
>
> **This step (Step 7) is a PRODUCTION RESTORE.** It replaces all data in the
> live production database with backup contents. This is the correct action
> during disaster recovery.
>
> When testing that backups are restorable — without affecting production data —
> use an isolated database instead. See `docs/BACKUP_RECOVERY.md` for the
> isolated drill procedure. The drill performed on 2026-09-21 used database
> `dzeroth_restore_drill` and confirmed: 14 public tables, schema_migrations
> version 13, dirty=false. The production database was not modified.

### 7a — Stop the API

```bash
docker compose -f /home/ec2-user/dzerothAI/docker-compose.prod.yml stop api
```

### 7b — Identify the most recent backup

```bash
aws s3 ls s3://dzeroth-production-postgres-backups-2026/postgres/ \
  --recursive | sort -k1,2r | head -5
```

### 7c — Create the local backup directory and download

```bash
sudo mkdir -p /var/backups/dzeroth
sudo chmod 700 /var/backups/dzeroth

# Replace <timestamp> with the filename from the S3 listing
aws s3 cp \
  s3://dzeroth-production-postgres-backups-2026/postgres/dzeroth_<timestamp>.dump.gz \
  /var/backups/dzeroth/dzeroth_<timestamp>.dump.gz
```

### 7d — Run the restore

```bash
sudo /home/ec2-user/dzerothAI/scripts/backup/pg_restore.sh \
  /var/backups/dzeroth/dzeroth_<timestamp>.dump.gz
```

Type `yes` when prompted. The restore script will:
1. Verify gzip integrity of the backup file.
2. Warn about the destructive operation and require confirmation.
3. Restore with `--clean --if-exists --no-owner --no-privileges --exit-on-error`.

### 7e — Verify the restore

```bash
docker compose -f /home/ec2-user/dzerothAI/docker-compose.prod.yml exec postgres \
  psql -U dzeroth -d dzeroth \
  -c "SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1;"
# Expected: version = 13, dirty = f
```

### 7f — Restart the API

```bash
IMAGE_TAG="${IMAGE_TAG}" \
  docker compose -f /home/ec2-user/dzerothAI/docker-compose.prod.yml up -d api

docker compose -f /home/ec2-user/dzerothAI/docker-compose.prod.yml logs api --follow
```

---

## Step 8 — Install Nginx and obtain TLS certificate

See `docs/NGINX_TLS.md` for the full procedure.

> **Important:** The DNS A record for `dzeroth.com` must point to this
> instance's IP before running certbot. Reassign the Elastic IP (Step 9)
> before running certbot. The IP is `98.130.17.78`.

Quick reference:

```bash
# Install Nginx
sudo yum install -y nginx
sudo systemctl enable nginx

# Install certbot
sudo yum install -y python3-certbot-nginx

# Minimal nginx config before certbot (upstream uses 127.0.0.1 — not 'api')
# Edit /etc/nginx/conf.d/dzeroth.conf — see docs/NGINX_TLS.md for full config

sudo nginx -t
sudo systemctl start nginx

# Obtain certificate (runs after DNS is pointing to this instance)
sudo certbot --nginx -d dzeroth.com

sudo nginx -t && sudo systemctl reload nginx
```

---

## Step 9 — Reassign Elastic IP and verify DNS

### Reassign Elastic IP 98.130.17.78

In the AWS console (region `ap-south-2`):

1. Navigate to **EC2 → Elastic IPs**.
2. Select the Elastic IP `98.130.17.78`.
3. If associated with the old instance: **Actions → Disassociate Elastic IP**.
4. **Actions → Associate Elastic IP address** → select the new instance.

Or via AWS CLI:

```bash
# Find the allocation ID
ALLOC_ID=$(aws ec2 describe-addresses \
  --region ap-south-2 \
  --filters "Name=public-ip,Values=98.130.17.78" \
  --query 'Addresses[0].AllocationId' \
  --output text)

# Disassociate from old instance if needed
aws ec2 disassociate-address \
  --region ap-south-2 \
  --association-id <assoc-id>

# Associate with new instance
aws ec2 associate-address \
  --region ap-south-2 \
  --instance-id <new-instance-id> \
  --allocation-id "${ALLOC_ID}"
```

### DNS verification

`dzeroth.com` has an A record pointing to `98.130.17.78`. After reassigning
the Elastic IP to the new instance, the DNS record does not change.

```bash
dig +short dzeroth.com A
# Expected: 98.130.17.78
```

> Reassign the Elastic IP **before** running certbot. Let's Encrypt's HTTP-01
> challenge connects to port 80 on the domain's A record. If the IP is not
> yet associated, the challenge will fail.

---

## Step 10 — Install backup systemd units

```bash
sudo cp /home/ec2-user/dzerothAI/deploy/systemd/dzeroth-backup.service \
  /etc/systemd/system/
sudo cp /home/ec2-user/dzerothAI/deploy/systemd/dzeroth-backup.timer \
  /etc/systemd/system/

sudo systemctl daemon-reload
sudo systemctl enable --now dzeroth-backup.timer
```

Verify the timer is scheduled:

```bash
sudo systemctl status dzeroth-backup.timer
# Expected: Active: active (waiting), next trigger shown
```

Trigger a manual backup run to confirm AWS credentials and S3 access work:

```bash
sudo systemctl start dzeroth-backup.service
sudo journalctl -u dzeroth-backup.service -f
# Expected final line: [INFO] Backup successful: ... -> s3://dzeroth-production-...
```

---

## Step 11 — Full health verification

```bash
# 1. API health via localhost
curl -s http://localhost:8080/health
# Expected: {"status":"ok","db":"ok","redis":"ok",...}

# 2. API readiness via admin port (not publicly exposed — use docker exec)
docker exec \
  $(docker compose -f /home/ec2-user/dzerothAI/docker-compose.prod.yml ps -q api) \
  wget -qO- http://localhost:9091/readyz
# Expected: {"status":"ready"}

# 3. End-to-end TLS via public domain
curl -s https://dzeroth.com/health
# Expected: {"status":"ok","db":"ok","redis":"ok",...}

# 4. TLS certificate valid
echo | openssl s_client -connect dzeroth.com:443 -servername dzeroth.com 2>/dev/null \
  | openssl x509 -noout -dates
# Expected: notAfter is in the future

# 5. Backup timer active
sudo systemctl is-active dzeroth-backup.timer
# Expected: active

# 6. All containers healthy
docker compose -f /home/ec2-user/dzerothAI/docker-compose.prod.yml ps
# Expected: api, postgres, redis — all Up and (healthy)

# 7. Build identity
docker exec \
  $(docker compose -f /home/ec2-user/dzerothAI/docker-compose.prod.yml ps -q api) \
  wget -qO- http://localhost:9091/metrics 2>/dev/null | grep dzeroth_build_info
# Expected: dzeroth_build_info{commit="...",version="..."} 1
```

---

## Final recovery checklist

```
[ ] EC2 instance running: t3.small, Amazon Linux 2023, ap-south-2
[ ] Security group: 22 (restricted), 80 (open), 443 (open) — no other public ports
[ ] IAM role DzerothProductionBackupRole attached to instance
[ ] Docker Engine installed and running
[ ] Docker Compose v2 plugin installed
[ ] System-wide docker compose symlink in /usr/local/lib/docker/cli-plugins/
[ ] Repository cloned to /home/ec2-user/dzerothAI
[ ] deploy/secrets/env.production created — no REQUIRED_ placeholders remain
[ ] POSTGRES_SSL_MODE=disable set (bundled Compose postgres has no TLS)
[ ] CORS_ALLOWED_ORIGINS=https://dzeroth.com set
[ ] Docker image built and tagged
[ ] docker-compose.prod.yml stack running: api, postgres, redis all healthy
[ ] PostgreSQL restored from S3 backup
[ ] schema_migrations version 13, dirty=false verified
[ ] Elastic IP 98.130.17.78 associated with new instance
[ ] Nginx installed and running
[ ] TLS certificate obtained for dzeroth.com via certbot
[ ] HTTP redirects to HTTPS
[ ] curl https://dzeroth.com/health returns {"status":"ok","db":"ok","redis":"ok"}
[ ] Backup systemd units installed and enabled
[ ] Manual backup run succeeded and S3 upload verified
[ ] dzeroth-backup.timer is active
```

---

## Rollback and abort

If recovery must be aborted mid-procedure:

1. Stop all containers: `docker compose -f docker-compose.prod.yml down`
2. Leave the Elastic IP unassigned until the new instance is confirmed healthy.
3. If the old instance is still reachable, re-associate the Elastic IP with it
   to restore service. Traffic cut-over is instant (Elastic IP reassignment
   takes effect in seconds, with no DNS propagation wait).
4. Do not terminate the old instance until the new instance is confirmed
   healthy and a fresh backup has been verified.

---

## Related documents

- `docs/EC2_PROVISIONING.md` — EC2 instance provisioning (Step 1–2 details)
- `docs/NGINX_TLS.md` — Nginx installation and TLS certificate setup (Step 8)
- `docs/BACKUP_RECOVERY.md` — Backup/restore procedure, IAM role, S3 bucket
- `docs/DEPLOYMENT_TOPOLOGY.md` — Normal deployment and migration runbook
- `deploy/systemd/README.md` — Systemd unit installation and management
