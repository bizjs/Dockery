# Dockery:一个容器跑起来,就是你的私有 Docker Registry

自己维护内网 Docker Registry 的人大概都经历过这个区间:一边嫌 `distribution/distribution` 裸跑太原始 —— 没 UI、没账户、htpasswd 所有人共用一把钥匙;一边嫌 Harbor 过重 —— 官方 `docker-compose.yml` 拉起十几个容器,给一个五人小组用配置成本远大于收益。**Dockery 就是为这中间地带做的**:一个镜像,一个端口,跑起来就是完整的私有仓库 —— 能 push/pull,有网页,能按人分权限,不依赖 Postgres / Redis / 任何外部服务。

## 横向对比

| | 裸 `distribution` | Harbor | **Dockery** |
|---|:---:|:---:|:---:|
| Web 管理界面 | 无 | 有 | **有** |
| 账户 / 角色 | htpasswd(平权) | LDAP / OIDC / SSO 全家桶 | **三档角色 + glob 仓库模式** |
| 需要起的容器数 | 1 | 10+ | **1** |
| 外部依赖 | 无 | Postgres + Redis + 多组件 | **无** |
| 备份 | 一个目录 | 多库多卷 | **一个 `/data` 目录** |
| 多架构镜像 | 自己打 | 官方提供 | **amd64 + arm64 官方** |
| 从零到可用 | 装完还得自研 UI | 半天起 | **几分钟** |

## 对用户来说,实际体验是这样

**有个网页。** 浏览器打开就看到所有仓库、每个仓库的 tag、每个 tag 的大小和层信息。不用再 `curl /v2/_catalog` 然后自己串 JSON。

**账号是账号,不是一张 htpasswd。** admin 在 Manage Users 页面里点一下就能新建账号、分配角色,给 `write` / `view` 用户发放像 `team-a/*,shared/app` 这样的 glob 仓库模式 —— 精细到项目组,不再"要么全给、要么不给"。

**UI 与 CLI 共用同一套授权。** 某个用户在网页里看不到的仓库,他用 `docker pull` 也拉不到。没有"UI 过滤了但 CLI 能直连"的错位。

**改密不用登机器。** UI 里改、或者一条 `docker exec ... dockery-api user passwd alice` 就行,不必 SSH 上去改 htpasswd 再 reload。

**搬家就 tar 一下。** 所有状态 —— 镜像 blob、用户表、审计日志、JWT 私钥 —— 都在 `/data` 下。整包 tar 出来,换台机器解压启动,全部带过去。

**ARM 机器能跑。** 官方镜像同时出 `linux/amd64` 和 `linux/arm64`,Apple Silicon 服务器、Raspberry Pi、各家云的 Graviton 实例都直接拉。

## 装起来是真的一条命令

```bash
docker run -d --name dockery --restart unless-stopped \
  -p 5000:5000 \
  -v /srv/dockery:/data \
  -e DOCKERY_ADMIN_PASSWORD='change-me' \
  -e REGISTRY_AUTH_TOKEN_REALM='http://registry.example.com:5000/token' \
  ghcr.io/bizjs/dockery:latest
```

浏览器开 `http://registry.example.com:5000`,用 `admin / change-me` 登录,新建账号,然后在客户端:

```bash
docker login registry.example.com:5000
docker tag myapp registry.example.com:5000/team-a/myapp:0.1.0
docker push registry.example.com:5000/team-a/myapp:0.1.0
```

纯 HTTP 场景要把 `registry.example.com:5000` 加进客户端 `daemon.json` 的 `insecure-registries`;前面挂 Caddy / Traefik 做 TLS 终结就不用这步。

## 技术选型里几个关键决定

- **registry 协议用官方 `distribution/distribution` v3.1.0**,不是自己写一份兼容实现;协议可靠性上限和社区一致。
- **账户模块是 Go + SQLite**(纯 Go 驱动,零 CGO),单一静态二进制,没 daemon 要起,备份只是一个 `.db` 文件。
- **Docker CLI 认证走 Ed25519 + JWKS 的短命 JWT(5 分钟)**,registry 在线验签。相比"共享 htpasswd"的做法,新增或吊销用户不需要重启 registry,签发记录还能落审计。

完整设计见 [`docs/dockery-design.md`](../dockery-design.md)。

## 不做什么

v0.1 明确不做的东西:镜像扫描、cosign 验签、跨机复制、HA、多租户。每一项都会把"单容器 + 本地盘"这个边界打破 —— 需要这些请上 Harbor,Dockery 不跟它抢场景。

## 适合的使用场景

- 3~30 人内部/项目组 registry
- CI/CD 产物仓,镜像不出内网
- 家庭实验室、单人开发者自托管
- 边缘节点、ARM 小机的镜像缓存


## 链接

- GitHub:<https://github.com/bizjs/Dockery>
- 镜像:`ghcr.io/bizjs/dockery` ([GHCR 页面](https://github.com/bizjs/Dockery/pkgs/container/dockery))
- 设计文档:[`docs/dockery-design.md`](../dockery-design.md)
- 部署参考:[`docs/deployment.md`](../deployment.md)

## 致谢

- [`distribution/distribution`](https://github.com/distribution/distribution) —— 内嵌的 registry 协议实现。
- [`joxit/docker-registry-ui`](https://github.com/joxit/docker-registry-ui) —— UI 交互形态的参考来源,Dockery 的前端基于 React 19 重写,跑在自家带鉴权的 `/api/registry/*` 接口之上。
