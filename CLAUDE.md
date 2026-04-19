# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository layout

- `web-ui/` — the active React frontend (this is where almost all development happens).
- `docker-compose.yaml` / `docker-compose.ghcr.yml` — two-container stack: `distribution/distribution:3.0.0` (registry) + `web-ui` behind nginx. Only port 3000 is exposed; the registry is internal-only and reached via nginx proxy at `/v2/`.
- `.github/workflows/build-and-push.yml` — builds the web-ui Docker image and pushes to `ghcr.io/<owner>/<repo>-ui` on tags matching `v*`. Multi-arch (`linux/amd64,linux/arm64`).
- `docker-registry-ui/` — vendored copy of Joxit's docker-registry-ui project (reference only, **not** the shipped UI). Do not edit unless the task specifically targets it.
- `auth/` — empty placeholder for future auth-service work.

## Common commands (run from `web-ui/`)

```bash
pnpm install
pnpm dev              # Vite dev server on :5173, proxies /v2 → http://localhost:4999
pnpm build            # tsc -b && vite build — the typecheck step is the canonical one
pnpm lint             # eslint .
pnpm test             # vitest (jsdom, globals, setup in src/__tests__/setup.ts)
pnpm test -- <file>   # run a single test file
pnpm test:ui          # vitest UI
pnpm test:coverage    # v8 coverage
pnpm ui               # shadcn CLI for adding components
```

Full-stack bring-up: `docker-compose up -d` from repo root, then open http://localhost:3000. Docker daemon needs `"insecure-registries": ["localhost:3000"]` for push/pull to work.

## Architecture

### Container topology

Two services on the `dockery-net` bridge network:

- `registry` (distribution 3.0.0) — no exposed port; storage at `/var/lib/registry`; `REGISTRY_STORAGE_DELETE_ENABLED=true` is required for the UI's delete feature to work.
- `web-ui` (nginx:alpine serving the built SPA) — exposes `3000:80`. `nginx.conf.template` uses `envsubst` on startup to inject `${REGISTRY_URL}` (defaults to `http://registry:5000`), then proxies `/v2/` to the registry. Everything else falls through to the SPA via `try_files`. This is why there are no CORS concerns in prod.

In local `pnpm dev`, the equivalent wiring is in `vite.config.ts` — `/v2` proxies to `http://localhost:4999`. If you run the registry locally for dev, map it to 4999, not 5000.

### Frontend structure (`web-ui/src/`)

- Entry: `main.tsx` → `router.tsx` (React Router v7, `createBrowserRouter`). Routes: `/` (Catalog) and `/tag-list/:image` (TagList), wrapped in `layouts/main`. The `App.tsx` file is Vite's stock scaffold and is **unused** — don't start from it.
- `services/registry.service.ts` — the business-layer API. Composes manifest + config-blob fetches into an `ImageInfo` object (computes total size as config size + sum of layer sizes; correlates `history[]` with `layers[]` by skipping empty layers).
- `lib/registry-client/RegistryClient.ts` — thin, typed wrapper over the Docker Registry v2 HTTP API (catalog, tags, manifest, blob, delete). Uses the OCI + Docker distribution `Accept` header set. Deletion is a two-step flow: `HEAD` for digest, then `DELETE /manifests/{digest}`.
- `services/http.ts` + `services/cache-request.ts` — a separate fetch wrapper with WWW-Authenticate / Bearer token handling and an in-memory response cache keyed by method+URL. Note this coexists with the simpler `fetch`-based `RegistryClient` above; new code should generally go through `registry.service.ts` rather than calling either layer directly.
- `config/index.ts` — reads `VITE_*` env vars. `registryUrl` falls back to `window.location.origin`, which is what lets the prod build work unchanged when nginx is proxying `/v2/` on the same origin.

### ViewModel pattern (important)

Pages use a custom Valtio-based OOP state layer at `lib/viewmodel/` (see its `README.md` for full API). Each page has a `view-model.ts` extending `BaseViewModel<State>` with lifecycle hooks (`$onInit`, `$onMounted`, `$onDestroy`, `$onStateChange`, `$onError`) and `$watch` / `$watchMultiple` for property observers. Components consume it via `useViewModel(VMClass)` (per-component instance) or `useViewModel(vmInstance)` (shared singleton) plus `vm.$useSnapshot()` for reactive reads. Update state only through `$updateState({...})` or direct mutation of `this.state.x` — never reassign `this.state`. When adding a new page, follow the `pages/Catalog/` layout: `index.tsx` + `view-model.ts` + child components.

### UI conventions

shadcn/ui components live in `components/ui/` and are added via `pnpm ui`. Tailwind v4 is configured through `@tailwindcss/vite` (no `tailwind.config.js`). Path alias `@/` → `src/`. React 19 with `babel-plugin-react-compiler` is enabled. The bundler is `rolldown-vite` (pinned via pnpm override) — if you see `vite:` referenced, it's rolldown-vite under the hood.

## Environment variables

Build-time (Vite, `web-ui/.env*`): `VITE_REGISTRY_URL`, `VITE_CATALOG_ELEMENTS_LIMIT`, `VITE_USE_CONTROL_CACHE_HEADER`, `VITE_SINGLE_REGISTRY`, `VITE_SHOW_CONTENT_DIGEST`, `VITE_SHOW_CATALOG_NB_TAGS`.

Runtime (nginx container): `REGISTRY_URL` — substituted into `nginx.conf.template` at container start, so you can change the backend without rebuilding the image.

## Release

Tag-driven: pushing a `v*` tag triggers `.github/workflows/build-and-push.yml`, which builds and publishes the web-ui image to GHCR with `latest` and the semver tag. See `docs/GHCR_DEPLOYMENT.md` for the consumer-side instructions.
