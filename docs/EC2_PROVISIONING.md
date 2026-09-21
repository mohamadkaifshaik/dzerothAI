# Dzeroth — EC2 Instance Provisioning

This document covers provisioning a fresh Amazon Linux 2023 EC2 instance for
Dzeroth production. Follow this document as Step 1 and Step 2 of
`docs/DISASTER_RECOVERY.md`.

---

## Verified production specification

| Parameter | Value |
|-----------|-------|
| AMI | Amazon Linux 2023 (x86_64) |
| Instance type | t3.small |
| vCPU / RAM | 2 vCPU / 2 GiB |
| Storage | 20 GiB gp3 |
| Region | ap-south-2 (Hyderabad) |
| OS user | `ec2-user` |
| Repository path | `/home/ec2-user/dzerothAI` |
| Elastic IP | 98.130.17.78 (production only) |
| IAM role | DzerothProductionBackupRole |

---

## Step 1 — Launch the EC2 instance

### Via AWS console

1. Open **EC2 → Launch instances** in region **ap-south-2**.
2. **Name:** `dzeroth-production`
3. **AMI:** Search for and select **Amazon Linux 2023 AMI (x86_64, HVM, gp2)**.
4. **Instance type:** `t3.small`
5. **Key pair:** Select the existing production key pair. If lost, create a new
   one and update all operational runbooks.
6. **Network settings:**
   - VPC: default or the production VPC.
   - Subnet: any public subnet in ap-south-2.
   - Auto-assign public IP: **Disable** — Elastic IP is assigned separately.
   - Security group: create or select — see Step 2.
7. **Storage:** 20 GiB, volume type gp3.
8. **Advanced details → IAM instance profile:** `DzerothProductionBackupRole`
9. **Advanced details → Metadata version:** Select **V2 only (token required)**
   This enforces IMDSv2 and prevents credential theft via SSRF attacks that use
   simple GET requests. See Step 3.5 for details.
10. Click **Launch instance**.

### Via AWS CLI

Find the current Amazon Linux 2023 AMI ID:

```bash
aws ec2 describe-images \
  --region ap-south-2 \
  --owners amazon \
  --filters "Name=name,Values=al2023-ami-*-x86_64" \
            "Name=state,Values=available" \
  --query 'sort_by(Images, &CreationDate)[-1].ImageId' \
  --output text
```

Launch the instance:

```bash
aws ec2 run-instances \
  --region ap-south-2 \
  --image-id <ami-id-from-above> \
  --instance-type t3.small \
  --key-name <key-pair-name> \
  --security-group-ids <sg-id> \
  --iam-instance-profile Name=DzerothProductionBackupRole \
  --metadata-options HttpTokens=required,HttpEndpoint=enabled \
  --block-device-mappings \
    '[{"DeviceName":"/dev/xvda","Ebs":{"VolumeSize":20,"VolumeType":"gp3","DeleteOnTermination":true}}]' \
  --no-associate-public-ip-address \
  --tag-specifications \
    'ResourceType=instance,Tags=[{Key=Name,Value=dzeroth-production}]'
```

---

## Step 2 — Security group

Create (or verify) a security group with these inbound rules.

| Type | Protocol | Port | Source | Purpose |
|------|----------|------|--------|---------|
| SSH | TCP | 22 | **Operator IP(s) only** | Administrative access |
| HTTP | TCP | 80 | 0.0.0.0/0, ::/0 | HTTP → HTTPS redirect via Nginx |
| HTTPS | TCP | 443 | 0.0.0.0/0, ::/0 | Production HTTPS traffic |

**All other ports must remain closed.** In particular:

- Port 5432 (PostgreSQL) — no host binding; Docker bridge network only.
- Port 6379 (Redis) — no host binding; Docker bridge network only.
- Port 8080 (API) — bound to `127.0.0.1` loopback; not reachable from outside.
- Port 9091 (admin/metrics) — not published; access via `docker exec` only.

