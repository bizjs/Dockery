# Dockery 详细设计

> 本文档是 Dockery 的权威设计参考。关于 upstream Distribution Registry 的行为细节，见姐妹文档 [`distribution-analysis.md`](./distribution-analysis.md)，本文不重复抄写。

## 1. 定位与能力边界

### 1.1 一句话
Dockery = **Docker Registry v3.1.0 + 现代 Web UI + 账户/权限系统** 的一体化自托管方案，**一个镜像搞定**。

### 1.2 目标用户
- 小型团队、个人开发者希望自建私有 registry 但不想部署 Harbor 那么重的栈；
- 需要"登录即用、开箱即权限"的自托管场景。

### 1.3 能力范围（v0.1 首版）
- ✅ 推送、拉取、浏览 OCI / Docker v2 镜像；
- ✅ Web UI：镜像目录、标签、详情（layer history / config / labels / cmd / env / exposed ports）、删除；
- ✅ 账户系统：用户 / 角色三档（`admin` / `write` / `view`） / per-repo 仓库模式（glob 通配）；
- ✅ 统一认证：Docker CLI 与 Web UI 共用同一套账户与权限；
- ✅ 单镜像：nginx + dockery-api + registry 由 supervisord 管控（原计划 s6-overlay，详见 §2.1 与 §13.9）。

### 1.4 非目标
- ❌ 不做 Harbor 级的镜像扫描、签名、复制、webhook 编排；
- ❌ 不做多租户、团队、项目层级（有用户和 repo 两层足够）；
- ❌ 不做分布式 / HA 部署（单节点）；
- ❌ 不做镜像代理缓存（`proxy.remoteurl` 不启用）。

---

## 2. 架构总览

### 2.1 进程拓扑（容器内）

```
                   外部端口 :5001 (host → :5000 container)
                            │
                         [nginx]
      ┌─────────────┬─────────────┬────────────────┐
      │             │             │                │
     / 静态       /token       /api/*            /v2/*
      │             │             │                │
  web-ui/dist   [dockery-api :3001]            [registry :5001]
                       │                              ▲
                       ├── SQLite (/data/db)          │
                       ├── /data/config/jwt-private.pem
                       ├── /data/config/jwt-jwks.json ┤ (registry 启动时读 JWKS)
                       └── 代理 + 注入 JWT ───────────┘ (UI 场景)
```

三个长驻进程 + 一份 SQLite + 一把 Ed25519 私钥（JWKS 由私钥在启动时派生并写盘）。

**进程管控**：由 **supervisord**（容器 PID 1）编排三个 longrun：
- `dockery-api`（priority 10）：最先起，写 `jwt-private.pem` 与 `jwt-jwks.json`，监听 `:3001`；
- `registry`（priority 20）：轮询等待 `jwt-jwks.json` 出现（200ms × 最多 150 次，~30s 超时）再 exec；这是替代 s6-overlay `notification-fd` 的简化版 ready 门闩；
- `nginx`（priority 30）：上两者就绪后启动；:5000 对外。

当初 ADR 评估过 s6-overlay，实际为减小镜像层与 Alpine 依赖面，切到了 supervisord + 文件轮询。

### 2.2 两条鉴权路径

**docker CLI 路径**：`docker push/pull` → nginx → registry (401) → nginx → `/token` → dockery-api → Ed25519 签 JWT → registry 验签 → 放行。
**UI 路径**：浏览器 → nginx → `/api/registry/*` → dockery-api（查 session cookie → 查权限 → 生成短命 JWT 注入）→ registry → 响应回流。

两条路径**权限语义完全一致**，来自同一份 `users` + `repo_permissions` 表。

### 2.3 数据流要点
- **SQLite 是唯一 SSOT**；所有用户/权限变更只在这张表落地。
- **Ed25519 私钥是"根凭据"**，丢失等于整个权限系统失守；放 `/data/config/jwt-private.pem`（0600），备份必须包含。
- **Registry 配置文件静态**；GC 和密钥轮换通过"改 config + 重启 registry 进程"实现。

---

## 3. 目录结构

### 3.1 仓库根

```
dockery/
├── apps/
│   ├── web-ui/              React 前端（现有 web-ui 迁入）
│   └── api/                 Go / Kratos + kratoscarf 后端
├── docker/
│   ├── Dockerfile           一体化镜像（多阶段）
│   ├── entrypoint.sh
│   └── rootfs/              镜像内 / 文件系统骨架
├── docs/
│   ├── dockery-design.md    ← 本文档
│   ├── distribution-analysis.md
│   └── GHCR_DEPLOYMENT.md
├── docker-compose.yaml          生产参考
├── docker-compose.dev.yaml      开发（registry 独立跑 :4999）
├── package.json                 pnpm workspace 根
├── pnpm-workspace.yaml
├── CLAUDE.md
├── README.md / README_EN.md
└── .github/workflows/build-and-push.yml
```

### 3.2 apps/api（Kratos 标准 Layout）

