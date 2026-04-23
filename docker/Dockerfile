# Dockery — all-in-one Docker registry + Web UI + auth.
#
# Four-stage build:
#   1. ui-builder      — build React app, output dist/
#   2. api-builder     — build dockery-api Go binary (CGO off, static)
#   3. registry-src    — pull the official distribution binary
#   4. runtime         — alpine + nginx + s6-overlay orchestrating all three
#
# Invoke from repository root:
#   docker build -f docker/Dockerfile -t dockery:dev .

# ============ Stage 1: web-ui build ============
# --platform=$BUILDPLATFORM: UI output is static/arch-neutral, so run once
# on the runner's native arch instead of once per target platform.
FROM --platform=$BUILDPLATFORM node:24-alpine AS ui-builder
# APP_VERSION is baked into the bundle (see apps/web-ui/vite.config.ts).
# Required — the image must always ship with a concrete version string.
# CI passes the git tag; local builds must supply --build-arg APP_VERSION=...
ARG APP_VERSION
ENV APP_VERSION=${APP_VERSION}
RUN test -n "$APP_VERSION" || ( \
    echo "ERROR: --build-arg APP_VERSION=<version> is required" >&2 && exit 1 )
WORKDIR /w
RUN npm install -g pnpm@9
# Copy only manifest files first for better caching.
COPY apps/web-ui/package.json apps/web-ui/pnpm-lock.yaml ./apps/web-ui/
RUN cd apps/web-ui && pnpm install --frozen-lockfile
COPY apps/web-ui ./apps/web-ui
RUN cd apps/web-ui && pnpm run build

# ============ Stage 2: api build ============
# --platform=$BUILDPLATFORM + GOARCH=$TARGETARCH: cross-compile on the
# runner's native arch so Go's toolchain runs once per target instead
# of the whole stage running under QEMU emulation.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS api-builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /w
COPY apps/api/go.mod apps/api/go.sum ./
RUN go mod download
COPY apps/api .
# Static binary; no cgo so modernc.org/sqlite works on scratch/alpine.
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags="-s -w" -o /out/dockery-api ./cmd/api

# ============ Stage 3: registry binary ============
FROM distribution/distribution:3.1.0 AS registry-src

# ============ Stage 4: runtime ============
FROM alpine:3.20

# nginx: reverse proxy / static server.
# supervisor: PID-1 process manager orchestrating api + registry + nginx.
# ca-certificates / tzdata: sane defaults for TLS and timestamps.
RUN apk add --no-cache nginx supervisor ca-certificates tzdata

# Binaries
COPY --from=registry-src /bin/registry /usr/local/bin/registry
COPY --from=api-builder /out/dockery-api /usr/local/bin/dockery-api

# Static UI
COPY --from=ui-builder /w/apps/web-ui/dist /usr/share/nginx/html

# Supervisord config, nginx + registry + dockery-api configs.
COPY docker/rootfs/ /

# Single public port: nginx fronts UI, /api/*, /token, /v2/*
EXPOSE 5000

# All persistent state lands under /data:
#   /data/registry   — image blobs (filesystem driver)
#   /data/db         — SQLite (users, permissions, audit)
#   /data/config     — Ed25519 jwt keypair + registry http.secret
VOLUME /data

# Silence distribution v3's built-in OpenTelemetry trace exporter. By
# default it tries to POST to localhost:4318 every few seconds and
# spams the log with connection-refused errors when no collector is
# running. Users opt in by setting DOCKERY_OTEL_ENDPOINT at runtime;
# the supervisord registry wrapper translates that into the standard
# OTEL_* vars and unsets this.
ENV OTEL_SDK_DISABLED=true

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:5000/healthz >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/usr/bin/supervisord", "-c", "/etc/supervisord.conf"]
