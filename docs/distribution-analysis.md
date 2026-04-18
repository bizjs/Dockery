# Distribution Registry 分析（面向 Dockery 设计）

本文不是官方文档的翻译，而是从 **Dockery（单镜像一体化 registry + webUI + 简单权限）** 的设计视角，对 CNCF Distribution（`distribution/distribution:3.1.0`，2026-04-06 发布，下文简称 Registry）做的一次"需要知道的事情"汇编。所有结论以 v3.1.0 源码为优先依据，文档次之。

> 一手源码与文档若冲突，以源码为准 —— 已验证 `registry/auth/htpasswd/access.go@v3.1.0` 的实际行为与官方文档描述不一致（见下文"htpasswd 热加载"）。

## 0. v3.0.0 → v3.1.0 变更对 Dockery 的影响

3.1.0 是 3.0 系列的首个特性版本，以下几条对 Dockery 的设计直接相关：

| 变更 | PR | 对 Dockery 的影响 |
|---|---|---|
| Tag list 原生分页（`n=` / `last=`） | #4353, #4360 | UI 的 TagList 页可以真正按页拉取，告别 `n=1000` 的兜底大值 |
| Ed25519 JWK thumbprint 正确实现（RFC 8037） | — | **对 Dockery 的 token 方案直接利好**：签名密钥用 Ed25519，thumbprint 算对才能让 registry 的 `kid` 匹配走通 |
| ECDSA thumbprint X/Y 修正 | — | 若备选 ES256 算法则受益；Dockery 首选 Ed25519，次要关心 |
| htpasswd 鉴权加固（dummy-hash 防时序攻击） | #4758 | Dockery 不用 htpasswd 驱动，该 PR 对我们无直接影响；仅作知识储备 |
| env 映射 YAML inline-struct 修复 | #4639 | 以前某些嵌套段用环境变量覆盖可能失效；Dockery 自己维护 config.yml 为主，用户用 env 叠加覆盖也更可靠 |
| CVE-2026-35172、CVE-2026-33540 | — | 直接给升级理由；Dockery 基线锁 3.1.0 |
| Redis TLS 可不带客户端证书、Azure 默认凭据修复 | #4770, #4619 | 不影响默认配置，只在高级用户启用云后端时有感 |
| 要求 Go 1.25.7 | — | 只影响编译 registry；我们用官方镜像，透明 |

结论：**Dockery 基线升到 `distribution/distribution:3.1.0`**，顺便把 `docker-compose.yaml` / `docker-compose.ghcr.yml` 里的 `3.0.0` 一起替换。

---

## 1. 定位与能力边界

Registry **是什么**：一个按 OCI/Docker 分发协议对外提供 push/pull 的内容寻址存储服务。它把 blob（layer/manifest/config）按 sha256 digest 存到后端（文件系统、S3、Azure、GCS…），并对外提供 HTTP API。

Registry **不是什么**：
- 没有用户管理 API（没有 `POST /users`）。认证委托给 htpasswd 文件或外部 token server。
- 没有 RBAC；htpasswd 模式下只有"通过 vs 不通过"两态。
- 没有 Web UI。
- 没有运行时配置热重载（**除了 htpasswd 文件**，见 §4.2）。改 `config.yml` 必须重启。
- 不负责镜像生命周期管理（保留策略、自动清理）。GC 要手动触发且要先进入 read-only。

这直接决定了 Dockery 的分工：**Registry 管 blob 存储和 /v2/ 协议，并把鉴权完全委托给 Token Server；Dockery API 兼任 Token Server + Web 管理后台 + UI 代理层，成为 Dockery 整个权限体系唯一的真相来源。**

---

## 2. HTTP API v2 端点速查

UI 和 Dockery API 都会用到的协议层面：