```
apps/api/
├── api/dockery/v1/               Proto 契约，按业务拆分
│   ├── auth.proto                /api/auth/{login,logout,me,refresh}
│   ├── user.proto                /api/users/*
│   ├── permission.proto          /api/permissions/*
│   ├── registry.proto            /api/registry/*  (UI 代理)
│   ├── token.proto               /token  (Docker CLI realm)
│   └── admin.proto               /api/admin/* (GC、密钥轮换)
├── cmd/dockery-api/
│   ├── main.go
│   ├── wire.go
│   └── wire_gen.go
├── configs/
│   └── config.yaml               默认配置（env 覆盖）
├── internal/
│   ├── conf/conf.proto           配置 schema
│   ├── server/
│   │   ├── http.go               Kratos HTTP + kratoscarf router 装配
│   │   └── health.go             /healthz、/readyz
│   ├── service/                  Proto 接口实现（薄 DTO 层）
│   │   ├── auth.go
│   │   ├── user.go
│   │   ├── permission.go
│   │   ├── registry.go
│   │   ├── token.go              /token 端点（Basic Auth）
│   │   └── admin.go
│   ├── biz/                      业务逻辑（usecase）
│   │   ├── user.go
│   │   ├── permission.go         Scope 求交集 + glob 匹配
│   │   ├── token.go              JWT 签发 / 校验
│   │   ├── keystore.go           Ed25519 密钥生成与加载
│   │   ├── registry_proxy.go     UI 代理上游 registry
│   │   └── maintenance.go        GC 流程 / 密钥轮换
│   ├── data/                     SQLite + 文件访问
│   │   ├── data.go               ent client 初始化 + 自动建表
│   │   ├── user.go               ent 查询封装成 biz repo 接口
│   │   ├── permission.go
│   │   ├── audit.go
│   │   ├── keystore.go           jwt-*.pem 读写
│   │   └── ent/                  ent 代码仓
│   │       ├── schema/           Schema 定义（手写）
│   │       │   ├── user.go
│   │       │   ├── repopermission.go
│   │       │   └── auditlog.go
│   │       ├── generate.go       //go:generate entc generate ./schema
│   │       └── (生成的 client.go / user.go / 等)
│   └── pkg/
│       └── scope/                Scope 字符串解析
├── third_party/                  googleapis 等
├── Makefile                      kratos / wire / ent / proto 快捷命令
├── openapi.yaml                  由 proto 生成，给前端消费
├── go.mod
└── go.sum
```

### 3.3 docker/rootfs 镜像内结构（实际）

```
docker/rootfs/
└── etc/
    ├── nginx/http.d/default.conf     nginx 单入口 :5000 → SPA / /api / /token / /v2
    ├── docker/registry/config.yml    registry 配置（auth.token.jwks → /data/config/jwt-jwks.json）
    ├── dockery/config.yaml           dockery-api 容器内配置（:3001，路径指 /data）
    └── supervisord.conf              三个 [program:*] 段：dockery-api / registry / nginx
```

编译产物（`dockery-api`、`registry`）由 Dockerfile 直接 `COPY --from=...` 到 `/usr/local/bin/`，不走 rootfs。

与最初设计（s6-overlay + `s6-rc.d/`）相比：supervisord 单文件配置，依赖关系用 `priority=10/20/30` 表达，`dockery-api` 的 ready 信号由 registry 进程自己"轮询 JWKS 文件"代替。

---

## 4. 技术栈

| 层 | 选型 | 备注 |
|---|---|---|
| 前端框架 | React 19 + TypeScript | 现有 web-ui 保留 |
| 前端样式 | Tailwind CSS v4 + shadcn/ui | 现有 |
| 前端状态 | 自研 Valtio ViewModel (`src/lib/viewmodel/`) | 现有，保留 |
| 前端构建 | Vite (rolldown-vite) | 现有 |
| 前端路由 | React Router v7 | 现有 |
| 前后端类型同步 | `openapi.yaml` → `openapi-typescript` | Kratos 生成 → 前端消费 |
| 后端语言 | Go 1.25+ | 单静态二进制 |
| 后端框架 | Kratos v2 + kratoscarf | kratoscarf 提供 router / validation / response / session |
| 数据库 | SQLite (纯 Go) | `modernc.org/sqlite`，无 CGO，Alpine 静态编译友好 |
| ORM | ent (entgo.io/ent) | Facebook 出品，Kratos 推荐；schema-as-code，图式查询 |
| 迁移 | ent 自带 `Schema.Create` + Atlas | 开发期自动建表；生产可生成 SQL 脚本审查后上 |
| JWT | `golang-jwt/jwt/v5` + `crypto/ed25519` | Registry token 手签，不走 kratoscarf |
| 密码散列 | `golang.org/x/crypto/bcrypt` | DefaultCost |
| Registry | `distribution/distribution:3.1.0` | 从官方镜像 COPY 二进制进最终镜像 |
| 反向代理 | nginx alpine | 统一入口 :5000 |
| 进程管控 | supervisord | Alpine 原生包；轻量依赖编排 + autorestart（替代最初计划的 s6-overlay） |
| 基础镜像 | alpine:3.20 | 最终运行时 |
| CI/CD | GitHub Actions → GHCR | 现有 workflow 微调镜像名 |

---

## 5. 数据模型

### 5.1 SQLite Schema（参考视图）

> 下方 SQL 为**参考视图**，帮助理解关系；实际表结构由 `internal/data/ent/schema/*.go` 的 ent Schema 驱动，`ent.Client.Schema.Create(ctx)` 在启动时自动同步。生产环境可用 `atlas migrate diff` 导出 SQL 脚本做审查部署。