SSH source must be restricted to known IP addresses or CIDR ranges.
**Never open SSH to 0.0.0.0/0 in production.**

Create via AWS CLI:

```bash
# Create the security group
SG_ID=$(aws ec2 create-security-group \
  --region ap-south-2 \
  --group-name dzeroth-production-sg \
  --description "Dzeroth production EC2 security group" \
  --query 'GroupId' --output text)

# SSH — replace <your-ip> with your actual IP
aws ec2 authorize-security-group-ingress \
  --region ap-south-2 --group-id "${SG_ID}" \
  --protocol tcp --port 22 --cidr <your-ip>/32

# HTTP
aws ec2 authorize-security-group-ingress \
  --region ap-south-2 --group-id "${SG_ID}" \
  --protocol tcp --port 80 --cidr 0.0.0.0/0

# HTTPS
aws ec2 authorize-security-group-ingress \
  --region ap-south-2 --group-id "${SG_ID}" \
  --protocol tcp --port 443 --cidr 0.0.0.0/0

echo "Security group ID: ${SG_ID}"
```

---

## Step 3 — Elastic IP association

The production Elastic IP `98.130.17.78` ensures the IP address remains stable
across instance replacements. The `dzeroth.com` A record points to this IP and
does not need to change when the instance is replaced.

```bash
# Find the allocation ID for 98.130.17.78
ALLOC_ID=$(aws ec2 describe-addresses \
  --region ap-south-2 \
  --filters "Name=public-ip,Values=98.130.17.78" \
  --query 'Addresses[0].AllocationId' \
  --output text)

# If it is currently associated with the old instance, disassociate first:
ASSOC_ID=$(aws ec2 describe-addresses \
  --region ap-south-2 \
  --filters "Name=public-ip,Values=98.130.17.78" \
  --query 'Addresses[0].AssociationId' \
  --output text)

# Only disassociate if ASSOC_ID is not empty/None
aws ec2 disassociate-address \
  --region ap-south-2 \
  --association-id "${ASSOC_ID}"

# Associate with the new instance
aws ec2 associate-address \
  --region ap-south-2 \
  --instance-id <new-instance-id> \
  --allocation-id "${ALLOC_ID}"
```

Verify:

```bash
aws ec2 describe-addresses \
  --region ap-south-2 \
  --filters "Name=public-ip,Values=98.130.17.78" \
  --query 'Addresses[0].{IP:PublicIp,Instance:InstanceId,State:AssociationId}'
```

---

## Step 3.5 — Enforce IMDSv2

IMDSv2 (Instance Metadata Service v2) requires a session-oriented token for
all IMDS requests. This prevents credential theft via SSRF attacks — a
compromised application that makes GET requests to `169.254.169.254` cannot
retrieve IAM credentials without first completing a PUT-based token exchange.

### Apply to an existing instance

If the instance was launched without `HttpTokens=required`, apply IMDSv2 to
the running instance:

```bash
aws ec2 modify-instance-metadata-options \
  --region ap-south-2 \
  --instance-id <instance-id> \
  --http-tokens required \
  --http-endpoint enabled
```

This takes effect immediately. No reboot required.

> **Do NOT set `--http-endpoint disabled`** — that disables IMDS entirely
> and breaks IAM role credential delivery, causing the backup script to fail.

### Verify IMDSv2 is enforced

```bash
aws ec2 describe-instances \
  --region ap-south-2 \
  --instance-ids <instance-id> \
  --query 'Reservations[0].Instances[0].MetadataOptions' \
  --output json
# Expected:
# {
#   "State": "applied",
#   "HttpTokens": "required",
#   "HttpEndpoint": "enabled",
#   ...
# }
```

Confirm that IMDS responds to an IMDSv2 token-authenticated request (run
from inside the instance via SSH):