| 分组 | 方法 | 路径 | 用途 |
|---|---|---|---|
| base | GET | `/v2/` | 探活 + 触发 401 以驱动认证流程 |
| catalog | GET | `/v2/_catalog?n=&last=` | 仓库列表（分页） |
| tags | GET | `/v2/<name>/tags/list?n=&last=` | 标签列表（分页） |
| manifests | GET/HEAD | `/v2/<name>/manifests/<reference>` | 读 manifest；`HEAD` 拿 `Docker-Content-Digest` |
| manifests | PUT | `/v2/<name>/manifests/<reference>` | 推送 manifest |
| manifests | DELETE | `/v2/<name>/manifests/<digest>` | **只能按 digest 删**，不能按 tag |
| blobs | GET/HEAD | `/v2/<name>/blobs/<digest>` | 读 layer/config |
| blobs | DELETE | `/v2/<name>/blobs/<digest>` | 删 blob |
| uploads | POST/PATCH/PUT | `/v2/<name>/blobs/uploads/...` | 分块上传流程 |

### 关键细节

- **删除 tag 的正确姿势**：`HEAD /manifests/<tag>` 读取响应头 `Docker-Content-Digest`，然后 `DELETE /manifests/<digest>`。这是目前 `web-ui/src/lib/registry-client/RegistryClient.ts` 里 `deleteTag()` 已经实现的两步走。
- **Accept 头必须带 OCI + Docker v2 一揽子**，否则 manifest 可能被转换为老格式或 406：
  ```
  application/vnd.oci.image.manifest.v1+json,
  application/vnd.docker.distribution.manifest.v2+json,
  application/vnd.docker.distribution.manifest.list.v2+json,
  application/vnd.oci.image.index.v1+json
  ```
- **分页**：`?n=<page_size>&last=<last_name>`，响应若还有更多会带 `Link: <...>; rel="next"` 头。**v3.1.0 起 tag list 原生支持分页**（PR #4353/#4360），UI 的 TagList 页应切换到真正的分页实现，不再靠 `n=1000` 的兜底大值。Catalog 在 v3.0 就已支持分页。
- **Docker-Content-Digest 响应头**：manifest/blob 的内容寻址 digest。UI 的 `cache-request.ts` 基于它做条件缓存。

---

## 3. 配置系统

### 3.1 两种配置方式（可混用）

1. **挂载 `config.yml`**：`-v $(pwd)/config.yml:/etc/distribution/config.yml`
2. **环境变量**：`REGISTRY_<SECTION>_<SUBSECTION>_<FIELD>`，大小写不敏感，下划线表示嵌套

Dockery 采用**策略 1 为主**：镜像内置一份 `/etc/docker/registry/config.yml`（或 `/etc/distribution/config.yml`），由 Dockery 自己维护；把不该暴露给用户的配置写死，把可调项通过 env 叠加覆盖。

### 3.2 对 Dockery 相关的配置段

| 段 | Dockery 里的处理 |
|---|---|
| `version: 0.1` | 固定 |
| `log` | 透传给 stdout/stderr，由 s6 的 logger 捕获 |
| `storage.filesystem.rootdirectory` | 固定为 `/data/registry` |
| `storage.delete.enabled: true` | **必须开**，否则 UI 的删除、以及 GC 前置步骤都用不了 |
| `storage.maintenance.uploadpurging` | 默认 168h（1 周）清未完成上传，保持默认即可 |
| `storage.maintenance.readonly.enabled` | 给 GC 模式用的运行时开关，由 Dockery API 在触发 GC 时临时改写 + 重启 registry 进程 |
| `storage.cache.blobdescriptor: inmemory` | 默认开 inmemory 即可；不引入 Redis |
| `auth.token.realm` | `http://<host>:5000/token`（nginx 反代到 Dockery API 的 `/token` 端点） |
| `auth.token.service` | 固定 `dockery`（供 JWT `aud` 校验） |
| `auth.token.issuer` | 固定 `dockery-api`（供 JWT `iss` 校验） |
| `auth.token.rootcertbundle` | `/data/config/jwt-public.pem`，Dockery API 启动时写出 |
| `auth.token.signingalgorithms` | `[EdDSA]`（Ed25519，最现代、最短签名） |
| `http.addr: 127.0.0.1:5001` | 只监听 loopback，外部走 nginx :5000 |
| `http.secret` | Dockery 首次启动生成随机值写到 `/data/config/registry-secret`，保持稳定 |
| `http.headers` | 让 nginx 统一处理，registry 这层少做 |
| `http.debug.addr: 127.0.0.1:5002` | 内部健康检查与 Prometheus 用 |
| `validation.manifests.urls.allow` | 目前不启用；若将来要限制 `urls`，要写 regex 白名单，否则 push 会失败 |