```sql
-- 用户。角色三档：
--   admin — 全权；bypass repo_permissions（含 registry:catalog:*）
--   write — 对匹配 pattern 的 repo 可 pull + push + delete
--   view  — 对匹配 pattern 的 repo 仅 pull
CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  username      TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,                 -- bcrypt
  role          TEXT NOT NULL CHECK(role IN ('admin','write','view')),
  disabled      INTEGER NOT NULL DEFAULT 0,
  created_at    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL
);

-- 仓库模式白名单（仅 write/view 使用；admin 不进此表）。
-- 只回答"哪些 repo 可访问"；actions 由 user.role 决定。
-- 管理 API 允许一次提交多个 pattern（CSV 或数组），后端拆成多行入库，
-- 便于按单条精准撤销与索引。
CREATE TABLE repo_permissions (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  repo_pattern TEXT NOT NULL,                  -- 支持 '*'、'alice/*'、'alice/app'
  created_at   INTEGER NOT NULL,
  UNIQUE(user_id, repo_pattern)
);

-- UI 会话（M3 视需要而定；也可直接用短命 JWT cookie 不落库）
CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,                 -- ULID
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at INTEGER NOT NULL,
  ip         TEXT,
  user_agent TEXT
);

-- 审计日志
CREATE TABLE audit_log (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  ts         INTEGER NOT NULL,
  actor      TEXT NOT NULL,
  action     TEXT NOT NULL,                    -- token.issued / user.created / image.deleted / gc.started ...
  target     TEXT,                             -- repository:alice/app
  scope      TEXT,                             -- 授予的 actions
  client_ip  TEXT,
  success    INTEGER NOT NULL,
  detail     TEXT                              -- JSON，额外上下文
);

CREATE INDEX idx_audit_ts ON audit_log(ts DESC);
CREATE INDEX idx_perm_user ON repo_permissions(user_id);
```

### 5.2 文件系统布局（/data）

```
/data/
├── registry/                         镜像 blob 存储（REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY）
├── db/
│   └── dockery.db                    SQLite 主库
└── config/
    ├── jwt-private.pem               Ed25519 私钥 (0600)，单一 SoT
    └── jwt-jwks.json                 由私钥派生的 JWKS（RFC 7517/8037），每次启动覆写
```

**私钥是单一真源**；公钥不单独落盘，JWKS 在 dockery-api 启动时从私钥派生并原子写入（tmp + rename），给 registry 消费（`auth.token.jwks`）。切换到 JWKS 的原因：distribution v3.1.0 的 `rootcertbundle` 只接受 CERTIFICATE PEM，会静默丢弃 PUBLIC KEY block；`jwks` 字段接受 RFC 7517 JSON Web Key Set。

**registry-secret**：当前直接写在 `config.yml` 里作为 placeholder，尚未持久化到 `/data/config/`；M4 密钥轮换时一并落盘。

**备份 = `/data` 目录整包**。没有 `/data/db/dockery.db` + `/data/config/jwt-private.pem`，老 token 验不过，用户登不上，等于从零重建。

---

## 6. API 设计

### 6.1 端点总览

| 路径 | 方法 | 鉴权 | 用途 |
|---|---|---|---|
| `/healthz` | GET | 无 | nginx 健康检查 |
| `/readyz` | GET | 无 | API 是否 ready |
| `/token` | GET | Basic Auth | Docker CLI token realm |
| `/api/auth/login` | POST | 无 | UI 登录 |
| `/api/auth/logout` | POST | Session | 注销 |
| `/api/auth/me` | GET | Session | 当前用户信息 |
| `/api/users` | GET/POST | Session + admin | 用户列表/创建 |
| `/api/users/{id}` | GET/PATCH/DELETE | Session + admin | 单用户管理 |
| `/api/users/{id}/password` | PUT | Session (self or admin) | 改密 |
| `/api/users/{id}/permissions` | GET/POST | Session + admin | 用户的 repo 权限列表 |
| `/api/permissions/{id}` | PATCH/DELETE | Session + admin | 单条权限管理 |
| `/api/registry/catalog` | GET | Session | UI 代理：列 repo |
| `/api/registry/{name}/tags` | GET | Session | UI 代理：列 tag |
| `/api/registry/{name}/manifests/{ref}` | GET/DELETE | Session | UI 代理：manifest |
| `/api/registry/{name}/blobs/{digest}` | GET | Session | UI 代理：config blob |
| `/api/admin/gc` | POST | Session + admin | 触发垃圾回收 |
| `/api/admin/rotate-signing-key` | POST | Session + admin | 密钥轮换（重启 registry） |
| `/api/audit` | GET | Session + admin | 审计日志查询 |

### 6.2 /token 端点（关键）

**请求**：
```
GET /token?service=dockery&scope=repository:alice/app:pull,push HTTP/1.1
Authorization: Basic <base64(user:pass)>
```

**处理步骤**：
1. 解 Basic Auth → `users` 表 → bcrypt 比对 → 失败返 401（不暴露"用户是否存在"）；
2. 解析每个 `scope`；对非 admin 用户，先判断是否存在 `repo_permissions` 行的 `repo_pattern` glob 命中 `scope.Name`；
3. 确定本用户的 **role_actions**：`admin → 全集`、`write → {pull, push, delete}`、`view → {pull}`；
4. 求交集：`final = requested.Actions ∩ role_actions`（仅当 pattern 命中或 user=admin）；
5. 拼装 JWT `access` claim（即使 `final` 为空也签发）；
6. Ed25519 签名；
7. 审计写一条 `token.issued`。

**响应**：
```json
{"token":"<JWT>","access_token":"<JWT>","expires_in":300,"issued_at":"2026-04-18T10:00:00Z"}
```

### 6.3 错误响应格式（kratoscarf 统一）

```json
{
  "code": 40101,
  "message": "invalid credentials",
  "data": null
}
```

业务 code 范围：
- `0` 成功
- `40xxx` 客户端错误（400 族 + 具体业务码）
- `50xxx` 服务端错误

对应 Docker Registry 返回的标准错误（401 UNAUTHORIZED 等）由 `/v2/*` 原样透传，不套 kratoscarf 信封，保持 registry 协议兼容。

---

## 7. 认证与授权

此节在 [`distribution-analysis.md` §4](./distribution-analysis.md) 已详细展开（三方协议、JWT 格式、scope 语法、密钥管理）。此处只补充 Dockery 特有的实现决策：

