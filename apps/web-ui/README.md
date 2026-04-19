# Dockery — Web UI

React 19 + TypeScript SPA for [Dockery](../../). Browses and manages images served by `dockery-api`; **never talks to `/v2/` directly** — all registry calls go through `/api/registry/*` on the backend (which handles session auth, per-user filtering, and JWT minting for the upstream registry).

## Tech stack

- React 19 + `babel-plugin-react-compiler`
- TypeScript
- Vite (via `rolldown-vite`) + Tailwind v4 (`@tailwindcss/vite`, no `tailwind.config.js`)
- shadcn/ui components (`components/ui/`, add via `pnpm ui`)
- React Router v7 (`createBrowserRouter`)
- Vitest (jsdom)
- Custom Valtio-based ViewModel layer (`src/lib/viewmodel/`)

## Scripts

```bash
pnpm install
pnpm dev              # :5173; proxies /api /token → :5001, /v2 → :5000
pnpm build            # tsc -b && vite build
pnpm lint
pnpm test
pnpm test:coverage
pnpm ui               # shadcn CLI (add/refresh components)
```

## Layout

```
src/
├── main.tsx           entry
├── router.tsx         /login, /, /tag-list/:image, /admin/users
├── pages/
│   ├── Login/         username + password → session cookie
│   ├── Catalog/       repo list (filtered server-side by user permissions)
│   ├── TagList/       tags + tag detail drawer
│   └── Admin/Users/   admin-only user CRUD
├── layouts/main/      header (UserMenu) + content + footer
├── components/
│   ├── common/        AuthGuard, ErrorPage, SearchBar, …
│   ├── dialogs/
│   └── ui/            shadcn/ui generated components
├── services/
│   ├── api.ts           fetch wrapper; unwraps kratoscarf envelope, throws ApiError
│   ├── auth.service.ts  /api/auth/{login,logout,me}
│   ├── user.service.ts  /api/users/*
│   └── registry.service.ts  /api/registry/*  (ONE entry point for image data)
├── hooks/
│   └── use-current-user.ts  singleton CurrentUserViewModel
└── lib/viewmodel/     Valtio-based OOP state (see its README)
```

## Authentication flow

1. First render: `CurrentUserViewModel.$onMounted` → `GET /api/auth/me`. 401 → `AuthGuard` redirects to `/login`.
2. `/login` submits to `/api/auth/login`; dockery-api sets the HttpOnly `dockery_session` cookie.
3. Subsequent `fetch`s carry the cookie; `ApiError` with `status === 401` re-triggers the redirect.
4. Logout calls `/api/auth/logout`, clears the in-memory session, then navigates to `/login`.

`AuthGuard` accepts `adminOnly` to gate the `/admin/users` route.

## Configuration

Vite reads `VITE_*` from `.env*`. Most paths are hard-coded now that the backend always proxies registry calls; `VITE_REGISTRY_URL` still exists as a fallback override (defaults to `window.location.origin`).

```env
VITE_REGISTRY_URL=
```

## Notes for contributors

- **Adding a page**: create `pages/YourPage/{index.tsx,view-model.ts}`, extend `BaseViewModel<State>`, wire in `router.tsx`.
- **Registry calls**: always add methods to `services/registry.service.ts`; never call `fetch('/v2/…')` directly — the backend will refuse.
- **Role gating**: read from `currentUserViewModel.state.user.role`; don't trust UI-only checks — the server re-enforces everything.