### 3.3 环境变量命名坑点

- `REGISTRY_HTTP_TLS_LETSENCRYPT_HOSTS_0=foo` —— 数组靠 `_<index>` 表示，Dockery 的 compose 示例文档要写清楚，别让用户踩。
- **v3.1.0 起**（PR #4639）修复了"env 变量到 YAML inline-struct 的映射"，早期版本某些嵌套路径用环境变量覆盖会静默失败。因此 Dockery 推荐**基线升到 3.1.0** 以让 env 覆盖行为稳定可预期。

---

## 4. 认证与授权（Dockery 的核心依赖）

Registry v3 支持 3 种 auth，**全局只能选一**。Dockery 选 **token**，由 Dockery API 自己担任 Token Server，与用户/角色/权限系统天然一体。

### 4.1 方案选择

最初考虑过三档：

| 模式 | 粒度 | 外部依赖 | Dockery 采用 |
|---|---|---|---|
| `silly` | 只检查 `Authorization` 头存在与否 | 无 | 否（仅开发用） |
| `htpasswd` | 登录即全权限（CLI 层面无法按 repo 限权） | htpasswd 文件 | 否 |
| `token` | per-repo per-action RBAC，scope 在 JWT 里 | 自建 Token Server | **是** |

选 `token` 的理由：
1. **CLI 与 UI 使用同一套权限模型**：不再有"UI 层角色"和"CLI 层全权"的割裂，`docker push/pull` 也被 per-repo 权限约束。
2. **Dockery API 反正要存在**（做 web 后台、UI 代理、GC 触发），多担一个 `/token` endpoint 成本可控。
3. **扩展性**：未来要接 LDAP/OIDC/GitHub SSO 只需在 Token Server 这一层加适配器，registry 无感知。
4. **审计更干净**：所有鉴权请求都进 Dockery API 一个入口，便于记录 who-did-what。
5. **无需再关心 htpasswd 的"幽灵 docker 用户"坑**：不使用 htpasswd 驱动，`createHtpasswdFile()` 那条代码路径根本不走。

### 4.2 Token 协议（三方流程）

```
 docker CLI                 Registry(:5001)           Dockery API(:3001)
                                                       (同时是 Token Server)
      │ ①GET /v2/foo/manifests/latest                │
      ├────────────────────────►                     │
      │      ②401 WWW-Authenticate: Bearer          │
      │         realm="http://host:5000/token",      │
      │         service="dockery",                   │
      │         scope="repository:foo:pull"          │
      │◄────────────────────────                     │
      │                                               │
      │ ③GET /token?service=dockery&scope=...        │
      │   Authorization: Basic base64(user:pass)     │
      ├────────────────────────────────────────────────►
      │                           ④200 {"token":"<JWT>", "expires_in":300}
      │◄────────────────────────────────────────────────
      │                                               │
      │ ⑤GET /v2/foo/manifests/latest                │
      │   Authorization: Bearer <JWT>                │
      ├────────────────────────►                     │
      │     ⑥验签（公钥）+ 校验 access claim → 200   │
      │◄────────────────────────                     │
```

`WWW-Authenticate` 头格式：
```
Bearer realm="http://host:5000/token", service="dockery", scope="repository:foo:pull"
```
- `realm`：Token Server URL，nginx 把 `/token` 路由到 Dockery API 的 `:3001/token`
- `service`：`dockery`（必须与 JWT `aud` 匹配）
- `scope`：registry 根据 HTTP method + 请求路径推断出来填进去

### 4.3 Dockery 作为 Token Server

**Token endpoint**：

```
GET /token?service=dockery&scope=<scope>[&scope=<scope>...]&offline_token=<bool>
Authorization: Basic <base64(user:pass)>
```

