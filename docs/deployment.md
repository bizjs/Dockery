# Dockery 部署指南

面向自托管运维者的实战手册。覆盖从 0 到 1 的拉起、配置参考、生产加固、常见运维动作。

> 设计原理与架构细节见 [`dockery-design.md`](./dockery-design.md)。本文只讲**怎么部署和配置**。

## 目录

- [1. 前置准备](#1-前置准备)
- [2. 快速部署(三条命令)](#2-快速部署三条命令)
- [3. 首次启动:会发生什么](#3-首次启动会发生什么)
- [4. 配置参数总表](#4-配置参数总表)
  - [4.1 容器环境变量](#41-容器环境变量)
  - [4.2 dockery-api YAML 配置](#42-dockery-api-yaml-配置)
  - [4.3 distribution registry 配置(REGISTRY_*)](#43-distribution-registry-配置registry_)
  - [4.4 挂载卷与数据布局](#44-挂载卷与数据布局)
- [5. 生产部署加固](#5-生产部署加固)
- [6. 升级与回滚](#6-升级与回滚)
- [7. 运维操作](#7-运维操作)
- [8. 故障排查](#8-故障排查)
- [9. 已知限制 / 未做项](#9-已知限制--未做项)

---

## 1. 前置准备

- **Docker Engine 20.10+** 和 **Docker Compose v2**
- 至少 **512 MB** 可用内存、**5 GB** 可用磁盘(随镜像增长)
- 一个**对外可达的 HTTPS URL**(生产)或 `localhost:5001`(开发)
- 开发模式下,Docker Daemon 需要信任本地 insecure registry:

  ```json
  // Docker Desktop → Settings → Docker Engine
  {
    "insecure-registries": ["localhost:5001"]
  }
  ```

---

## 2. 快速部署(三条命令)

**A. 本地 build + 跑**(适合二次定制):

```bash
git clone https://github.com/bizjs/light-registry.git dockery && cd dockery
export DOCKERY_ADMIN_PASSWORD='change-me-on-first-boot'
docker compose up --build -d
```

**B. 拉 GHCR 镜像**(适合纯部署):

```bash
curl -O https://raw.githubusercontent.com/bizjs/light-registry/main/docker-compose.ghcr.yml
export DOCKERY_ADMIN_PASSWORD='change-me-on-first-boot'
export REGISTRY_AUTH_TOKEN_REALM='https://registry.example.com/token'   # 生产必改
docker compose -f docker-compose.ghcr.yml pull
docker compose -f docker-compose.ghcr.yml up -d
```

**C. 开发模式**(前端跑 Vite、后端跑 `make run`、registry 跑裸容器):

```bash
docker compose -f docker-compose.dev.yaml up -d        # 只起 :5000 的 registry
cd apps/api && make run                                # :5001
cd apps/web-ui && pnpm install && pnpm dev             # :5173
```

访问 http://localhost:5001 — 用 `admin` / 上面设置的密码登录。

---

## 3. 首次启动:会发生什么

dockery-api 进程按顺序做:

1. **读 `/etc/dockery/config.yaml`**(或 `-conf` 指定的目录)
2. **打开 `/data/db/dockery.db`**,`ent.Schema.Create` 自动建表(users / repo_permissions / audit_log)
3. **加载或生成 Ed25519 密钥**
   - 若 `/data/config/jwt-private.pem` 存在:读进来
   - 否则:`ed25519.GenerateKey` 生成,PKCS#8 写 `jwt-private.pem`(0600)
   - **无论新旧**,每次启动都派生 `jwt-jwks.json`(0644),tmp+rename 原子覆写
4. **`EnsureAdmin`**:users 表为空时,用 `DOCKERY_ADMIN_USERNAME` / `DOCKERY_ADMIN_PASSWORD` 建第一个 admin。
   - 没设密码 → **进程 fatal**,不会随机生成也不会跳过
   - 已有用户 → 忽略 env,走正常启动
5. **HTTP 服务器绑 `127.0.0.1:3001`**(容器内)

registry 进程通过 supervisord 的 priority=20 在 api 之后启动,启动前会**轮询 `/data/config/jwt-jwks.json`**(200 ms × 150 次,~30 s 超时)。

nginx 最后起,统一暴露容器 `:5000`(对外 host `:5001`)。

---

## 4. 配置参数总表

### 4.1 容器环境变量

真正最常用的只有 3 个:

| 变量 | 默认 | 场景 | 备注 |
|---|---|---|---|
| `DOCKERY_ADMIN_USERNAME` | `admin` | 首次启动 | 空 DB 才生效,之后忽略 |
| `DOCKERY_ADMIN_PASSWORD` | — | **首次启动必填** | 未设 → api 进程 fatal |
| `REGISTRY_AUTH_TOKEN_REALM` | `http://localhost:5001/token` | 总是要改 | 必须是 docker CLI 实际能访问到的 URL,例 `https://registry.example.com/token` |

> ⚠️ `REGISTRY_AUTH_TOKEN_REALM` 是 **distribution 自身**读的 env(不是 dockery-api 读的)。docker push/pull 拿 401 后会回源到这个 URL 去拿 JWT — 填错就登录失败。

另有一批 distribution 原生的 `REGISTRY_*` 透传规则(见 §4.3)。

### 4.2 dockery-api YAML 配置

容器内位置:`/etc/dockery/config.yaml`(由 `docker/rootfs/etc/dockery/config.yaml` 烘焙到镜像里)。

**覆盖方式**:挂你自己的 config.yaml 到容器:

```yaml
# docker-compose.override.yml
services:
  dockery:
    volumes:
      - ./my-config.yaml:/etc/dockery/config.yaml:ro
```

**完整字段表**:

| 路径 | 类型 | 默认 | 作用 |
|---|---|---|---|
| `server.http.network` | string | `tcp` | 绑定协议 |
| `server.http.addr` | string | `127.0.0.1:3001` | 容器内监听地址(nginx 代理到这里,不要改) |
| `server.http.timeout` | duration | `5s` | HTTP handler 超时 |
| `data.database.driver` | string | `sqlite` | 目前只支持 sqlite(modernc) |
| `data.database.source` | string | 容器: `file:/data/db/dockery.db?cache=shared&_pragma=...` | DSN;pragma 里 WAL 日志模式 + 外键约束必须保留 |
| `dockery.keystore.private_path` | string | `/data/config/jwt-private.pem` | Ed25519 私钥路径,单一真源 |
| `dockery.keystore.jwks_path` | string | `/data/config/jwt-jwks.json` | registry 验签用的 JWKS(由私钥每次启动派生) |
| `dockery.token.issuer` | string | `dockery-api` | **必须**等于 registry `auth.token.issuer` |
| `dockery.token.audience` | string | `dockery` | **必须**等于 registry `auth.token.service` |
| `dockery.token.ttl_seconds` | int | `300` | 每次 `/token` 签发的 JWT 生命期(秒);建议保持 5 分钟 |
| `dockery.token.public_url` | string | `""` | 留空 = 用请求 host;反代下别名不同时显式填 |
| `dockery.admin.username` | string | `admin` | 首启 admin 名(env 优先) |
| `dockery.admin.password` | string | `""` | 首启 admin 密码(env 优先,**不建议写这里**) |
| `dockery.session.ttl_hours` | int | `168` | UI 会话有效期(小时),默认 7 天 |
| `dockery.session.cookie_name` | string | `dockery_session` | HttpOnly cookie 名 |
| `dockery.session.cookie_secure` | bool | `false` | **生产 HTTPS 必须改 true**,否则 cookie 走 HTTP |
| `dockery.gc.supervisorctl_bin` | string | `/usr/bin/supervisorctl` | 只有自定义镜像才需要改 |
| `dockery.gc.supervisord_conf` | string | `/etc/supervisord.conf` | 同上 |
| `dockery.gc.registry_bin` | string | `/usr/local/bin/registry` | 同上 |
| `dockery.gc.registry_conf` | string | `/etc/docker/registry/config.yml` | 同上 |
| `dockery.gc.service_name` | string | `registry` | supervisord `[program:registry]` 的 name |
| `dockery.gc.delete_untagged` | bool | `true` | GC 时顺便清理无 tag 的 manifest(强烈推荐保持 true) |
| `dockery.gc.timeout_seconds` | int | `1800` | 单次 GC 全流程硬超时;超大仓库要调高 |

### 4.3 distribution registry 配置(`REGISTRY_*`)

distribution 有**原生** env 替换机制:`REGISTRY_<A>_<B>_<C>=v` → `config.yml` 中 `a.b.c = v`。容器 yaml 基线在 `docker/rootfs/etc/docker/registry/config.yml`,任何字段都可覆盖。

**最常改的**:

| 变量 | 默认 | 作用 |
|---|---|---|
| `REGISTRY_AUTH_TOKEN_REALM` | `http://localhost:5001/token` | **必改**(见 §4.1) |
| `REGISTRY_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY` | `/data/registry` | 默认 OK;容器外挂 volume |
| `REGISTRY_STORAGE_DELETE_ENABLED` | `true` | 必须为 true,否则 UI 删除不工作 |

**切到对象存储**(示例,S3):

```yaml
environment:
  REGISTRY_STORAGE: s3
  REGISTRY_STORAGE_S3_REGION: us-east-1
  REGISTRY_STORAGE_S3_BUCKET: my-registry
  REGISTRY_STORAGE_S3_ACCESSKEY: AKIA...
  REGISTRY_STORAGE_S3_SECRETKEY: ...
```

切后端后 `/data/registry/` volume 只剩空目录,别急着删 — 想回滚得留着旧 blob。

**⚠️ 不要动这几个**:

| 变量 | 原因 |
|---|---|
| `REGISTRY_HTTP_ADDR` | nginx 固定反代 `127.0.0.1:5001` |
| `REGISTRY_AUTH_TOKEN_ISSUER` | 改就必须同步改 `dockery.token.issuer`,否则验签失败 |
| `REGISTRY_AUTH_TOKEN_SERVICE` | 同上,对应 `dockery.token.audience` |
| `REGISTRY_AUTH_TOKEN_JWKS` | 固定 `/data/config/jwt-jwks.json`,和 keystore 输出路径绑死 |

### 4.4 挂载卷与数据布局

**一个 volume,一个备份点**:

```
/data/
├── registry/              镜像 blob(filesystem driver 下)
├── db/
│   └── dockery.db         SQLite 主库 — users / repo_permissions / audit_log
└── config/
    ├── jwt-private.pem    Ed25519 私钥(0600)— 单一真源
    └── jwt-jwks.json      派生的 JWKS(每次启动覆写)
```

**备份 = `/data/` 整包打 tar**。任何时候只要保住:
- `db/dockery.db` → 用户、权限、审计日志都还在
- `config/jwt-private.pem` → 已签出去的 token 仍然能验(虽然 5 分钟就过期,但避免用户被迫重登)
- `registry/` → 镜像本身

没 `jwt-private.pem` 就等于 registry 的信任根丢了 — 所有在途 token 作废,用户被迫全员重新 `docker login`。

---

## 5. 生产部署加固

按清单走,每一项都是必做:

### 5.1 TLS 终端

Dockery 容器内**不做 TLS**(v0.1 没集成 Let's Encrypt)。外面放个反代:

```nginx
# /etc/nginx/sites-enabled/dockery
server {
    listen 443 ssl http2;
    server_name registry.example.com;

    ssl_certificate     /etc/letsencrypt/live/registry.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/registry.example.com/privkey.pem;

    # 镜像可能很大 — 取消 body 限制 + 关 buffering
    client_max_body_size 0;
    proxy_request_buffering off;
    proxy_buffering off;
    proxy_http_version 1.1;

    location / {
        proxy_pass http://127.0.0.1:5001;
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 900s;
    }
}

server {
    listen 80;
    server_name registry.example.com;
    return 301 https://$host$request_uri;
}
```

对应的 docker-compose 覆盖:

```yaml
services:
  dockery:
    environment:
      REGISTRY_AUTH_TOKEN_REALM: https://registry.example.com/token
    # 把容器 :5000 只绑本地,别暴露到公网
    ports: !reset []
    ports:
      - "127.0.0.1:5001:5000"
    volumes:
      - ./cookie-secure.yaml:/etc/dockery/config.yaml:ro
```

`cookie-secure.yaml` 至少包含:

```yaml
dockery:
  session:
    cookie_secure: true     # ★ 生产强制
    cookie_name: dockery_session
    ttl_hours: 168
```

### 5.2 日志 & 监控

- **registry 日志**:`REGISTRY_LOG_LEVEL=info` 足够;排错时临时切 `debug`
- **Prometheus**:registry 已经在 `127.0.0.1:5002/metrics` 暴露(我们写在 `config.yml` 里);如果要外采,加一行 nginx 反代:

  ```nginx
  location = /metrics {
      allow 10.0.0.0/8;
      deny all;
      proxy_pass http://127.0.0.1:5002/metrics;
  }
  ```

- **dockery-api 日志**:kratos stdlog 走 stdout,`docker logs dockery` 看就行。结构化 JSON 目前没加。

### 5.3 备份

最低频率:**每日 cron**:

```bash
# /etc/cron.daily/dockery-backup
#!/bin/sh
set -e
SNAP=/var/backups/dockery-$(date +%F).tgz
docker exec dockery sqlite3 /data/db/dockery.db '.backup /tmp/db.bak'   # 在线安全备份
docker run --rm -v dockery_dockery-data:/data -v /var/backups:/out \
  alpine tar czf /out/dockery-$(date +%F).tgz -C /data .
```

测过才算有备份 — 每月跑一次 restore 演练。

### 5.4 资源限制

```yaml
services:
  dockery:
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
        reservations:
          memory: 256M
```

---

## 6. 升级与回滚

**升级**:

```bash
docker compose -f docker-compose.ghcr.yml pull
docker compose -f docker-compose.ghcr.yml up -d    # 重建 dockery 容器
```

SQLite schema 会自动 migrate(`ent.Schema.Create` 幂等)。密钥持久化在 volume,升级不丢。

**回滚**:

```bash
export DOCKERY_IMAGE=ghcr.io/bizjs/light-registry:v0.1.0
docker compose -f docker-compose.ghcr.yml up -d
```

`DOCKERY_IMAGE` 就是 `docker-compose.ghcr.yml` 里用来拉镜像的变量,不指定就是 `:latest`。**生产请锁版本**(`:v0.1.x`),不要跟 `:latest`。

**schema 降级**:ent auto-migrate 只加不删,回滚老版本通常没问题。但新版本若加了非空字段,回滚后会卡在 "column not found"。出现这种断层会在 release note 里明确标注,届时 restore 备份再启老版本。

---

## 7. 运维操作

### 7.1 用户与权限

**Web UI**(管理员菜单 → Manage users):创建 / 改角色 / 改密 / 禁用 / 删除 / 管理 repo 模式。

**CLI**(无 HTTP 即可用):

```bash
docker exec -it dockery dockery-api -conf /etc/dockery user list
docker exec -it dockery dockery-api -conf /etc/dockery user create alice write
docker exec -it dockery dockery-api -conf /etc/dockery user grant  alice 'alice/*,shared/app'
docker exec -it dockery dockery-api -conf /etc/dockery user passwd alice
docker exec -it dockery dockery-api -conf /etc/dockery user revoke 42       # permission id
docker exec -it dockery dockery-api -conf /etc/dockery user delete alice
```

系统会拒绝:
- 删或降最后一个 admin
- 删自己、禁用自己

### 7.2 垃圾回收

**Web UI**(管理员菜单 → Maintenance)点按钮即可。流程:
1. 设维护 flag,UI 的镜像删除立刻返 503
2. `supervisorctl stop registry`
3. `registry garbage-collect --delete-untagged /etc/docker/registry/config.yml`
4. `supervisorctl start registry`
5. 清 flag,写 `gc.completed` 审计

期间 docker push/pull 会**失败**,因为 registry 本身停了 — 挑低谷时段跑。

超大仓库可调高 `dockery.gc.timeout_seconds`(默认 1800)。

### 7.3 审计日志

Web UI(管理员菜单 → Audit log)按 actor / action / 时间段过滤,可展开看 JSON detail。覆盖 17 种动作:token / auth / user / permission / image / gc。

HTTP 直接查也行:

```bash
curl -b cookie.txt 'https://registry.example.com/api/audit?action=token.denied&limit=50'
```

---

## 8. 故障排查

| 症状 | 原因 / 排查 |
|---|---|
| `docker login` 超时 | `REGISTRY_AUTH_TOKEN_REALM` 不是 CLI 能访问的 URL;检查 DNS / 防火墙 |
| `docker push` 401 `invalid_token` | `dockery.token.issuer` / `audience` 和 registry `auth.token.issuer` / `service` 不一致;**或** `jwt-jwks.json` 丢了(重启 api 会自动重建) |
| 启动后 registry 进程一直 "RESTARTING" | jwt-jwks.json 30 s 内没出现 → 看 `docker logs dockery | grep dockery-api`,十有八九 DB 打不开(volume 权限)或 admin 密码没设 |
| UI 登录后立刻又被踢回登录页 | `cookie_secure: true` 但反代没 `X-Forwarded-Proto https` → 浏览器不回传 cookie |
| 删镜像 UI 返 503 "registry is in maintenance" | 有 GC 在跑,等它结束(Maintenance 页可以看状态) |
| GC 提示 "already in progress" | 上一次 GC 卡住了;`docker exec dockery supervisorctl status` 看 registry 是不是 STOPPED,手动 `start registry` 后 volume 里删掉 `/run/dockery-gc.lock`(若存在)再重试 |
| SQLite "database is locked" | 并发大 / volume 在网络盘上;确认 volume 是本地磁盘,不要放 NFS |
| 首次启动 fatal `admin password required` | 设 `DOCKERY_ADMIN_PASSWORD` 再启动 |

查日志:

```bash
docker logs dockery                           # 全部
docker logs dockery | grep dockery-api        # 后端
docker logs dockery | grep registry           # upstream
docker exec dockery supervisorctl status      # 三个进程状态
```

---

## 9. 已知限制 / 未做项

v0.1 明确不做:

- ❌ 内置 TLS / Let's Encrypt 自动证书 — 请在前面放 nginx/Caddy/traefik
- ❌ 密钥轮换 UI(`/api/admin/rotate-signing-key` 后端是 stub) — 要轮换目前只能手动删 `jwt-private.pem` 后重启容器,这会让所有已签 token 立即失效,用户重登(正式的轮换端点在 M4 路线图)
- ❌ 镜像扫描 / cosign 签名 / 复制 / 多租户 / HA — 不在 v0.1 范围
- ❌ Session 持久化 — 目前进程内存 store,**dockery-api 重启 = 所有 UI 用户被强制重登**(docker CLI 不受影响,token 是独立的)
- ⚠️ `REGISTRY_STORAGE_MAINTENANCE_READONLY` 我们不用 — GC 改走 supervisorctl 停 registry 更彻底

如果你的场景卡在上面某条,开 issue 讨论优先级。