### 7.1 两类凭据严格分离

| 类别 | 介质 | 用途 | 签发者 | 验签 / 解析方 |
|---|---|---|---|---|
| **Registry Token** | Ed25519 JWT（私钥签） | docker CLI / UI 代理访问 /v2/ | `biz/token.go:IssueRegistryToken` | Registry 本身（读 `jwt-jwks.json`） |
| **UI Session** | 不透明 session ID（HttpOnly cookie `dockery_session`） | Web UI 会话 | kratoscarf `auth/session` + `session.NewManager` | dockery-api 内存 Store 查表 |

分离的好处：UI session 永远出不了 dockery-api；Registry token 即使泄露也 5 分钟过期，且只带对应 scope。

**Session Store 当前实现**：`session.NewMemoryStore` —— 进程内 map。含义：
- 进程重启 = 所有在线用户被迫重新登录；
- 单节点部署完全够用（Dockery 本就非 HA）；
- 横向扩展 / 强制下线需求请在 M4 切 SQLite-backed store（设计 §14 已登记为 open question，v0.1 选了内存）。

**Logout 限制**：`/api/auth/logout` 仅清空 session 值，不会调 Manager.DestroySession（kratoscarf 目前要求原始 ResponseWriter，biz 层拿不到）。Cookie 本身按 TTL 到期；下一次带老 cookie 的请求会命中一个空 session，被 `RequireSession` 拒绝。等价于"登出"，但不是"立即吊销"。若要严格吊销，M4 换 SQLite store 时一并补足。

### 7.2 UI 代理层 JWT 注入流程

```
UI (带 session cookie)
  → GET /api/registry/catalog
dockery-api:
  1. kratoscarf session middleware 校验 cookie → 得到 user
  2. biz.registry_proxy.ProxyCatalog(user):
       scope := "registry:catalog:*"
       granted := permission.Match(user, scope)    // admin 直通
       token := biz.token.IssueRegistryToken(user, granted, 30*time.Second)
       resp := http.Get("http://127.0.0.1:5001/v2/_catalog", Bearer token)
       filter(resp, user)                           // 二次裁剪：只返回 user 可见的 repo
  3. ctx.Success(filtered)
```

**二次裁剪**是安全兜底：即使某天 token 签错，API 层仍不会把用户没权的 repo 透给 UI。

### 7.3 Scope 匹配算法

```go
// pkg/scope/match.go (伪代码)
var roleActions = map[string][]string{
    "admin": {"pull", "push", "delete", "*"},
    "write": {"pull", "push", "delete"},
    "view":  {"pull"},
}

func Match(user User, requested Scope) (granted []string) {
    // registry:catalog:* 类管理 scope 仅 admin 拥有
    if requested.Type == "registry" {
        if user.Role == "admin" { return requested.Actions }
        return nil
    }
    // admin 跳过 pattern 检查
    if user.Role == "admin" {
        return requested.Actions
    }
    // write/view 必须至少有一条 pattern 命中
    var matched bool
    for _, p := range user.Permissions {
        if glob.Match(p.RepoPattern, requested.Name) { matched = true; break }
    }
    if !matched { return nil }

    return intersect(requested.Actions, roleActions[user.Role])
}
```

- glob 支持 `*`、`alice/*`、`alice/app`；不支持正则。多条命中等价（只要命中就过）。
- `actions` 不再存在权限表中 —— 角色唯一决定能做什么。
- 非 admin 用户拿不到 `registry:catalog:*`；UI 代理层通过过滤后的列表给 view/write 用户呈现 catalog。

---

## 8. 关键流程

### 8.1 docker login 与 push

```
$ docker login registry.example.com:5000 -u alice
Password: ****
```
1. Docker 用 Basic Auth 试 `GET /v2/`；
2. 401 + `WWW-Authenticate: Bearer realm=".../token",...`；
3. Docker 调 `/token?service=dockery&scope=` （空 scope 只是探活）+ Basic Auth；
4. dockery-api 验密码成功 → 签一个空 `access` 的 JWT 返回；
5. Docker 存 credential。

后续 `docker push registry.example.com:5000/alice/app:v1`：
1. PUSH 触发 `PUT /v2/alice/app/blobs/uploads/...`；
2. registry 401 + `scope="repository:alice/app:pull,push"`；
3. Docker 调 `/token` 得到含该 scope 的 JWT；
4. 带 Bearer JWT 重试 → registry 验签 → 放行；
5. 分块上传完成。

### 8.2 UI 登录 + 浏览

1. 用户提交 `/api/auth/login` (username, password)；
2. API 验密码 → 生成 Session JWT 写 HttpOnly cookie；
3. UI 跳 `/`，发起 `/api/registry/catalog`；
4. API 按 §7.2 流程代理 + 裁剪返回 repo 列表。

### 8.3 创建用户并授权

管理员在 UI 上：
1. `POST /api/users` `{username, password, role}`，`role ∈ {admin, write, view}`。
2. `POST /api/users/{id}/permissions` `{repo_patterns: ["alice/*", "shared/app", "team/api/*"]}`
   —— 后端把列表拆成多行写入 `repo_permissions`；重复的 pattern 冲突由 unique 约束自然拦下。
3. 新用户立即可用 `docker login` + 按 role 决定的能力操作匹配 pattern 的 repo。
   admin 创建时**不传 permissions**，其本身就是全权。

无需重启任何进程。撤销某条 pattern：`DELETE /api/permissions/{id}`。

### 8.4 垃圾回收（M4 计划）

