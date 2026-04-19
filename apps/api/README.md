# Dockery — API

Go 1.25 backend for [Dockery](../../). Single static binary (`dockery-api`) that:

- terminates `/api/*` for the Web UI (session-based auth, user/permission CRUD, registry proxy),
- serves Docker CLI's token realm at `/token` (Basic Auth → scoped Ed25519 JWT),
- boots the Ed25519 keystore + SQLite on first start, derives the JWKS file the distribution registry consumes via `auth.token.jwks`.

## Stack

- [go-kratos/kratos v2](https://go-kratos.dev/) — server framework
- [bizjs/kratoscarf](https://github.com/bizjs/kratoscarf) — router / validator / response-wrapper / session / secure middleware
- [ent](https://entgo.io/) + [modernc.org/sqlite](https://pkg.go.dev/modern.org/sqlite) — ORM on pure-Go SQLite (no CGO; static binary)
- [golang-jwt/jwt/v5](https://github.com/golang-jwt/jwt) + `crypto/ed25519` — registry token signing
- [google/wire](https://github.com/google/wire) — DI
- `golang.org/x/crypto/bcrypt` — password hashing

## Layout

```
apps/api/
├── cmd/api/
│   ├── main.go         entrypoint; EnsureAdmin bootstrap; subcommand dispatch
│   ├── user_cmd.go     `dockery-api user list|create|passwd|grant|revoke|delete`
│   ├── wire.go
│   └── wire_gen.go
├── configs/config.yaml local dev config (listens :5001)
└── internal/
    ├── conf/           yaml schema + generated pb code
    ├── data/           ent client + User/Permission repos
    │   └── ent/        generated ent client + schema/*
    ├── biz/            usecases: user, permission, token, keystore
    ├── service/        HTTP handlers (system, auth, user, permission, registry, token, admin)
    ├── server/         kratoscarf wiring + middleware (RequireSession, RequireAdmin) + route tree
    └── pkg/scope/      Docker scope parsing + glob matching + role→actions
```

## Common commands

```bash
make init              # go mod download + install tool chain
make api               # regenerate ent / wire after schema edits
make run               # ./bin/dockery-api -conf ./configs  (HTTP on :5001)
go test ./...          # unit + integration tests

# Manage users without starting the HTTP server:
./bin/dockery-api -conf ./configs user list
./bin/dockery-api -conf ./configs user create alice write
./bin/dockery-api -conf ./configs user grant  alice 'alice/*,shared/app'
./bin/dockery-api -conf ./configs user passwd alice
./bin/dockery-api -conf ./configs user revoke 42
./bin/dockery-api -conf ./configs user delete alice
```

## Routes (`internal/server/routes.go`)

Grouped by auth requirement:

- **Public** (no session): `GET /healthz` · `GET /readyz` · `GET /ping` · `GET /token`
- **Session loaded, not required**: `POST /api/auth/login`
- **Session required**: `/api/auth/{logout,me}`, `/api/users/{id}/password` (self-or-admin), `/api/registry/{catalog,tags,manifests,blobs}`
- **Session + admin**: `/api/users/*` CRUD, `/api/users/{id}/permissions`, `/api/permissions/{id}`, `/api/admin/{gc,rotate-signing-key}`, `/api/audit`

`/token` bypasses the kratoscarf `{code,message,data}` envelope — it returns the Docker-spec `{token, access_token, expires_in, issued_at}` shape via `ctx.JSON`. Everything else goes through the envelope (`ctx.Success`) and the shared ErrorEncoder.

## Key files

- `internal/biz/keystore.go` — loads/generates `jwt-private.pem` (PKCS#8) and re-derives `jwt-jwks.json` on every boot. JWKS written atomically (tmp + rename) so a concurrent registry reload never sees a truncated file.
- `internal/biz/token.go` — Ed25519 JWT issuance with scoped `access` claim for distribution.
- `internal/biz/permission.go` — `ResolveAccess` is the hot path called on every `/token` hit: load patterns once, intersect each requested scope with role actions.
- `internal/service/registry.go` — UI proxy. Checks per-user permissions, mints a 30s admin-scoped JWT for the upstream registry, filters `/v2/_catalog` output for non-admins.
- `internal/service/token.go` — Docker realm handler; returns the Docker-spec JSON envelope.

## Config (`configs/config.yaml`)

```yaml
server:
  http:
    addr: 0.0.0.0:5001
data:
  database:
    driver: sqlite
    source: file:./data/dockery.db?cache=shared&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)
dockery:
  keystore: { private_path: ./data/jwt-private.pem, jwks_path: ./data/jwt-jwks.json }
  token:    { issuer: dockery-api, audience: dockery, ttl_seconds: 300 }
  admin:    { username: admin, password: "" }                 # or set DOCKERY_ADMIN_PASSWORD env
  session:  { ttl_hours: 168, cookie_name: dockery_session, cookie_secure: false }
```

Inside the container a separate `config.yaml` (from `docker/rootfs/etc/dockery/`) points paths at `/data/…` and binds to `127.0.0.1:3001`.

## Progress

See [`docs/dockery-design.md`](../../docs/dockery-design.md) §11. In short: M1 + M2 done; M3 mostly done but `PermissionService` handlers are stubs; M4 (GC / key rotation / audit writes) not started.
