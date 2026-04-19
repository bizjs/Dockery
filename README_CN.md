# Dockery

自托管 Docker Registry —— **Distribution v3.1.0 + React UI + 账户/权限 + 单镜像**。一个容器跑 registry、API、Web UI，同一个 nginx 端口对外。面向小团队 / 个人开发者。

[English](./README.md) · [部署指南](./docs/deployment.md) · [设计文档](./docs/dockery-design.md)

## 特性

- 📦 推送 / 拉取 / 浏览 OCI + Docker v2 镜像
- 🔐 CLI 与 Web UI 共用账户：三档角色（`admin` / `write` / `view`）+ per-user glob 仓库模式
- 🔑 Ed25519 短命 registry JWT（默认 5 分钟，registry 通过 JWKS 验签）
- 🌐 React 19 UI：登录、角色守卫、用户 & 权限管理、改密
- 🐳 一个镜像、一个端口、SQLite + 文件系统 blob；备份只需 `/data`

**v0.1 不做**：镜像扫描、cosign 签名、复制、多租户、HA、代理缓存。

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

| 变量 | 默认值 | 用途 |
|---|---|---|
| `DOCKERY_ADMIN_USERNAME` | `admin` | 首次启动的 admin 用户名 |
| `DOCKERY_ADMIN_PASSWORD` | _首次启动必填_ | 首次启动的 admin 密码 |
| `REGISTRY_AUTH_TOKEN_REALM` | `http://localhost:5001/token` | Docker CLI 回源 token 的 URL；必须与对外地址一致 |
| `REGISTRY_STORAGE_*` | — | 透传给 distribution，切换存储后端 |

其余项（token TTL、issuer、session cookie 等）在 `docker/rootfs/etc/dockery/config.yaml`；需要定制就挂自己的 `config.yaml` 到 `/etc/dockery/`。

### 持久化

```
/data/
├── registry/          镜像 blob
├── db/dockery.db      SQLite（users / repo_permissions / audit_log）
└── config/
    ├── jwt-private.pem  Ed25519 私钥（0600），单一真源
    └── jwt-jwks.json    由私钥每次启动派生
```

**备份 = `/data` 整包**。丢 `jwt-private.pem` → 签出去的 token 全废；丢 `dockery.db` → 用户表重置。

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

## License

见 [LICENSE](LICENSE)。贡献请先开 issue / discussion。