处理逻辑：
1. 解 Basic Auth → SQLite `users` 表 → bcrypt 比对密码 → 失败返 401；
2. 解析每个 scope（`repository:name:actions`），查 `permissions` 表得该用户在该 repo 上的实际 actions 集合；
3. **求交集**：client 请求的 actions ∩ 用户实际拥有的 actions = 授予的 actions；
4. 拼装 JWT `access` claim；
5. Ed25519 签名；
6. 返回。

返回体：
```json
{
  "token": "<JWT>",
  "access_token": "<JWT>",
  "expires_in": 300,
  "issued_at": "2026-04-18T10:00:00Z"
}
```

**约定**：即便请求的 actions 完全不被授予，也返回一个 `access: []` 的 token，不返 403。Registry 在 ⑥ 拿到后发现没权限再抛 401 给 CLI。这是 spec 要求的行为，避免 token server 泄露"这个 repo 存在但你没权"。

### 4.4 JWT 格式

**Header**（Ed25519）：
```json
{"typ":"JWT","alg":"EdDSA","kid":"<JWK thumbprint per RFC 8037>"}
```

**Payload**：
```json
{
  "iss": "dockery-api",
  "sub": "alice",
  "aud": "dockery",
  "exp": 1745020800,
  "nbf": 1745020500,
  "iat": 1745020500,
  "jti": "<random 128-bit>",
  "access": [
    {"type":"repository","name":"alice/my-app","actions":["pull","push"]},
    {"type":"repository","name":"team-b/shared","actions":["pull"]}
  ]
}
```

**TTL 约定**：`exp - iat = 300s`（5 分钟）。这是"够完成一个大镜像 push"与"作废后快速失效"的平衡。过期后 docker CLI 自动再次走 ②~⑤ 流程，对用户无感。

### 4.5 Scope 与资源类型

| 示例 scope | 含义 |
|---|---|
| `repository:alice/app:pull` | 拉取 alice/app |
| `repository:alice/app:pull,push` | 拉取 + 推送 |
| `repository:alice/app:delete` | 删 manifest / blob |
| `repository:alice/app:*` | alice/app 上任意操作 |
| `registry:catalog:*` | 列 `_catalog` 等跨仓库操作（仅 admin） |

### 4.6 SQLite Schema（与 Token Server 集成）

```sql
-- 用户
users (
  id         INTEGER PRIMARY KEY,
  username   TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,          -- bcrypt
  role       TEXT NOT NULL,             -- 'admin' | 'user'
  created_at INTEGER,
  disabled   BOOLEAN DEFAULT 0
)

-- 仓库权限（admin 不进这张表，默认全通）
repo_permissions (
  id         INTEGER PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  repo_pattern TEXT NOT NULL,           -- 支持 'alice/*' 通配
  actions    TEXT NOT NULL,             -- CSV: 'pull,push' 或 '*'
  UNIQUE(user_id, repo_pattern)
)

-- 审计（可选 M3 实装）
audit_log (
  id         INTEGER PRIMARY KEY,
  ts         INTEGER NOT NULL,
  actor      TEXT NOT NULL,
  action     TEXT NOT NULL,             -- 'token.issued', 'user.created', ...
  target     TEXT,                      -- 'repository:alice/app'
  scope      TEXT,                      -- 授予的 actions
  client_ip  TEXT
)
```

**权限匹配算法**：给定用户 + scope，遍历该用户的 `repo_permissions`，用 glob 匹配 `repo_pattern`，取所有命中行 actions 的并集。admin 直接返回请求的所有 actions。

### 4.7 密钥管理

- **算法**：Ed25519（v3.1.0 已修正的 RFC 8037 thumbprint 实现；短密钥、快验签、现代）。
- **生成**：Dockery API 首次启动时生成一对 Ed25519 密钥，私钥写 `/data/config/jwt-private.pem`（mode 0600），公钥写 `/data/config/jwt-public.pem`。Registry `auth.token.rootcertbundle` 读后者。
- **轮换**：密钥轮换需**重启 registry 进程**才能加载新公钥（registry 启动时一次性加载，不热更新）。Dockery 提供 `/api/admin/rotate-signing-key` endpoint：生成新密钥对 → 宽限期内双签（registry 只认一把公钥，所以实际做法是写新公钥文件 + `s6-svc -r registry` 重启 + 旧 token 会失效）。MVP 可不做轮换 UI。
- **备份**：`/data/config/jwt-private.pem` 是整个权限系统的根凭据，用户的备份/迁移脚本必须包含 `/data/` 整个目录。

