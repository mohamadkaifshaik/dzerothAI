# Dzeroth API — multi-stage production Dockerfile
#
# Build context: apps/backend/
#
# Build command:
#   docker build -f apps/backend/Dockerfile apps/backend/ \
#     --build-arg VERSION=<version> \
#     --build-arg COMMIT=<git-sha> \
#     --build-arg BUILD_TIME=<rfc3339> \
#     -t dzeroth-api:<tag>
#
# The binary is statically linked (CGO_ENABLED=0). All SQL migrations are
# embedded into the binary via //go:embed — no external files are required
# at runtime beyond the binary itself and system CA certificates.

# ── Stage 1: builder ──────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

WORKDIR /build

# Copy dependency manifests first so that go mod download is cached
# independently of application source changes.
COPY go.mod go.sum ./
RUN go mod download

# Copy all application source.
COPY . .

# Build arguments for build identity. Injected at build time via CI/CD or
# the docker build command above. Defaults match the buildinfo package defaults
# so the image still builds correctly when args are omitted.
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

# Produce a fully static binary.
# CGO_ENABLED=0  — no cgo; pgx/v5 and go-redis/v9 are pure Go.
# -s -w          — strip symbol table and DWARF debug info to reduce image size.
# -ldflags -X    — inject build identity into buildinfo variables.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w \
      -X github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/buildinfo.Version=${VERSION} \
      -X github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/buildinfo.Commit=${COMMIT} \
      -X github.com/mohamadkaifshaik/dzerothAI/apps/backend/internal/platform/buildinfo.BuildTime=${BUILD_TIME}" \
    -o /build/bin/api \
    ./cmd/api

# ── Stage 2: runtime ──────────────────────────────────────────────────────────
# alpine:3.20 is chosen over distroless because:
#   1. CA certificates are required for PostgreSQL TLS (POSTGRES_SSL_MODE=verify-full)
#      and Redis TLS (REDIS_TLS=true), both added in Phase 8A-1.
#   2. tzdata is required for timezone-aware weekly title rotation (rank-cron).
#   3. wget is available for HEALTHCHECK without an additional layer.
# alpine:3.20 with apk packages adds ~7 MB; acceptable for the functionality gained.
FROM alpine:3.20 AS runtime

# Install CA certificates for TLS connections and tzdata for timezone support.
# --no-cache avoids storing the apk index in the image layer.
RUN apk --no-cache add ca-certificates tzdata

# Create a non-root system group and user.
# -S creates a system account (no password, no home directory by default).
RUN addgroup -S dzeroth && adduser -S dzeroth -G dzeroth

WORKDIR /app

# Copy the statically-linked binary from the builder stage.
COPY --from=builder /build/bin/api ./api

# Ensure the binary is owned by the non-root user.
RUN chown dzeroth:dzeroth /app/api

USER dzeroth

# Public API server (default API_PORT=8080).
# Admin/observability server (default ADMIN_ADDR=:9091) — do NOT expose
# this port through public load balancers; it serves /livez, /readyz, /metrics.
EXPOSE 8080 9091

# API_PORT matches the default in config.go (8080). Setting it as an ENV makes
# the default explicit and allows the HEALTHCHECK to reference it via ${API_PORT}.
# If API_PORT is overridden at runtime, override this ENV at build time via
# --build-arg API_PORT=<port> so the HEALTHCHECK stays aligned.
# See docs/DEPLOYMENT_TOPOLOGY.md — Healthcheck constraint.
ARG API_PORT=8080
ENV API_PORT=${API_PORT}

# HEALTHCHECK uses the public /health endpoint which is registered on the API
# port in all environments. /livez and /readyz are on the admin port only.
# wget is provided by Alpine's busybox.
# --start-period allows the binary time to connect to PostgreSQL and run migrations
# before the first health check fires.
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD wget -qO- http://localhost:${API_PORT}/health || exit 1

ENTRYPOINT ["/app/api"]