UI admin 页点"执行 GC"：
1. POST `/api/admin/gc`；
2. API 写维护 flag，`/v2/` 的写入路径在 UI 代理层 / `/token` 端点返回 503，阻断新写入；
3. API 通过 `supervisorctl stop registry`（走 `/run/supervisor.sock`）停 registry；
4. 进程外 `exec registry garbage-collect /etc/docker/registry/config.yml`；
5. `supervisorctl start registry` 重启；
6. 清维护 flag；写 audit `gc.completed`。

### 8.5 密钥轮换（M4 计划）

1. POST `/api/admin/rotate-signing-key`；
2. 生成新 Ed25519 私钥，原子写 `jwt-private.pem`（tmp + rename，0600）；
3. Keystore 重新加载，派生 JWKS 原子写 `jwt-jwks.json`；
4. `supervisorctl restart registry` 让 registry 重新读 JWKS；
5. 用旧密钥签的 token 全部失效（用户会被迫重新 docker login + 重新访问 UI）；
6. 写 audit `key.rotated`。

---

## 9. 容器构建

### 9.1 多阶段 Dockerfile（实际）

实际 Dockerfile（`docker/Dockerfile`）是四阶段：

```dockerfile
# ============ Stage 1: build web-ui ============
FROM node:24-alpine AS ui-builder
WORKDIR /w
RUN npm install -g pnpm@9
COPY apps/web-ui/package.json apps/web-ui/pnpm-lock.yaml ./apps/web-ui/
RUN cd apps/web-ui && pnpm install --frozen-lockfile
COPY apps/web-ui ./apps/web-ui
RUN cd apps/web-ui && pnpm run build

# ============ Stage 2: build dockery-api ============
FROM golang:1.25-alpine AS api-builder
WORKDIR /w
COPY apps/api/go.mod apps/api/go.sum ./
RUN go mod download
COPY apps/api .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/dockery-api ./cmd/api

# ============ Stage 3: registry binary ============
FROM distribution/distribution:3.1.0 AS registry-src

# ============ Stage 4: runtime ============
FROM alpine:3.20
RUN apk add --no-cache nginx supervisor ca-certificates tzdata
COPY --from=registry-src /bin/registry /usr/local/bin/registry
COPY --from=api-builder /out/dockery-api /usr/local/bin/dockery-api
COPY --from=ui-builder /w/apps/web-ui/dist /usr/share/nginx/html
COPY docker/rootfs/ /
EXPOSE 5000
VOLUME /data
ENTRYPOINT ["/usr/bin/supervisord", "-c", "/etc/supervisord.conf"]
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:5000/healthz >/dev/null 2>&1 || exit 1
```

说明：
- 当前未使用 pnpm workspace，Stage 1 直接在 `apps/web-ui/` 里装依赖；
- Stage 2 入口是 `./cmd/api`（不是设计最初写的 `./cmd/dockery-api`）；
- runtime 装 `supervisor` 而非 `s6-overlay`（详见 §9.2）。

### 9.2 supervisord 依赖关系（实际）

三个 `[program:*]` 段定义在 `docker/rootfs/etc/supervisord.conf`：

- `dockery-api`（priority 10）：`sh -c 'mkdir -p /data/db /data/config /data/registry && exec dockery-api -conf /etc/dockery'`。mkdir 是为 SQLite 打开 `/data/db/dockery.db` 留出目录（volume 首次挂载为空）。
- `registry`（priority 20）：轮询等待 `/data/config/jwt-jwks.json` 文件出现（每 200ms 检查一次，上限 ~30s），再 `exec registry serve /etc/docker/registry/config.yml`。这是替代 s6-overlay `notification-fd` 的简化就绪门闩。
- `nginx`（priority 30）：直接 `nginx -g "daemon off;"`；前两者没起时访问 /api 或 /v2 会短暂 502，这是可接受代价。

`autorestart=true` 对三者均开启；任一服务进程异常退出都会被 supervisord 拉起。如果需要"任一 critical 服务死掉就整体退容器"的语义，未来可用 `exitcodes` + `startretries=0` 实现。

---

## 10. 配置与持久化

### 10.1 环境变量

**当前实际读取**（`cmd/api/main.go` + `docker-compose.yaml`）：

| 变量 | 默认 | 说明 |
|---|---|---|
| `DOCKERY_ADMIN_USERNAME` | `admin` | 首次启动创建的 admin 账号名（users 表为空时才生效） |
| `DOCKERY_ADMIN_PASSWORD` | _（必填，否则启动 fatal）_ | 首次启动的 admin 密码；也可写在 `config.yaml` 里但不建议 |
| `REGISTRY_AUTH_TOKEN_REALM` | `http://localhost:5001/token` | distribution registry 读；docker CLI 拿到 401 后回源 `/token` 用的 URL |
| `REGISTRY_STORAGE_*` | 透传 | 用户想切 S3 等后端时覆盖 |

**config.yaml（`dockery` 段）**：token TTL、issuer、audience、session TTL/cookie、keystore 路径都从 yaml 读，不走 env。M4 可加 env 覆盖层。

**设计中列过但尚未实现**：`DOCKERY_SESSION_SECRET`（当前 session 用不透明 ID + 内存 Store，无需 HMAC secret）、`DOCKERY_PUBLIC_URL`（realm URL 直接走 `REGISTRY_AUTH_TOKEN_REALM`）、`DOCKERY_TOKEN_TTL`、`DOCKERY_LOG_LEVEL`。

### 10.2 挂载卷

```yaml
volumes:
  - dockery-data:/data
ports:
  - "5001:5000"   # host 5001 avoids macOS AirPlay Receiver on :5000
```

---

## 11. 实施里程碑

