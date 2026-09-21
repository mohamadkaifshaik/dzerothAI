# Dzeroth — Nginx and TLS Setup

This document covers installing Nginx as a system service on the production
EC2 instance, configuring it to reverse-proxy the Dzeroth API, and obtaining
a TLS certificate via Let's Encrypt and certbot.

This is Step 8 of `docs/DISASTER_RECOVERY.md`.

**Production host:** EC2 `dzeroth-production`, ap-south-2
**Domain:** `dzeroth.com`
**Elastic IP:** `98.130.17.78`

---

## Prerequisites

Complete these before running this procedure:

- EC2 instance is running and the Elastic IP `98.130.17.78` is associated
  with it (see `docs/EC2_PROVISIONING.md`).
- The `dzeroth.com` A record resolves to `98.130.17.78` — certbot's HTTP-01
  challenge requires this before certificate issuance.
- Security group inbound: port 80 and 443 open to 0.0.0.0/0.
- The Dzeroth API container is running and healthy on `127.0.0.1:8080`.

Verify before proceeding:

```bash
# DNS is correct
dig +short dzeroth.com A
# Must return: 98.130.17.78

# API is up locally
curl -s http://127.0.0.1:8080/health
# Expected: {"status":"ok","db":"ok","redis":"ok",...}
```

---

## Step 1 — Install Nginx

```bash
sudo yum install -y nginx
sudo systemctl enable nginx
```

---

## Step 2 — Install certbot

```bash
# Recommended: Amazon Linux 2023 package
sudo yum install -y python3-certbot-nginx
```

If `python3-certbot-nginx` is not available in the Amazon Linux 2023 package
repository, use the snap installation:

```bash
sudo yum install -y snapd
sudo systemctl enable --now snapd.socket
sudo snap install --classic certbot
sudo ln -sf /snap/bin/certbot /usr/bin/certbot
```

---

## Step 3 — Create the initial Nginx configuration

> **Upstream address note:**
>
> The reference file `nginx/nginx.prod.conf` uses `server api:8080` in the
> upstream block. The hostname `api` is the Docker Compose service name,
> which is only resolvable within the Docker bridge network.
>
> Nginx running as a system service (not a Docker container) cannot resolve
> `api`. The upstream must use the loopback address `127.0.0.1:8080`.

Create `/etc/nginx/conf.d/dzeroth.conf` with a minimal configuration for the
certbot HTTP-01 challenge:

```bash
sudo tee /etc/nginx/conf.d/dzeroth.conf > /dev/null <<'EOF'
upstream dzeroth_api {
    server 127.0.0.1:8080;
    keepalive 32;
}

server {
    listen 80;
    server_name dzeroth.com;
    # certbot will add the ACME challenge location and HTTPS redirect here.
}
EOF
```

Test and start:

```bash
sudo nginx -t
sudo systemctl start nginx
```

Verify Nginx is accepting connections on port 80:

```bash
curl -s -o /dev/null -w "%{http_code}" http://dzeroth.com/
# Expected: 200 or 301 (not a connection error)
```

---

## Step 4 — Obtain the TLS certificate

```bash
sudo certbot --nginx -d dzeroth.com
```

When prompted:

- Enter an email address for expiry notifications.
- Agree to the Let's Encrypt Terms of Service.
- Select **option 2 — Redirect HTTP traffic to HTTPS** (recommended).

certbot will:
1. Complete the HTTP-01 ACME challenge to verify domain ownership.
2. Download a certificate from Let's Encrypt.
3. Update `/etc/nginx/conf.d/dzeroth.conf` with the TLS configuration and
   the HTTP → HTTPS redirect.

Certificates are stored at:
- Certificate chain: `/etc/letsencrypt/live/dzeroth.com/fullchain.pem`
- Private key: `/etc/letsencrypt/live/dzeroth.com/privkey.pem`

---

## Step 5 — Apply the production Nginx configuration

After certbot runs, replace the auto-generated TLS config with the hardened
production configuration. Edit `/etc/nginx/conf.d/dzeroth.conf`:

```bash
sudo tee /etc/nginx/conf.d/dzeroth.conf > /dev/null <<'EOF'
upstream dzeroth_api {
    server 127.0.0.1:8080;
    keepalive 32;
}

server {
    listen 80;
    server_name dzeroth.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl;
    server_name dzeroth.com;

    ssl_certificate     /etc/letsencrypt/live/dzeroth.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/dzeroth.com/privkey.pem;

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 1d;
    ssl_session_tickets off;

    add_header Strict-Transport-Security "max-age=86400" always;

    proxy_connect_timeout 5s;
    proxy_send_timeout    35s;
    proxy_read_timeout    35s;
    send_timeout          35s;

    # Strip client-supplied forwarding headers and set the observed TCP source IP.
    # This is REQUIRED for per-IP rate limiting to be effective.
    # Do NOT use $http_x_real_ip here — that trusts the client-supplied header.
    proxy_set_header X-Real-IP        $remote_addr;
    proxy_set_header X-Forwarded-For  $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto https;
    proxy_set_header Host             $host;

    proxy_pass_request_headers on;
    proxy_http_version 1.1;
    proxy_set_header Connection "";

    location / {
        proxy_pass http://dzeroth_api;
    }
}
EOF
```