### 4.8 UI → Registry 的权限注入

UI 后端（属于 Dockery API 内部）代理 UI 的 `/api/registry/*` 请求到 upstream registry：

1. UI 带 session cookie 请求 `/api/registry/catalog`；
2. API 验证 session → 查当前用户角色；
3. API 为这个内部请求生成一个**短命 JWT**（TTL=30s），`access` 按用户实际权限填；
4. API 带 `Authorization: Bearer <JWT>` 发往 `127.0.0.1:5001/v2/_catalog`；
5. Registry 验签 + 校验 → 返回；
6. API 转发响应给 UI。

这样做的好处：
- UI 不需要存用户明文密码；
- 权限逻辑只在一个地方（权限匹配函数同时被 `/token` 和内部代理调用）；
- UI 看到的数据自动被当前用户权限裁剪（catalog 只列可见的 repo）。

### 4.9 启动顺序与 s6 依赖

```
[Dockery API 启动]
  ↓ SQLite migrate
  ↓ 若 users 空：从 env DOCKERY_ADMIN_USER/DOCKERY_ADMIN_PASS 创建 admin
  ↓ 若签名密钥缺失：生成 Ed25519 密钥对，写 /data/config/jwt-{private,public}.pem
  ↓ 监听 :3001，暴露 /token、/.well-known/jwks.json、/api/*
  ↓ s6 notify ready
[Registry 启动]
  ↓ 读 config.yml 的 auth.token
  ↓ 从 /data/config/jwt-public.pem 加载公钥
  ↓ 监听 :5001
[Nginx 启动]
  ↓ 路由 /token、/api/* → :3001；/v2/* → :5001；/ → dist
```

**关键依赖**：registry 启动时必须能读到 `jwt-public.pem`，所以 API 必须先起并写好密钥文件，s6 用 `notification-fd` 让 registry 等 API ready。

### 4.10 htpasswd 热加载 —— 作为背景知识保留

方案变为 token 后，Dockery **不再使用** htpasswd auth 驱动。以下内容仅作技术档案留存（比如将来想降级到 htpasswd、或对比 registry 两种 auth 实现细节时参考）：

> v3.1.0 源码 `registry/auth/htpasswd/access.go` 的 `Authorized()` 方法每次请求都 `os.Stat` 检查 htpasswd 文件 mtime，变了就重读。这意味着 htpasswd 驱动**支持运行时热加载，无需重启**，与官方文档描述"加载一次需重启"相矛盾。v3.1.0 的 PR #4758 还加入了 dummy-hash 时序攻击防御。这部分能力在 token 模式下用不到，但是一手源码结论。

---

## 5. 存储

### 5.1 驱动选择

| 驱动 | 适用 | Dockery 的态度 |
|---|---|---|
| `filesystem` | 单机、小规模 | **默认**，根目录 `/data/registry` |
| `s3` | 云/大规模 | 通过 env 暴露给高级用户（需配 `forcepathstyle` 以兼容 MinIO 等） |
| `azure` / `gcs` | 云 | 文档提一下，不做默认预设 |
| `inmemory` | 测试 | 不用 |

### 5.2 删除开关

```yaml
delete:
  enabled: true
```

必开。否则：
- UI 的"删除 tag"按钮返回 405。
- GC 的 sweep 阶段无法实际清除。

### 5.3 上传清理

```yaml
maintenance:
  uploadpurging:
    enabled: true
    age: 168h
    interval: 24h
```

默认即合理：一周内没完成的分块上传会被清。不需要 Dockery 干预。

---

## 6. 垃圾回收（GC）

这是设计一体化镜像时**最需要当心的点**，因为 GC 要求 registry 进入 read-only 或停机。

### 6.1 命令

```bash
bin/registry garbage-collect [--dry-run] [--delete-untagged] [--quiet] /etc/docker/registry/config.yml
```