以下把"用户创建项目框架" 与 "Claude 协助填充" 分开标注。状态标识：
✅ 完成　⏳ 进行中　⬜ 未开始　⏭️ 跳过（有替代）

### 进度速查（截至 2026-04-19）
- **M1.1** ✅ 项目重组（`apps/api` 已生成、`apps/web-ui` 已迁入）。**未建 pnpm workspace 根**（Dockerfile 目前在 `apps/web-ui/` 内独立装依赖）
- **M1.2** ✅ kratoscarf 集成（ErrorEncoder / CORS / Secure / Recovery / RequestID / Router / Validator / ResponseWrapper）
- **M1.3** ⏭️ proto 生成跳过 —— 改用 kratoscarf 直接声明路由
- **M1.4** ✅ 容器化 —— 四阶段 Dockerfile + nginx + **supervisord** + `docker-compose.yaml`（本地 build）。**未建** `docker-compose.dev.yaml` / `docker-compose.ghcr.yml`
- **M2.1** ✅ ent schema (User/RepoPermission/AuditLog) + SQLite + auto-migrate
- **M2.2** ✅ 密钥 (Ed25519) + JWKS 派生写盘 + Token 签发 (JWT/EdDSA) + Scope 匹配 —— 全层单测覆盖
- **M2.3** ✅ EnsureAdmin + bcrypt + CLI (`user list/create/passwd/grant/revoke/delete`) + `/token` 真实签发 + `/api/auth/login` 真实验证
- **M2.4** ✅ Registry 切 token auth（`auth.token.jwks` → `/data/config/jwt-jwks.json`；`REGISTRY_AUTH_TOKEN_REALM` 由 env 覆盖）。distribution v3.1.0 不接受 PUBLIC KEY PEM 的 `rootcertbundle`，故改用 JWKS
- **M3**   ⏳ UI 接入与权限化 —— **部分完成**
  - ✅ kratoscarf `auth/session`（内存 Store）、`RequireSession` / `RequireAdmin`
  - ✅ Auth Login/Me/Logout；User List/Create/Get/Update/Delete/SetPassword
  - ✅ UI：Login 页 + AuthGuard（含 adminOnly）+ Header UserMenu + Admin/Users 页（创建/禁用/删除）
  - ✅ UI `registry.service.ts` 改走 `/api/registry/*`
  - ⬜ **PermissionService handler 全部 `errNotImplemented()`**（biz 层就绪）—— admin 无法在 UI/HTTP 管理 repo 授权（仅 CLI）
  - ⬜ UI 用户管理页缺 role 编辑 / 改密 / 自助改密 / 权限列表
  - ⬜ UI 按角色隐藏 / 禁用删除按钮
- **M4**   ⬜ 打磨与发布 —— 全部 TODO
  - ⬜ `AdminService.TriggerGC` / `RotateKey` / `Audit` 仅 stub
  - ⬜ 审计日志：ent schema 齐备但**没有任何写入点**（`/token` 签发、user/perm 变更、image 删除都没记）
  - ⬜ README.md / README_CN.md Dockery 品牌更新
  - ⬜ `docker-compose.ghcr.yml` + GHCR tag 流程验证
  - ⬜ apps/api/README.md 仍是 Kratos 模板


### M1 — 骨架搭建与 kratoscarf 集成（你主导）

**目标**：能 `docker run dockery:dev` 跑起来，跑通旧 UI 的浏览 + 删除，**尚不启用 auth**。后端跑在 Kratos + kratoscarf 之上，有 `/healthz` 可访问。

#### M1.1 项目重组与依赖
1. **[你]** 删除 `docker-registry-ui/` 与空的 `auth/`；迁 `web-ui/` → `apps/web-ui/`；建 `apps/api/`。
2. **[你]** 初始化 pnpm workspace（`pnpm-workspace.yaml` 含 `apps/web-ui`；根 `package.json` 加 `dev:ui/dev:api/build/docker` 脚本）。
3. **[你]** `cd apps/api && kratos new .` 或手工按 §3.2 建 Kratos layout；`go mod init github.com/bizjs/dockery/apps/api`。

#### M1.2 集成 kratoscarf（关键，先做再谈业务）
4. **[你]** `go get github.com/bizjs/kratoscarf@latest`。
5. **[我]** 改写 `internal/server/http.go`：
   - 用 `kratoshttp.NewServer` 创建 server，注入 `response.NewHTTPErrorEncoder()` 作为 ErrorEncoder；
   - Filter 链加 `middleware.CORS()` + `middleware.Secure(...)`；
   - Kratos middleware 链加 `middleware.RequestID()`；
   - `router.NewRouter(srv, router.WithValidator(validation.New()), router.WithResponseWrapper(response.Wrap))`。
6. **[我]** 写 `internal/server/health.go`：用 kratoscarf `health.New()` 暴露 `/healthz` + `/readyz`，liveness 直返 ok，readiness 留 hook（M2 再接 DB + 密钥就绪检查）。
7. **[我]** 写一个样板 handler `internal/service/ping.go`，演示 `ctx.Bind/ctx.Success/return err` 三段式确已生效（此 handler 在 M2 后删除）。
8. **[验证 kratoscarf 集成]**：
   - `GET /healthz` 返 `{"status":"ok"}`；
   - `GET /ping?name=` 返 422（validator 生效）；
   - `GET /ping?name=foo` 返 `{"code":0,"message":"ok","data":{...}}`（response wrapper 生效）；
   - 任意 handler `return response.ErrInternal.WithCause(err)` 返标准错误信封（ErrorEncoder 生效）。

