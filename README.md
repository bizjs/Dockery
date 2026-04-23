# Dockery

自托管 Docker Registry —— **Distribution v3.1.0 + React UI + 账户/权限 + 单镜像**。一个容器跑 registry、API、Web UI，同一个 nginx 端口对外。面向小团队 / 个人开发者。

[English](./README_EN.md) · [部署指南](./docs/deployment.md) · [设计文档](./docs/dockery-design.md) · [变更日志](./CHANGELOG.md)

## 特性

- 📦 推送 / 拉取 / 浏览 OCI + Docker v2 镜像
- 🔐 CLI 与 Web UI 共用账户：三档角色（`admin` / `write` / `view`）+ per-user glob 仓库模式
- 🔑 Ed25519 短命 registry JWT（默认 5 分钟，registry 通过 JWKS 验签）
- 🌐 React 19 UI：登录、角色守卫、用户 & 权限管理、改密
- 🐳 一个镜像、一个端口、SQLite + 文件系统 blob；备份只需 `/data`

**v0.1 不做**：镜像扫描、cosign 签名、复制、多租户、HA、代理缓存。

## 界面预览

<table>
<tr>
<td colspan="2" valign="top" align="center"><img src="./docs/screenshots/02-catalog.png" alt="Catalog 首页" /><br/><sub>首页:所有仓库一览,支持搜索</sub></td>
</tr>
<tr>
<td width="50%" valign="top"><img src="./docs/screenshots/03-tag-list.png" alt="Tag 列表" /><br/><sub>Tag 列表:排序、批量多选、多架构、分页</sub></td>
<td width="50%" valign="top"><img src="./docs/screenshots/04-tag-detail.png" alt="Tag 详情" /><br/><sub>Tag 详情抽屉:pull 命令、config、layer 历史、多架构 digest</sub></td>
</tr>
<tr>
<td valign="top"><img src="./docs/screenshots/05-admin-users.png" alt="用户管理" /><br/><sub>用户管理:角色、启停、per-user 仓库 glob 模式</sub></td>
<td valign="top"><img src="./docs/screenshots/06-admin-maintenance.png" alt="维护 / GC" /><br/><sub>维护:触发 Garbage Collection,回收悬空 blob</sub></td>
</tr>
<tr>
<td valign="top"><img src="./docs/screenshots/07-admin-audit.png" alt="审计日志" /><br/><sub>审计日志:登录 / 令牌 / 管理操作,可按 actor / action / 时间过滤</sub></td>
<td valign="top"><img src="./docs/screenshots/01-login.png" alt="登录" /><br/><sub>登录页</sub></td>
</tr>
</table>

## 快速开始

```bash
# Docker Desktop → 设置 → Docker Engine: "insecure-registries": ["localhost:5001"]
DOCKERY_ADMIN_PASSWORD='change-me' docker compose up --build -d
open http://localhost:5001      # 用 admin / change-me 登录
```

`DOCKERY_ADMIN_PASSWORD` 只在 `/data` 首次初始化时生效。TLS 请自挂反向代理（M4 会内置）。

### 推送镜像

```bash
docker login localhost:5001
docker tag hello-world localhost:5001/demo/hello:1
docker push localhost:5001/demo/hello:1
```

## 正式部署