两阶段：
- **Mark**：扫所有 manifest，收集应保留的 digest 集。
- **Sweep**：扫所有 blob，删不在 mark 集合里的。

### 6.2 安全约束

> 文档原话："garbage collection is a stop-the-world operation"

如果 sweep 期间有 push 进来，**正在上传的新镜像的 layer 可能被误删**，导致 corrupt image。因此必须二选一：
- 停 registry 进程；
- 或 config 加 `maintenance.readonly.enabled: true` 并重启（不能热切换）。

### 6.3 Dockery 的 GC 设计

因为 registry 没有运行时 read-only 切换，Dockery 要做的是：

1. UI 管理页提供"触发 GC"按钮（admin-only）。
2. API 收到请求 →
   - 改写一份临时 config 开启 readonly；
   - s6 `s6-svc -r` 重启 registry 进程（走进 readonly）；
   - `exec` 运行 `registry garbage-collect /etc/docker/registry/config.yml`；
   - 恢复原 config，再次 `s6-svc -r` 重启 registry。
3. 过程中 UI 显示"维护中"横幅；API 对写操作返回 503。
4. 首版可以简化为"GC 期间 API 拒绝 docker push"，不开 readonly，直接停 registry + 跑 GC + 重启。简单可靠。

`--delete-untagged` 会把所有没打 tag 的 manifest 视为垃圾 —— 注意它会影响用 digest 引用的工作流；首版 Dockery **不默认开这个标志**。

---

## 7. 通知（Webhook）

Registry 能把 push/pull/delete 事件 POST 到 HTTP endpoint。

```yaml
notifications:
  endpoints:
    - name: dockery
      url: http://127.0.0.1:3001/api/internal/registry-events
      timeout: 1s
      threshold: 5
      backoff: 1s
```

### 对 Dockery 的价值

- UI 上"最近活动"、"镜像更新时间"这类数据，如果定时扫 catalog 会慢；**订阅事件是更优雅的做法**。
- API 收到 push 事件后可以：
  - 更新 SQLite 里 `images` 缓存表（repo、tag、digest、pushed_at、pushed_by）；
  - 触发"新镜像"通知推给在线 UI（SSE）。

### 可靠性约束（必须知道）

- **事件队列在进程内存**。registry 进程重启会丢事件。
- 投递"至少一次"，失败超过 threshold 会退避重试。
- 对 Dockery：**不要把事件作为唯一事实来源**。API 依旧要有兜底的定时扫 catalog 对账机制，或者在 UI 请求某个仓库时触发一次按需对齐。

---

## 8. 健康检查

Registry 自身提供（启用 debug HTTP 后）：
- `/debug/health` — 内置 health 聚合结果
- `/debug/vars` — expvar（含通知队列深度）
- `/metrics` — Prometheus（若 `debug.prometheus.enabled: true`）

Dockery 的 s6 readiness：
- `api`：HTTP GET `127.0.0.1:3001/healthz` 返回 200。
- `registry`：HTTP GET `127.0.0.1:5001/v2/` 返回 200 或 401（401 说明 auth 工作正常）。
- `nginx`：HTTP GET `127.0.0.1:5000/healthz` → nginx 配 `return 200;`。

容器级健康（Dockerfile `HEALTHCHECK`）：打 nginx 那个 `/healthz`，三个进程只要 nginx 通就认为整体通（因为 nginx 就是入口）。

---

## 9. 代理模式（pull-through cache）

`proxy.remoteurl` 可把 registry 变成上游 registry（如 Docker Hub）的只读缓存。**Dockery 不启用此模式** —— 它与"自托管私有 registry"的定位冲突（缓存模式不支持 push）。在文档里注明就好。

---

## 10. Manifest 兼容性

- Registry v3 原生支持：**OCI image manifest v1**、**OCI image index v1**、**Docker distribution manifest v2**、**Docker manifest list v2**。
- Docker schema 1 仍可读，但**新 push 应只用 v2/OCI**，schema 1 是历史包袱。
- 按 tag 拉取时老 Docker 客户端可能触发 schema 1 转换；按 digest 拉取不会转换。
- OCI Artifacts / Referrers API：v3 已支持 referrers 相关端点。Dockery 首版**不需要对外暴露**这部分能力，但 UI 在显示 manifest 类型时要正确识别 `application/vnd.oci.image.index.v1+json` 等 mediaType。