```bash
# Obtain a session token (valid for 60 seconds)
TOKEN=$(curl -s -X PUT "http://169.254.169.254/latest/api/token" \
  -H "X-aws-ec2-metadata-token-ttl-seconds: 60")

# Use the token to retrieve the IAM role name
curl -s -H "X-aws-ec2-metadata-token: ${TOKEN}" \
  "http://169.254.169.254/latest/meta-data/iam/security-credentials/"
# Expected: DzerothProductionBackupRole
```

An IMDSv1-style GET (no token header) must be rejected:

```bash
curl -s -o /dev/null -w "%{http_code}" \
  "http://169.254.169.254/latest/meta-data/iam/security-credentials/"
# Expected: 401 (rejected — IMDSv2 is enforced)
```

### New instances

The `run-instances` command in Step 1 already includes
`--metadata-options HttpTokens=required,HttpEndpoint=enabled`.
No additional step is required when provisioning a new instance.

---

## Step 4 — IAM role: DzerothProductionBackupRole

This IAM role grants the EC2 instance permission to upload and verify backups
in S3 using the instance metadata service (IMDS). No stored AWS credentials
are required on the host.

### Verify the role exists

```bash
aws iam get-role --role-name DzerothProductionBackupRole \
  --query 'Role.RoleName' --output text
```

### Create the role (if it does not exist)

```bash
# 1. Create the role with an EC2 trust policy
aws iam create-role \
  --role-name DzerothProductionBackupRole \
  --description "Allows EC2 to upload PostgreSQL backups to S3" \
  --assume-role-policy-document '{
    "Version": "2012-10-17",
    "Statement": [{
      "Effect": "Allow",
      "Principal": {"Service": "ec2.amazonaws.com"},
      "Action": "sts:AssumeRole"
    }]
  }'

# 2. Attach the least-privilege inline policy
aws iam put-role-policy \
  --role-name DzerothProductionBackupRole \
  --policy-name DzerothS3BackupPolicy \
  --policy-document '{
    "Version": "2012-10-17",
    "Statement": [{
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
    }]
  }'

# 3. Create the instance profile and attach the role
aws iam create-instance-profile \
  --instance-profile-name DzerothProductionBackupRole

aws iam add-role-to-instance-profile \
  --instance-profile-name DzerothProductionBackupRole \
  --role-name DzerothProductionBackupRole
```

`s3:DeleteObject` is intentionally omitted. S3 object expiry is managed by an
S3 Lifecycle rule on the bucket — see `docs/BACKUP_RECOVERY.md`.

---

## Step 5 — S3 backup bucket

The S3 bucket `dzeroth-production-postgres-backups-2026` must exist before the
first backup runs. See `docs/BACKUP_RECOVERY.md` for the full setup procedure
including the Lifecycle rule.

Quick reference:

```bash
# Create the bucket in ap-south-2
aws s3api create-bucket \
  --bucket dzeroth-production-postgres-backups-2026 \
  --region ap-south-2 \
  --create-bucket-configuration LocationConstraint=ap-south-2

# Block all public access
aws s3api put-public-access-block \
  --bucket dzeroth-production-postgres-backups-2026 \
  --public-access-block-configuration \
    "BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true"

# Enable versioning
aws s3api put-bucket-versioning \
  --bucket dzeroth-production-postgres-backups-2026 \
  --versioning-configuration Status=Enabled

# Verify
aws s3api get-bucket-location \
  --bucket dzeroth-production-postgres-backups-2026
# Expected: ap-south-2
```

---

## Step 6 — Docker Engine installation

```bash
sudo yum install -y docker
sudo systemctl enable --now docker

# Allow ec2-user to run docker without sudo
sudo usermod -aG docker ec2-user
newgrp docker   # applies group change to current session

# Verify
docker --version
docker run --rm hello-world
```

---

## Step 7 — Docker Compose v2 plugin

Docker Compose v2 is a CLI plugin. Install it under `ec2-user` and create a
system-wide symlink so that root (used by the backup systemd service) can also
find it.