生产环境直接拉预构建镜像 —— [`ghcr.io/bizjs/dockery`](https://github.com/bizjs/Dockery/pkgs/container/dockery),不要从源码构建。**生产必须钉具体版本号**,别用滚动的 `:latest`。TLS 请自挂反向代理(nginx / Caddy / Traefik);未启用 TLS 时,docker 客户端要把 `localhost:5001`(或实际地址)加进 `insecure-registries`。

### 方式 A —— `docker run`

```bash
docker run -d \
  --name dockery \
  --restart unless-stopped \
  -p 5001:5000 \
  -v /srv/dockery:/data \
  -e DOCKERY_ADMIN_PASSWORD='change-me-on-first-boot' \
  -e REGISTRY_AUTH_TOKEN_REALM='https://registry.example.com/token' \
  ghcr.io/bizjs/dockery:0.1.0
```

`-v /srv/dockery:/data` 把宿主目录 bind-mount 到容器,生产推荐(好备份)。快速尝试用 named volume(`-v dockery-data:/data`)也行。详见 [存储](#存储--data常用)。

### 方式 B —— `docker compose`

用仓库里的 [`docker-compose.ghcr.yml`](./docker-compose.ghcr.yml):

```bash
export DOCKERY_ADMIN_PASSWORD='change-me-on-first-boot'
export REGISTRY_AUTH_TOKEN_REALM='https://registry.example.com/token'
export DOCKERY_IMAGE='ghcr.io/bizjs/dockery:0.1.0'   # 钉版本

docker compose -f docker-compose.ghcr.yml pull
docker compose -f docker-compose.ghcr.yml up -d
```

首次启动会用 `DOCKERY_ADMIN_PASSWORD` 创建 admin,之后这个环境变量被忽略(改密去 UI 或用 `dockery-api user passwd`)。

## 用户与权限管理

**Web UI（管理员菜单 → Manage users）** —— 创建用户、改角色、改密、启用/禁用、删除，以及通过 permissions 抽屉给 `write` / `view` 用户添加/编辑/撤销仓库模式。`view` 角色登录后不会看到删除按钮。所有用户都可在头像菜单里自助改密。

**CLI 备用**（无需启动 HTTP 服务）：

```bash
docker exec -it dockery dockery-api -conf /etc/dockery user list
docker exec -it dockery dockery-api -conf /etc/dockery user create alice write
docker exec -it dockery dockery-api -conf /etc/dockery user grant  alice 'alice/*,shared/app'
docker exec -it dockery dockery-api -conf /etc/dockery user passwd alice
docker exec -it dockery dockery-api -conf /etc/dockery user revoke 42     # permission id
docker exec -it dockery dockery-api -conf /etc/dockery user delete alice
```

系统会拒绝删除或降级最后一个 admin。

## 配置

### 环境变量

按级别分组：**必须**(否则启动不了)、**常用**(生产几乎都要改)、**其他**(高级透传项)。

**必须**

| 变量 | 说明 |
|---|---|
| `DOCKERY_ADMIN_PASSWORD` | 首次启动的 admin 密码。`/data` 为空时必填,之后忽略。空 DB + 未设 → api 故意 fatal(不随机生成密码,避免日志泄漏)。 |

**常用**(生产)

| 变量 | 默认值 | 说明 |
|---|---|---|
| `REGISTRY_AUTH_TOKEN_REALM` | `http://localhost:5001/token` | distribution 在 `WWW-Authenticate` 里告诉 docker CLI 去哪拿 JWT。**必须是 docker CLI 实际能访问到的 URL**,例 `https://registry.example.com/token`。填错 → `docker push` 401。 |
| `DOCKERY_ADMIN_USERNAME` | `admin` | 首次启动的 admin 用户名。users 表为空才生效。 |
| `DOCKERY_IMAGE` *(仅 compose)* | `ghcr.io/bizjs/dockery:latest` | 固定镜像 tag,例如 `ghcr.io/bizjs/dockery:0.1.0`。 |

**其他**(高级)

| 变量 | 说明 |
|---|---|
| `REGISTRY_STORAGE_*` | 原样透传给 distribution,切 S3 / OSS / Azure 等存储后端。见 [distribution 配置文档](https://distribution.github.io/distribution/about/configuration/)。 |
| 其他 `REGISTRY_*` | `REGISTRY_<SECTION>_<FIELD>` 都会被 distribution 消费(日志级别、HTTP header 等)。 |
| `DOCKERY_OTEL_ENDPOINT` | OTLP/HTTP 遥测端点,例如 `http://jaeger:4318`。**默认不设,遥测关闭**。distribution v3 默认会往 `localhost:4318` 刷 traces,不开收集器就会持续打 `connection refused`;Dockerfile 因此固定 `OTEL_SDK_DISABLED=true`,只有设置本变量时才打开并把 `OTEL_EXPORTER_OTLP_ENDPOINT` 指过去。 |

其余项(token TTL、issuer、session cookie 等)在 `docker/rootfs/etc/dockery/config.yaml`(打到镜像里);需要定制就挂自己的 `config.yaml` 到 `/etc/dockery/`。

### 存储 —— `/data`(常用)

所有持久化状态都在容器的 `/data` 下。生产环境**推荐用宿主目录 bind-mount**,备份和巡检就是文件系统操作:

```bash
-v /srv/dockery:/data      # docker run
```

```yaml
# docker-compose.*.yml —— 替换掉 named volume
volumes:
  - /srv/dockery:/data
```

命名 volume(快速体验示例里用的)单机也完全够用,但宿主路径更好备份、快照、迁移。

`/data` 结构:

```
/data/
├── registry/          镜像 blob(默认 filesystem driver)
├── db/dockery.db      SQLite(users / repo_permissions / audit_log)
└── config/
    ├── jwt-private.pem  Ed25519 私钥(0600),单一真源
    └── jwt-jwks.json    每次启动由私钥派生
```

**备份 = `/data` 整包**。丢 `jwt-private.pem` → 签出去的 token 全废;丢 `dockery.db` → 用户表重置。

> 用 `REGISTRY_STORAGE_*` 切 S3 / OSS / Azure 只会搬 `registry/`,`db/` 和 `config/` 仍然需要挂 `/data`。

## 架构

```
           外部 :5001 (host → :5000 container)
                       │
                   [ nginx ]
    ┌────────────┬────────┬────────────┐
    │            │        │            │
   / 静态     /token   /api/*        /v2/*
    │            │        │            │
 web-ui    dockery-api :3001   distribution :5001
                 │                    ▲
                 ├── SQLite           │
                 ├── jwt-private.pem  │
                 └── jwt-jwks.json ───┘  registry 用 JWKS 验签
```

容器内三个进程由 supervisord 编排。完整设计见 [`docs/dockery-design.md`](./docs/dockery-design.md)。

## 本地开发

```bash
# 前端（:5173）
cd apps/web-ui && pnpm install && pnpm dev

# 后端（:5001）
cd apps/api && make run

# 裸 registry（:5000），给前端 /v2 代理用
docker run -p 5000:5000 distribution/distribution:3.1.0
```

## 发布

打 `v*` tag → GitHub Actions 构建并推送 `ghcr.io/<owner>/<repo>:<version>` 与 `:latest`（linux/amd64 + linux/arm64）。

## 致谢

- [`distribution/distribution`](https://github.com/distribution/distribution) —— Dockery 内嵌的 registry 协议实现。
- [`joxit/docker-registry-ui`](https://github.com/joxit/docker-registry-ui) —— 设计 Dockery 的 catalog 与 tag 页面时参考了它的交互形态(布局、体积格式化、manifest 详情流)。Dockery 基于 React 19 完全重写,跑在自家带鉴权的 `/api/registry/*` 接口之上。

## License

见 [LICENSE](LICENSE)。贡献请先开 issue / discussion。
