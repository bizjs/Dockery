# Changelog

本文件记录 Dockery 的所有重要变更。格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/),版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [0.7.3] - 2026-04-25

### 修复

- **repo_meta**: use semver ordering in pickRepresentativeTag


## [0.7.2] - 2026-04-25

### 新增

- **catalog**: default sort to newest-first (updated desc)


## [0.7.1] - 2026-04-24

### 新增

- **gc**: resync repo_meta cache after garbage collection


## [0.7.0] - 2026-04-23

### 修复

- use biz-utils copyText for HTTP-localhost clipboard fallback

### 变更

- extract catalog fetch into registryfetch.Client


## [0.6.0] - 2026-04-23

### 变更

- extract registryfetch package, deduplicate registry HTTP logic
- **catalog**: replace client-side fan-out with server-side overview API

### 新增

- **catalog**: add repo_meta cache design and VM tests
- configurable reconciler interval and pkg→util scope rename
- **catalog**: add refresh button and debounce search input
- harden pull tracking, parallel child fetch, and shutdown
- add repo_meta catalog cache with webhooks and reconciler
- optimize UI


## [0.5.0] - 2026-04-23

### 修复

- OTEL_SDK_DISABLED not set

### 新增

- update UI


## [0.4.0] - 2026-04-22

### 文档

- add CHANGELOG.md and link it from both READMEs

### 新增

- **ci**: add git-cliff changelog automation on tag push
- unrestricted access when no repo patterns; enrich manifest sizes
## [0.2.0] - 2026-04-20


## [0.3.0] - 2026-04-21

### 新增
- 中英文 README 增加 `界面预览 / Screenshots` 区,包含 catalog、tag 列表、tag 详情、用户管理、维护 / GC、审计日志、登录共 7 张截图。

### 修复
- Tag 列表表头复选框与 body 水平 / 垂直未对齐:统一所有 `TableHead` 为 `px-4`,排序按钮改用 `-ml-3 h-8 px-3` 让文本与 body 对齐;body 复选框单元格补 `translate-y-[2px]`,与 shadcn `TableHead` 默认行为一致。

## [0.2.0] - 2026-04-20

### 新增
- **多架构 manifest list 支持**:tag 列表 Architecture 列在 multi-arch tag 上显示 `linux/amd64, linux/arm64`(3+ 平台折叠为 `首个 +N more`,hover 展开全部);tag 详情抽屉新增 Platforms 区,列出每个 platform 的 os/arch/variant、digest、size,并提供逐项 digest 复制。此前 multi-arch tag 在列表中会显示为 0 byte / 空 arch。
- **Tag 列表分页**:支持 25 / 50 / 100 / 200 行每页,首页 / 上一页 / 下一页 / 尾页控件;切换排序或 pageSize 时自动回到第 1 页。
- **批量删除 tag**:多选 checkbox,支持 Shift+Click 范围选择、"本页全选"、Clear;顺序删除并显示进度,失败的 tag 保留选中以便重试。
- **tag 名自然排序**:`v10` 现在排在 `v2` 之后(`localeCompare` 启用 `numeric: true`),替代原先的字典序。
- **生产部署说明**:中英文 README 增加 `docker run` / `docker compose` 示例,环境变量按 必须 / 常用 / 其他 三级分组,`/data` 存储章节强调 bind-mount。
- **推广博客** `docs/blogs/introducing-dockery.md`,及 V2EX 推广文案。
- **致谢区**(Acknowledgments):`distribution/distribution`(registry 协议实现)、`joxit/docker-registry-ui`(UX 参考)。

### 变更
- `README.md` 切换为中文主版本,英文版迁移到 `README_EN.md`。
- `docker-compose.ghcr.yml` 镜像名修正为 `ghcr.io/bizjs/dockery:latest`。
- 多架构 Docker 构建改用 `--platform=$BUILDPLATFORM` + `GOOS`/`GOARCH` 交叉编译,不再走 QEMU 模拟。
- Tag 列表 Created 列固定 24 小时制、宽度 200px。

## [0.1.0] - 2026-04-20

首个公开版本。单镜像 Docker Registry —— registry、API、Web UI 共住一个容器,同一个 nginx 端口对外。

### 新增
- **单容器部署**:nginx + supervisord 编排 `dockery-api`(:3001)、`distribution` v3.1.0(:5001)、nginx(:5000);三进程启停与健康就绪顺序由 supervisord 管。
- **Ed25519 JWT token 鉴权**:dockery-api 签,registry 通过 JWKS 验;docker CLI `WWW-Authenticate` → `/token`(Basic Auth)→ 短命 JWT → retry `/v2/`。token TTL / issuer 在 config.yaml 里可改。
- **账户与权限模型**:`users` + `repo_permissions` 存 SQLite,角色 `admin` / `write` / `view`,后两者支持 per-user glob 仓库模式;CLI 与 Web UI 共用同一套。
- **React 19 + Tailwind v4 Web UI**:登录、session cookie 鉴权、路由守卫、自助改密、仓库 catalog、tag 列表、tag 详情抽屉。
- **管理员页面**:用户管理(增删改、角色切换、启停、permission 授权抽屉)、审计日志(登录 / 令牌 / 管理操作,可按 actor / action / 时间过滤)、维护(触发 registry GC)。
- **空仓库自动清理**:删除最后一个 tag 后自动清理仓库目录。
- **一次性 CLI**(`dockery-api user list|create|grant|passwd|revoke|delete`):不启动 HTTP 服务也能管账号;拒绝删除或降级最后一个 admin。
- **多架构镜像发布**:`ghcr.io/bizjs/dockery`,`linux/amd64` + `linux/arm64`;`v*` tag 触发 GitHub Actions 构建 + SLSA 构建来源证明(build provenance attestations)。
- UI bundle 烤入 `APP_VERSION`,页脚显示版本号。

[Unreleased]: https://github.com/bizjs/Dockery/compare/v0.3.0...HEAD
[0.2.0]: https://github.com/bizjs/Dockery/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/bizjs/Dockery/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/bizjs/Dockery/releases/tag/v0.1.0