Test and reload:

```bash
sudo nginx -t && sudo systemctl reload nginx
```

---

## Step 6 — Verify certificate renewal

certbot installs an automatic renewal mechanism (systemd timer or cron).
Let's Encrypt certificates expire after 90 days; certbot renews when fewer
than 30 days remain.

Verify renewal is configured:

```bash
# Check for certbot systemd timer
sudo systemctl list-timers certbot* snap.certbot* 2>/dev/null

# Test dry-run renewal
sudo certbot renew --dry-run
```

---

## Validation

Run after completing all steps:

```bash
# 1. Nginx configuration is valid
sudo nginx -t

# 2. Nginx is running
sudo systemctl is-active nginx

# 3. HTTP redirects to HTTPS
curl -sI http://dzeroth.com/ | grep -i "^location:"
# Expected: Location: https://dzeroth.com/

# 4. HTTPS health check (end-to-end)
curl -s https://dzeroth.com/health
# Expected: {"status":"ok","db":"ok","redis":"ok",...}

# 5. TLS certificate issuer and expiry
echo | openssl s_client -connect dzeroth.com:443 -servername dzeroth.com 2>/dev/null \
  | openssl x509 -noout -subject -issuer -dates
# Expected: issuer includes "Let's Encrypt", notAfter is in the future

# 6. HSTS header present
curl -sI https://dzeroth.com/health | grep -i strict-transport
# Expected: strict-transport-security: max-age=86400

# 7. Admin port is NOT reachable from the public internet
# (Should time out or refuse — 9091 is not in the security group)
curl -s --max-time 5 http://98.130.17.78:9091/metrics
# Expected: connection refused or timeout — never a response
```

---

## Troubleshooting

### certbot HTTP-01 challenge fails

**Cause:** The DNS A record for `dzeroth.com` does not point to this instance.

```bash
# Verify DNS resolves to this instance
dig +short dzeroth.com A
# Must return 98.130.17.78

# Verify port 80 is reachable from outside
curl -s -o /dev/null -w "%{http_code}" http://dzeroth.com/
# Must not time out (a 404 is acceptable — certbot manages the challenge path)
```

If the Elastic IP is not yet associated, associate it first (see
`docs/EC2_PROVISIONING.md` Step 3), then retry certbot.

### Nginx cannot reach the API

**Cause:** The API container is not running, or its port is not 127.0.0.1:8080.

```bash
# Verify the API is listening on the loopback
ss -tlnp | grep 8080
# Expected: LISTEN ... 127.0.0.1:8080

# Verify it responds
curl -s http://127.0.0.1:8080/health
# Expected: {"status":"ok",...}
```

If the API is not running, start the Compose stack first:

```bash
cd /home/ec2-user/dzerothAI
IMAGE_TAG=<tag> docker compose -f docker-compose.prod.yml up -d
```

### Rate limiting appears ineffective (all requests show 127.0.0.1)

**Cause:** Nginx is forwarding the client's `X-Real-IP` header instead of the
real observed TCP source IP.

Verify the Nginx config uses:
```nginx
proxy_set_header X-Real-IP $remote_addr;
```
and NOT `$http_x_real_ip`. See `docs/DEPLOYMENT_TOPOLOGY.md` — "Trusted
reverse proxy requirement" for the security implications of this setting.

### Certificate renewal fails

```bash
# Check the certbot renewal log
sudo cat /var/log/letsencrypt/letsencrypt.log | tail -50

# Check available disk space (certificates require minimal space)
df -h /etc/letsencrypt

# Manually trigger renewal
sudo certbot renew --force-renewal
```

---

## Related documents

- `nginx/nginx.prod.conf` — Reference Nginx config (uses Docker service name;
  adapt upstream to `127.0.0.1:8080` for system service deployment)
- `docs/DISASTER_RECOVERY.md` — Full DR runbook (this is Step 8)
- `docs/EC2_PROVISIONING.md` — EC2 provisioning (must complete before this)
- `docs/DEPLOYMENT_TOPOLOGY.md` — Trusted reverse proxy security requirements
