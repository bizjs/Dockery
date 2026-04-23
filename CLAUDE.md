# CLAUDE.md

Guidance for Claude Code working in this repository.

> Authoritative design reference: [`docs/dockery-design.md`](./docs/dockery-design.md). This file summarizes; the design doc is source of truth.

## Repository layout

- `apps/web-ui/` — React 19 + TypeScript SPA (Vite / rolldown-vite, Tailwind v4, shadcn/ui, React Router v7). All browser code lives here.
- `apps/api/` — Go 1.25 + Kratos v2 + [kratoscarf](https://github.com/bizjs/kratoscarf) backend. Single static binary (`dockery-api`). ent ORM on SQLite (`modernc.org/sqlite`, no CGO).
- `docker/` — `Dockerfile` (four-stage: ui-builder / api-builder / registry-src / runtime) and `rootfs/` (nginx, supervisord, registry `config.yml`, api `config.yaml` dropped into the container image).
- `docker-compose.yaml` — single-service `dockery` built from `docker/Dockerfile`. Binds host `:5001` → container `:5000`.
- `docs/dockery-design.md` — authoritative design doc (CN).
- `docs/distribution-analysis.md` — upstream Distribution Registry behavior reference.
- `.github/workflows/build-and-push.yml` — builds & pushes `ghcr.io/<owner>/<repo>` on `v*` tags (multi-arch: `linux/amd64,linux/arm64`).

Not in repo yet (planned): pnpm workspace root, `docker-compose.dev.yaml`, `docker-compose.ghcr.yml`.

## Common commands

**Frontend** (run from `apps/web-ui/`):

```bash
pnpm install
pnpm dev              # Vite on :5173; proxies /api /token → :5001, /v2 → :5000
pnpm build            # tsc -b && vite build
pnpm lint
pnpm test             # vitest (jsdom)
pnpm ui               # shadcn CLI
```

**Backend** (run from `apps/api/`):

```bash
make init             # go mod download + tool installs
make api              # regenerate ent / wire (only after schema edits)
make run              # dockery-api -conf ./configs (HTTP on :5001)
go test ./...

# One-shot user management (no HTTP server):
./bin/dockery-api -conf ./configs user list
./bin/dockery-api -conf ./configs user create alice write
./bin/dockery-api -conf ./configs user grant  alice 'alice/*,shared/app'
./bin/dockery-api -conf ./configs user passwd alice
./bin/dockery-api -conf ./configs user revoke 42
./bin/dockery-api -conf ./configs user delete alice
```

**Full stack via compose**:

```bash
DOCKERY_ADMIN_PASSWORD=changeme docker compose up --build -d
open http://localhost:5001      # first login: admin / changeme
```

Docker daemon needs `"insecure-registries": ["localhost:5001"]` until TLS lands.

## Architecture (single container)

Three long-running processes managed by **supervisord** (PID 1, priorities 10/20/30):

1. `dockery-api` (`:3001`) — SQLite at `/data/db/dockery.db`, Ed25519 key at `/data/config/jwt-private.pem`, JWKS at `/data/config/jwt-jwks.json`. Runs first.
2. `registry` (distribution 3.1.0, `:5001`) — polls for `jwt-jwks.json` (200 ms × ~150, ~30 s timeout) before `exec`, then validates incoming tokens via `auth.token.jwks`.
3. `nginx` (`:5000` → host `:5001`) — sole public port. Routes:
   - `/` → static UI (`/usr/share/nginx/html`)
   - `/api/*`, `/token`, `/healthz`, `/readyz` → `:3001`
   - `/v2/*` → `:5001`

Two auth paths share one permission model:
- **Docker CLI**: `docker push` → nginx → registry returns 401 with `WWW-Authenticate: Bearer realm=…/token` → docker hits `/token` (Basic Auth) → dockery-api signs an Ed25519 JWT with scoped `access` claim → registry verifies via JWKS.
- **Web UI**: browser → nginx → `/api/registry/*` on dockery-api → (session check) → mints short-lived admin-scoped JWT for itself → forwards to `127.0.0.1:5001` → filters catalog by repo patterns before returning.

### Roles

Three roles in the `users` table; `users.role` alone dictates actions (no per-row action list):

| role    | registry:catalog:* | repo actions                     |
|---------|--------------------|----------------------------------|
| `admin` | yes                | all, on all repos                |
| `write` | no                 | pull + push + delete (see default below) |
| `view`  | no                 | pull (see default below)         |

`repo_permissions` stores one row per `(user_id, glob_pattern)`. `admin` bypasses this table. **Default when the user has no rows: unrestricted — the role's actions apply to every repo.** Admin narrows this by adding patterns; the first pattern switches the user from "all repos" to "only repos matching any pattern". Applies to both the UI catalog filter and the docker CLI token realm.

### Frontend structure (`apps/web-ui/src/`)

- Entry: `main.tsx` → `router.tsx` (React Router v7). Routes: `/login` · `/` (Catalog, AuthGuard) · `/tag-list/:image` · `/admin/users` (AuthGuard `adminOnly`). `App.tsx` is Vite scaffold — **unused**, don't start from it.
- `services/registry.service.ts` — only entry point for image data; composes manifest + config blob into `ImageInfo`. **Calls `/api/registry/*` (not `/v2/`)**.
- `services/auth.service.ts`, `services/user.service.ts` — thin `api.*` wrappers over the Go backend.
- `services/api.ts` — fetch wrapper with kratoscarf envelope `{code, message, data}` unwrap + `ApiError`.
- `hooks/use-current-user.ts` — singleton `CurrentUserViewModel`; `/me` bootstrap, login/logout mutate state; `AuthGuard` and `UserMenu` observe it.
- `lib/viewmodel/` — Valtio-based OOP state (see its `README.md`). Each page has `index.tsx` + `view-model.ts`.

### Backend structure (`apps/api/internal/`)

- `conf/` — yaml config schema (`conf.proto` + `dockery.go`).
- `data/` + `data/ent/` — ent client + repo adapters for User / RepoPermission / AuditLog.
- `biz/` — usecases: `user`, `permission`, `token` (JWT signing), `keystore` (Ed25519 + JWKS).
- `service/` — HTTP handlers: `system`, `auth`, `user`, `permission` (CRUD for `repo_permissions`), `registry` (UI proxy), `token` (Docker CLI realm), `admin` (GC / key rotation / audit).
- `server/http.go` — kratoscarf wiring (ErrorEncoder / CORS / Secure / Recovery / RequestID / Validator / ResponseWrapper).
- `server/routes.go` — three-tier grouping: public / session / session+admin.
- `server/middleware.go` — `RequireSession`, `RequireAdmin`.
- `pkg/scope/` — Docker scope parsing + glob matching + role→actions mapping.
- `cmd/api/main.go` + `user_cmd.go` + `wire_gen.go` — entry point; `user` subcommand dispatches to `user_cmd.go` without starting HTTP.

### UI conventions

shadcn/ui in `components/ui/` (added via `pnpm ui`). Tailwind v4 via `@tailwindcss/vite` (no `tailwind.config.js`). Path alias `@/` → `src/`. React 19 + `babel-plugin-react-compiler` on. Bundler is `rolldown-vite` (pnpm override).

## Environment variables

**Runtime (container / compose)**:
- `DOCKERY_ADMIN_USERNAME` (default `admin`) — first-boot admin account name.
- `DOCKERY_ADMIN_PASSWORD` (**required on first boot**, otherwise api fatals) — first-boot admin password.
- `REGISTRY_AUTH_TOKEN_REALM` (default `http://localhost:5001/token`) — URL the docker CLI reaches back to for tokens; must match the external URL of the Dockery deployment.
- `REGISTRY_STORAGE_*` — passed through to distribution (S3 etc.).
- `DOCKERY_OTEL_ENDPOINT` (default **unset** → telemetry disabled) — OTLP/HTTP endpoint for distribution's built-in tracing, e.g. `http://jaeger:4318`. Set it to opt in. Dockerfile pins `OTEL_SDK_DISABLED=true` so the default container stays silent (otherwise distribution v3 spams `connection refused` against `localhost:4318`); the registry wrapper flips that when this var is present and exports `OTEL_EXPORTER_OTLP_ENDPOINT` to it.

**Build-time (Vite, `apps/web-ui/.env*`)**: `VITE_REGISTRY_URL` (falls back to `window.location.origin`), plus a few legacy `VITE_*` flags retained for now.

## Progress (see design §11 for detail)

- M1 ✅ skeleton + container + kratoscarf
- M2 ✅ keys + tokens + users + CLI + registry token auth
- M3 ✅ UI session + login + admin/users page + UI-driven permission granting
- M4 ⬜ GC / key rotation / audit log writes / README rebrand

## Release

Push a `v*` tag → `.github/workflows/build-and-push.yml` builds & pushes `ghcr.io/<owner>/<repo>:latest` + `:<semver>` (multi-arch). No separate `-ui` image — Dockery ships as one image.

### Changelog (semi-automatic via git-cliff)

`cliff.toml` drives the generator. The release workflow, after a successful build, runs `git cliff --latest --strip header` to:

1. Create/update the GitHub Release with the current tag's section as the body.
2. Splice the same section into `CHANGELOG.md` on `main` right above the previous `## [x.y.z]` heading (plain awk — not `git cliff --prepend`, which would write above the `# Changelog` preamble) and push the result back with `[skip ci]` so it doesn't retrigger the build.

**Implications for commit style**: commit messages are now the source of truth for the changelog. Use conventional-commit prefixes — `feat:`, `fix:`, `perf:`, `refactor:`, `docs:` land in sections; `chore:` / `ci:` / `build:` / `test:` / `style:` and merge commits are skipped; scopes render as bolded prefixes (`**registry**: …`). Unconventional messages fall through to an "其他" group so nothing disappears silently. Hand-written entries for `0.1.0`–`0.3.0` are preserved because the workflow only splices the `--latest` section.
