# Dockery

`Dockery` 是一个轻量级的 Docker Registry 解决方案，结合了 Docker Distribution、可插拔的认证服务和现代化的 Web UI —— 没有 Harbor 的复杂性。

[English Documentation](./README.md)

## 特性

- ✅ **Docker Registry v3.0.0**：官方 Docker Distribution
- ✅ **现代化 Web UI**：React + TypeScript + shadcn/ui
- ✅ **完整管理功能**：浏览、查看详情、删除镜像
- ✅ **Nginx 代理**：通过单一端口统一访问
- ✅ **灵活配置**：支持环境变量配置
- ✅ **生产就绪**：Docker Compose 一键部署

## 架构说明

### 网络架构
- Web UI 通过 nginx 在 3000 端口提供服务
- Registry API 通过 nginx 的 `/v2/` 路径代理
- 无 CORS 问题，统一域名访问
- Registry 服务在内部 5000 端口运行

### 端口映射
- **Web UI**：外部端口 3000 → 内部端口 80 (nginx)
- **Registry**：内部端口 5000（默认不对外暴露）

## 快速开始

### 前置要求

- Docker Engine 20.10+
- Docker Compose 2.0+

### 1. 配置 Docker Daemon

编辑 Docker 配置（macOS 在 Docker Desktop 设置中）：

```json
{
  "insecure-registries": ["localhost:3000"]
}
```

配置后重启 Docker。

### 2. 启动服务

```bash
# 构建并启动所有服务
docker-compose up -d

# 查看日志
docker-compose logs -f

# 查看特定服务的日志
docker-compose logs -f web-ui
docker-compose logs -f registry
```

### 3. 访问 Web UI

打开浏览器：http://localhost:3000

### 4. 推送镜像到 Registry

```bash
# 标记镜像
docker tag your-image:tag localhost:3000/your-image:tag

# 推送镜像（通过 nginx 代理）
docker push localhost:3000/your-image:tag
```

### 5. 拉取镜像

```bash
# 从 registry 拉取
docker pull localhost:3000/your-image:tag
```

## 管理命令

### 停止服务
```bash
docker-compose down
```

### 停止并删除数据
```bash
docker-compose down -v
```

### 重新构建 UI
```bash
docker-compose build web-ui
docker-compose up -d web-ui
```

### 查看服务状态
```bash
docker-compose ps
```

## 配置说明

### Registry 配置

在 `docker-compose.yaml` 中的环境变量：

```yaml
registry:
  environment:
    REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY: /var/lib/registry
    REGISTRY_STORAGE_DELETE_ENABLED: 'true'
```

### Web UI 配置

UI 使用环境变量进行配置：

```yaml
web-ui:
  environment:
    - REGISTRY_URL=http://registry:5000
```

您可以自定义 registry URL 而无需重新构建镜像。

### Nginx 配置

nginx 配置使用模板和环境变量替换：

- 模板文件：`web-ui/nginx.conf.template`
- 使用 `${REGISTRY_URL}` 进行动态配置
- 容器启动时自动处理

## Web UI 功能

### 镜像管理
- ✅ 浏览所有镜像和标签
- ✅ 查看镜像详情（大小、创建时间、架构）
- ✅ 查看完整的镜像配置（Cmd、Env、Labels、Ports）
- ✅ 查看镜像层历史记录
- ✅ 按标签、大小或创建时间排序
- ✅ 复制 Docker pull 命令
- ✅ 删除镜像标签（带确认对话框）
- ✅ 响应式设计，支持移动端

### 技术细节
- **框架**：React 19 + TypeScript
- **构建工具**：Vite
- **UI 组件**：shadcn/ui + Tailwind CSS
- **状态管理**：Valtio
- **HTTP 服务器**：Nginx (Alpine)

## 故障排查

### UI 无法连接到 Registry

1. 检查服务是否运行：`docker-compose ps`
2. 检查网络连接：`docker network inspect dockery-net`
3. 查看 UI 日志：`docker-compose logs web-ui`
4. 查看 Registry 日志：`docker-compose logs registry`

### 无法推送镜像

1. 确保 Docker daemon 配置了 insecure registry：
   ```json
   {
     "insecure-registries": ["localhost:3000"]
   }
   ```
2. 重启 Docker daemon
3. 验证 registry 可访问：`curl http://localhost:3000/v2/`

### 删除功能不工作

确保 Registry 启用了删除功能（已在 docker-compose.yaml 中配置）：
```yaml
REGISTRY_STORAGE_DELETE_ENABLED: 'true'
```

## 生产环境部署

### 1. 使用 HTTPS

配置 SSL 证书：

```yaml
web-ui:
  ports:
    - "443:443"
  volumes:
    - ./ssl:/etc/nginx/ssl
```

更新 nginx 配置以使用 SSL。

### 2. 添加认证

配置 Registry 认证（基本认证或 Token 认证）。

### 3. 配置存储后端

使用云存储（S3、Azure Blob 等）替代本地文件系统：

```yaml
registry:
  environment:
    REGISTRY_STORAGE: s3
    REGISTRY_STORAGE_S3_BUCKET: your-bucket
    REGISTRY_STORAGE_S3_REGION: us-east-1
```

### 4. 设置资源限制

```yaml
web-ui:
  deploy:
    resources:
      limits:
        cpus: '1'
        memory: 512M
```

## 开发

### 在开发模式下运行 UI

```bash
cd web-ui
pnpm install
pnpm run dev
```

然后访问：http://localhost:5173

### 构建 UI

```bash
cd web-ui
pnpm run build
```

## 环境变量

### Registry

| 变量 | 默认值 | 描述 |
|------|--------|------|
| `REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY` | `/var/lib/registry` | 存储目录 |
| `REGISTRY_STORAGE_DELETE_ENABLED` | `true` | 启用删除操作 |

### Web UI

| 变量 | 默认值 | 描述 |
|------|--------|------|
| `REGISTRY_URL` | `http://registry:5000` | Registry 后端 URL |

## 许可证

详见 [LICENSE](LICENSE) 文件。

## 贡献

欢迎贡献！请随时提交 Pull Request。

## 支持

如果遇到任何问题，请在 GitHub 上创建 issue。