#### M1.3 Proto 骨架
9. **[你]** `kratos proto add api/dockery/v1/auth.proto` 起一个空 AuthService；`make api` 跑通 `protoc` 生成。只要保证 proto 工具链工作即可，真正的 service 实现 M2 再写。

#### M1.4 容器化
10. **[我]** 写 `docker/Dockerfile` 四阶段骨架（ui-builder / api-builder / registry-src / runtime）。
11. **[我]** 写 `docker/rootfs/etc/nginx/http.d/default.conf`：路由 `/` → static，`/api/*` + `/token` → `:3001`，`/v2/*` → `:5001`。
12. **[我]** 写 `docker/rootfs/etc/supervisord.conf` 三个 `[program:*]` 段与启动顺序（priority 10/20/30）；M1 阶段 registry 用无 auth 配置先跑通。
13. **[我]** 改 `docker-compose.yaml` 为单服务 `dockery`（本地构建），删除原 `registry` / `web-ui` 两服务。**尚未**写 `docker-compose.dev.yaml`（registry 独立跑 + `pnpm dev` + `make run`）。
14. **[我/你]** 更新 `CLAUDE.md` 反映新结构（M1 完工后一起提交）。

**验收**：
- `docker compose up --build` 起来；`:5001/healthz` 返 `{"status":"ok"}`；
- 访问 `:5001/` 能看到旧 UI，能浏览/删除镜像；
- docker push/pull 走 `:5001/v2/` 正常（registry 无 auth 模式）；
- `make run` 在本地跑起 dockery-api，kratoscarf 三段式行为验证通过。

### M2 — Token Server 与账户（核心）

**目标**：`docker login` + 按 repo 权限推拉；UI 暂未适配。

#### M2.1 数据层（ent）
1. **[你]** `cd apps/api && go get entgo.io/ent/cmd/ent`；`ent init --target internal/data/ent/schema User RepoPermission AuditLog`。
2. **[我]** 填充 `internal/data/ent/schema/*.go`：按 §5.1 定义字段、索引、唯一约束、edges（User 1:N RepoPermission）。
3. **[我]** `internal/data/ent/generate.go` 加 `//go:generate go run -mod=mod entgo.io/ent/cmd/ent generate ./schema`；`make ent` 跑生成。
4. **[我]** `internal/data/data.go`：用 `modernc.org/sqlite` 打开 `/data/db/dockery.db`，初始化 `ent.Client`，启动时 `client.Schema.Create(ctx)` 自动建表/升级。
5. **[我]** `internal/data/user.go` / `permission.go` / `audit.go`：把 ent 查询封装成 biz 层需要的 repo 接口（便于 biz 层单测 mock）。

#### M2.2 密钥与 Token
6. **[我]** `biz/keystore.go`：启动时检查 `/data/config/jwt-private.pem`，缺失则生成 Ed25519 私钥（PKCS#8、0600）；无论是否新生成，每次启动都从私钥派生 JWKS 原子写入 `jwt-jwks.json`（0644）。公钥不单独落盘。
7. **[我]** `biz/token.go`：`IssueRegistryToken(subject, access)` 用 `golang-jwt/jwt/v5` + `crypto/ed25519` 签 JWT（`kid` = RFC 7638 JWK thumbprint）。UI session 不走 JWT，改用 kratoscarf `auth/session` 的不透明 ID（内存 Store）。
8. **[我]** `pkg/scope/` + `biz/permission.go`：scope 解析 + glob 匹配 + role → actions 映射（admin / write / view 三档直查表）。

#### M2.3 用户初始化
9. **[我]** `biz/user.go`：`EnsureAdmin()` —— 启动时若 users 表空，从 `DOCKERY_ADMIN_USERNAME`/`DOCKERY_ADMIN_PASSWORD`（env 优先于 yaml）创建首位 admin；未设密码则进程 fatal（不随机生成，避免日志泄漏）。
10. **[我]** `cmd/api` 子命令：`dockery-api user list/create/passwd/grant/revoke/delete`，M3 UI 未好前的管理口。

#### M2.4 接入 Registry
11. **[我]** `service/token.go`：`GET /token` handler —— 解 Basic Auth、查 ent、求交集、签 JWT、写审计（审计写入是 M4 待办）。这个 handler 不套 kratoscarf 的 response wrapper（要返回 Docker 规定的 JSON 结构）。
12. **[我]** `docker/rootfs/etc/docker/registry/config.yml` 切到 `auth: token`（**实际用 `jwks` 而非 `rootcertbundle`** — distribution v3.1.0 的 `rootcertbundle` 只识别 CERTIFICATE PEM，会丢 PUBLIC KEY block）：
    ```yaml
    auth:
      token:
        realm: http://localhost:5001/token    # 由 REGISTRY_AUTH_TOKEN_REALM 覆盖
        service: dockery
        issuer: dockery-api
        jwks: /data/config/jwt-jwks.json
        signingalgorithms: [EdDSA]
    ```
13. **[我]** supervisord `registry` 段加"等 JWKS 文件"的轮询门闩（200ms × 150，~30s 超时）替代 s6-overlay `notification-fd`；`registry` priority=20 在 `dockery-api` priority=10 之后启动，即便 supervisord 本身没有 hard dependency 概念。

**验收**：
- `docker login :5001` 用 env 注入的 admin 通过；
- admin 能 push 任意 repo；
- `dockery-api user create alice`、`dockery-api user grant alice 'alice/*' pull,push` 后，alice 能 push `alice/foo`、不能 push `bob/bar`；
- 未授权返 401；权限改动立即生效（无需重启，下一次 /token 签发即反映）；
- `readyz` 在密钥/DB 未就绪前返 503，就绪后返 200。

### M3 — UI 接入与权限化

