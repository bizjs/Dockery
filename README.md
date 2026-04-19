# Dockery

Self-hosted Docker Registry — **Distribution v3.1.0 + React UI + accounts/permissions + single image**. One container runs the registry, API, and Web UI behind one nginx port. For small teams and individuals who don't want Harbor.

[中文](./README_CN.md) · [Deployment guide (CN)](./docs/deployment.md) · [Design doc (CN)](./docs/dockery-design.md)

## Features

- 📦 Push / pull / browse OCI + Docker v2 images
- 🔐 CLI + Web UI share one user store; three roles (`admin` / `write` / `view`) with per-user glob repo patterns
- 🔑 Ed25519-signed short-lived registry JWTs (5 min default, verified by registry via JWKS)
- 🌐 React 19 UI: login, route guards, user & permission management, password change
- 🐳 Single image, single port, SQLite + filesystem blob storage; back up `/data`

**Out of scope for v0.1:** image scanning, cosign signing, replication, multi-tenancy, HA, pull-through cache.

## Quick start

```bash
# Docker Desktop → Settings → Docker Engine: "insecure-registries": ["localhost:5001"]
DOCKERY_ADMIN_PASSWORD='change-me' docker compose up --build -d
open http://localhost:5001      # log in as admin / change-me
```

`DOCKERY_ADMIN_PASSWORD` only takes effect on first boot against an empty `/data`. Add your own reverse proxy for TLS (M4 will bundle it).

### Push an image

```bash
docker login localhost:5001
docker tag hello-world localhost:5001/demo/hello:1
docker push localhost:5001/demo/hello:1
```

## User & permission management

**Web UI (admin menu → Manage users)** — create users, edit role, reset password, enable/disable, delete, and manage per-user repo patterns in a drawer for `write` / `view` accounts. Users with the `view` role don't see the tag-delete button. Everyone can self-service password change via the avatar menu.

**CLI fallback** (no HTTP server required):

```bash
docker exec -it dockery dockery-api -conf /etc/dockery user list
docker exec -it dockery dockery-api -conf /etc/dockery user create alice write
docker exec -it dockery dockery-api -conf /etc/dockery user grant  alice 'alice/*,shared/app'
docker exec -it dockery dockery-api -conf /etc/dockery user passwd alice
docker exec -it dockery dockery-api -conf /etc/dockery user revoke 42       # permission id
docker exec -it dockery dockery-api -conf /etc/dockery user delete alice
```

Deleting or demoting the last admin is refused.

## Configuration

### Environment

| Variable | Default | Purpose |
|---|---|---|
| `DOCKERY_ADMIN_USERNAME` | `admin` | First-boot admin username |
| `DOCKERY_ADMIN_PASSWORD` | _required on first boot_ | First-boot admin password |
| `REGISTRY_AUTH_TOKEN_REALM` | `http://localhost:5001/token` | URL the docker CLI reaches for tokens; must match your external URL |
| `REGISTRY_STORAGE_*` | — | Forwarded to distribution to switch storage backends |

Token TTL, issuer, session cookies, etc. live in `docker/rootfs/etc/dockery/config.yaml` (baked into the image). To customize, mount your own over `/etc/dockery/`.

### Persistence

```
/data/
├── registry/          image blobs
├── db/dockery.db      SQLite (users / repo_permissions / audit_log)
└── config/
    ├── jwt-private.pem  Ed25519 private key (0600) — single source of truth
    └── jwt-jwks.json    JWKS derived from the private key on every boot
```

**Back up `/data` as a whole.** Lose `jwt-private.pem` → all issued tokens are void; lose `dockery.db` → user table reset.

## Architecture

```
           external :5001 (host → :5000 container)
                       │
                   [ nginx ]
    ┌────────────┬────────┬────────────┐
    │            │        │            │
   / static   /token   /api/*        /v2/*
    │            │        │            │
 web-ui    dockery-api :3001   distribution :5001
                 │                    ▲
                 ├── SQLite           │
                 ├── jwt-private.pem  │
                 └── jwt-jwks.json ───┘  registry verifies via JWKS
```

Three in-container processes managed by supervisord. Full design in [`docs/dockery-design.md`](./docs/dockery-design.md) (Chinese).

## Local development

```bash
# Frontend (:5173)
cd apps/web-ui && pnpm install && pnpm dev

# Backend (:5001)
cd apps/api && make run

# Bare registry (:5000) for the frontend's /v2 proxy
docker run -p 5000:5000 distribution/distribution:3.1.0
```

## Release

Push a `v*` tag → GitHub Actions builds and pushes `ghcr.io/<owner>/<repo>:<version>` and `:latest` (linux/amd64 + linux/arm64). One image, no `-ui` split.

## License

See [LICENSE](LICENSE). Contributions welcome — please open an issue or discussion first for larger changes.