```bash
# Install for ec2-user
DOCKER_CONFIG=${DOCKER_CONFIG:-$HOME/.docker}
mkdir -p "$DOCKER_CONFIG/cli-plugins"

curl -SL \
  https://github.com/docker/compose/releases/latest/download/docker-compose-linux-x86_64 \
  -o "$DOCKER_CONFIG/cli-plugins/docker-compose"
chmod +x "$DOCKER_CONFIG/cli-plugins/docker-compose"

# System-wide symlink — required for the backup systemd service (runs as root)
sudo mkdir -p /usr/local/lib/docker/cli-plugins
sudo ln -sf "$DOCKER_CONFIG/cli-plugins/docker-compose" \
  /usr/local/lib/docker/cli-plugins/docker-compose

# Verify both users can use docker compose
docker compose version
sudo docker compose version
```

The backup script (`scripts/backup/pg_backup.sh`) calls `find_docker_compose()`
which checks system-wide plugin paths before falling back to the user-space
path. The symlink above ensures it works when the script runs under systemd
as root.

---

## Step 8 — Add swap (recommended)

`t3.small` has 2 GiB RAM. Under normal operating conditions the Go API,
PostgreSQL, and Redis fit within this budget. During a Docker image build
(multi-stage, Go compiler), memory usage spikes. A 1 GiB swap file provides
headroom without additional cost.

```bash
sudo dd if=/dev/zero of=/swapfile bs=1M count=1024 status=progress
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile

# Persist across reboots
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab

# Verify
free -h
```

---

## Step 9 — Clone the repository

```bash
cd /home/ec2-user
git clone https://github.com/mohamadkaifshaik/dzerothAI.git dzerothAI
```

> **The repository must be at `/home/ec2-user/dzerothAI`.**
> This path is hardcoded in both systemd unit files
> (`deploy/systemd/dzeroth-backup.service`) and is the working directory
> used by the backup script. Cloning elsewhere requires updating the
> `WorkingDirectory=` and `ExecStart=` lines in the service unit before
> the backup timer will work correctly.

---

## Provisioning verification checklist

```bash
# 1. Instance is running
aws ec2 describe-instances \
  --region ap-south-2 \
  --filters "Name=tag:Name,Values=dzeroth-production" \
  --query 'Reservations[0].Instances[0].State.Name' \
  --output text
# Expected: running

# 2. Elastic IP associated
dig +short dzeroth.com A
# Expected: 98.130.17.78

# 3. Docker running
sudo systemctl is-active docker
# Expected: active

# 4. Docker Compose accessible from both ec2-user and root
docker compose version
sudo docker compose version

# 5. Repository present
ls /home/ec2-user/dzerothAI/docker-compose.prod.yml

# 6. IMDSv2 is enforced (run from the EC2 instance)
#    Step 1: Obtain a session token
TOKEN=$(curl -s -X PUT "http://169.254.169.254/latest/api/token" \
  -H "X-aws-ec2-metadata-token-ttl-seconds: 60")
#    Step 2: Retrieve the IAM role name using the token
curl -s -H "X-aws-ec2-metadata-token: ${TOKEN}" \
  "http://169.254.169.254/latest/meta-data/iam/security-credentials/"
# Expected: DzerothProductionBackupRole
#
#    Step 3: Confirm IMDSv1 is rejected (no token header)
curl -s -o /dev/null -w "%{http_code}" \
  "http://169.254.169.254/latest/meta-data/iam/security-credentials/"
# Expected: 401

# 7. S3 bucket accessible
aws s3 ls s3://dzeroth-production-postgres-backups-2026/ --region ap-south-2
# Expected: no error; folder listing (postgres/)
```

---

## Related documents

- `docs/DISASTER_RECOVERY.md` — Full DR runbook (uses this document as Steps 1–2)
- `docs/NGINX_TLS.md` — Nginx and TLS setup (after EC2 provisioning)
- `docs/BACKUP_RECOVERY.md` — S3 bucket lifecycle rule and restore procedure