**目标**：Web UI 登录、按角色显隐、用户管理页可用。

1. **[我]** `service/auth.go`：`/api/auth/{login,logout,me}`；kratoscarf session。
2. **[我]** `biz/registry_proxy.go`：`/api/registry/*` 代理层 + 二次裁剪。
3. **[我]** `service/user.go` + `service/permission.go`：用户/权限 CRUD。
4. **[我]** `make openapi` 导出 `openapi.yaml`。
5. **[你/我]** `apps/web-ui/` 加 `openapi-typescript` 生成 TS 客户端到 `src/gen/`。
6. **[我]** web-ui 登录页、"当前用户"头像、401 自动跳登录。
7. **[我]** 改 `registry.service.ts`：所有 /v2/ 调用走 `/api/registry/*`。
8. **[我]** 用户管理页（admin-only）：列表、创建、改密、授权、禁用。
9. **[我]** 按 role 隐藏/禁用 UI 删除按钮。
10. **验收**：
    - 新用户登录只看到自己有权的 repo；
    - admin 能在 UI 管理用户与权限；
    - 删除按钮对 reader 不可见 / 对无 `delete` 权限的用户返 403。

### M4 — 打磨与首个发布

1. **[我]** GC 触发端点（§8.4 流程）与 UI 维护页。
2. **[我]** 密钥轮换端点（§8.5）。
3. **[我]** 审计日志查询页。
4. **[我]** 更新 `README.md` / `README_CN.md` 为 Dockery 品牌，标注能力与局限。
5. **[我]** `docker-compose.yaml` / `docker-compose.ghcr.yml` 切到 `ghcr.io/bizjs/dockery:latest`。
6. **[我]** `.github/workflows/build-and-push.yml` 镜像名改 `dockery`，registry tag 用 `v*`。
7. **[我]** 补基础测试：`pkg/scope` 单测、token 签发 / 验签单测、几个 biz 层集成测。
8. **[你]** 打 `v0.1.0` tag，观察 GHCR workflow。

---

## 12. 后续扩展（v0.1 之后）

按优先级粗排，**不在首版范围**：

- **LDAP / OIDC 接入**：在 Token Server 里加 external identity provider，用户表仍存但密码委托给外部。
- **Webhook 订阅**：订阅 registry 的 notifications，UI 活动流 + 外部触发器。
- **镜像扫描 (Trivy 等) + 签名 (cosign)**：调外部 CLI，结果写 audit + 展示。
- **指标导出**：Prometheus scrape `registry /metrics` 与 dockery-api 自身指标。
- **镜像保留策略**：自动 GC（按 tag 数、按时间），避免手动触发。
- **多架构 index 可视化**：按 platform 展开显示。
- **镜像 copy / mount**（registry 已支持，补 UI 操作）。

---

## 13. 决策记录（ADR 要点）

这些是讨论过程中确定的关键决策，记录备查：

1. **单镜像一体化** vs 多镜像 compose —— 选前者，品牌定位"轻"。
2. **Node + Hono** vs **Go + Kratos** —— 选 Go，理由：单二进制、JWT 性能、与 registry 同生态、容器体积。
3. **htpasswd (方案 B)** vs **Token (方案 C)** —— 最终选 C：CLI 与 UI 权限统一、天然对接用户系统、未来可平滑接 SSO。
4. **Ed25519** vs RS256 / ES256 —— Ed25519：签名短、性能好、v3.1.0 已修正 JWK thumbprint。
5. **ent** vs sqlc / gorm —— ent：Kratos 社区默认，schema-as-code，edges 表达关系图直观，生成的 client 类型安全；开发期 `Schema.Create` 自动建表省掉迁移工具链。
6. **Kratos proto 拆 6 个文件** vs 一个大 proto —— 跳过 proto，直接 kratoscarf 声明路由（M1.3 ⏭️）。
7. **"/token" 与 "/api/\*" 同进程不同路由组** —— 统一在 dockery-api，nginx 按前缀反代；避免双进程。
8. **UI 不直连 /v2/** —— 一律经 `/api/registry/*` 代理；即使 token 签错也有二次裁剪兜底。
9. **supervisord** vs s6-overlay（实施时换过）—— supervisord 是 Alpine 原生包，减少镜像层；`notification-fd` 语义用"轮询 JWKS 文件是否就绪"200ms × 150 次替代，简化且够用。
10. **Registry 验签用 `jwks`** 而非 `rootcertbundle`（实施时换过）—— distribution v3.1.0 的 `rootcertbundle` 只识别 CERTIFICATE PEM，会静默丢弃 PUBLIC KEY block；改用 RFC 7517 JWK Set，dockery-api 每次启动从私钥派生并原子覆写 `jwt-jwks.json`。
11. **UI session 用不透明 ID + 内存 Store**（实施时换过，取代最初的 HS256 JWT）—— 单节点场景下不透明 ID 更简单、不用管 secret；代价是进程重启全员重登，v0.1 接受。

---

## 14. 开放问题

- [ ] **TLS**：首版通过 nginx 反代，证书挂载路径约定？letsencrypt 自动化是否 M4 就做？
- [ ] **多架构 build**：GHCR workflow 已支持 `linux/amd64,linux/arm64`，Go `CGO_ENABLED=0` + `modernc.org/sqlite` 能覆盖，待验证 arm64 编译通过。
- [ ] **密钥备份提示**：UI 是否在 admin 首登时强制提示"请备份 `/data/config/jwt-private.pem`"？
- [x] **session 存储**：v0.1 选定**内存 Store**（单节点够用，进程重启全体重登）。M4 如需强制下线或 HA，再切 SQLite-backed store。

待 M4 推进时按需拍板。