---

## 11. HTTP/TLS 策略

Dockery 选择**不在 registry 这层做 TLS**，把 TLS 全部收敛到 nginx（或用户的反代）：

- 容器内部 registry 只监听 `127.0.0.1:5001`，plain HTTP。
- Nginx 在 5000 上可选 TLS（未来启用），对外给 docker CLI。
- 好处：证书轮换只需重载 nginx；registry 不参与 cert 管理；letsencrypt 也走 nginx 的常规姿势。
- `http.secret` 仍要配置（哪怕单节点），会影响 URL 签名，Dockery 首次启动生成并持久化到 `/data/config/registry-secret`。

---

## 12. 对 Dockery 的最终启示汇总

| 需求 | Distribution 给了什么 | Dockery 要做什么 |
|---|---|---|
| 账户存储 | 仅 htpasswd 或外置 token server | SQLite 作 SSOT，Dockery API 自身担任 Token Server |
| 账户热更新 | Token TTL=300s，用户改动下次签发即生效 | 密码/权限改动实时，已签发 JWT 在 exp 内短暂仍有效 |
| 细粒度权限 | Token + JWT `access` 数组原生支持 per-repo RBAC | SQLite `repo_permissions` 表，scope 求交集 |
| 初始 admin | 无内置机制 | API 首启从 env 创建 admin，同时生成 Ed25519 密钥对 |
| 签名密钥 | `auth.token.rootcertbundle` 文件路径，**启动时加载一次** | 启动前由 API 写出公钥；轮换需重启 registry 进程 |
| UI 权限一致性 | 若 UI 直连 /v2/ 就绕过 token | UI 一律经 Dockery API 代理，API 为每次请求注入短命 JWT |
| 删除镜像 | `delete.enabled: true` + 两步删（HEAD 取 digest → DELETE by digest） | 已实现；记得 `REGISTRY_STORAGE_DELETE_ENABLED=true` 默认打开；JWT scope 要含 `delete` |
| GC | 命令行 + 必须 readonly 或停机 | API 封装"维护模式"流程，重启 registry 进程执行 |
| 事件通知 | 内存队列、at-least-once、会丢 | 订阅加速活动流，不作为唯一事实来源，仍要对账 |
| 多架构 | OCI index / Docker list 原生支持 | UI 能识别 index 类型，按 platform 分别显示 |
| TLS | registry 自身可做，但不灵活 | 收敛到 nginx |
| 外部配置 | yaml + env 叠加 | 内置固定 yaml，暴露少数 env 给用户调 |

---

## 13. 参考

- [CNCF Distribution — About](https://distribution.github.io/distribution/about/)
- [Configuring a registry](https://distribution.github.io/distribution/about/configuration/)
- [Deploying a registry](https://distribution.github.io/distribution/about/deploying/)
- [Garbage collection](https://distribution.github.io/distribution/about/garbage-collection/)
- [Notifications](https://distribution.github.io/distribution/about/notifications/)
- [Compatibility](https://distribution.github.io/distribution/about/compatibility/)
- [HTTP API v2 spec](https://distribution.github.io/distribution/spec/api/)
- [Token authentication specification](https://distribution.github.io/distribution/spec/auth/token/)
- [Token authentication implementation (JWT)](https://distribution.github.io/distribution/spec/auth/jwt/)
- [Token scope and access](https://distribution.github.io/distribution/spec/auth/scope/)
- 源码（v3.1.0）：[`registry/auth/htpasswd/access.go`](https://github.com/distribution/distribution/blob/v3.1.0/registry/auth/htpasswd/access.go)（背景知识）
- 源码（v3.1.0）：[`registry/auth/token/`](https://github.com/distribution/distribution/tree/v3.1.0/registry/auth/token)（Dockery 采用）
- [v3.1.0 Release Notes](https://github.com/distribution/distribution/releases/tag/v3.1.0)
