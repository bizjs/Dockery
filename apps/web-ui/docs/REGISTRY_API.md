# Docker Registry API v2 参考文档

本文档梳理了 Docker Registry 3.0.0 支持的所有 API 接口。

## 基础信息

- **API 版本**: v2（目前唯一的 API 版本）
- **Registry 软件版本**: 3.0.0
- **基础路径**: `/v2`
- **协议**: HTTP/HTTPS

> **注意**: Docker Registry 只有 v2 API，没有 v3 API。Registry 的版本号（如 3.0.0）指的是软件本身的版本，而不是 API 版本。v2 API 规范自 2015 年发布以来一直保持稳定，并持续在新版本的 Registry 软件中使用。

## API 端点列表

> **图例说明**:
> - 🔵 **UI 必需**: Dockery 必须使用的核心接口
> - 🟢 **UI 可选**: UI 可以使用但非必需的接口
> - ⚪ **UI 不用**: UI 通常不需要使用的接口（主要用于镜像推送）

### 1. 版本检查 🟢

检查 Registry API 版本。

**UI 用途**: 可用于健康检查和验证 Registry 是否可访问

**端点**: `GET /v2/`

**响应**:
```json
{}
```

**响应头**:
- `Docker-Distribution-Api-Version: registry/2.0`

**状态码**:
- `200 OK`: API 可用
- `401 Unauthorized`: 需要认证

---

### 2. 获取仓库列表 🔵

列出 Registry 中所有可用的仓库。

**UI 用途**: 在首页展示所有可用的镜像仓库列表（Catalog 页面）

**端点**: `GET /v2/_catalog`

**查询参数**:
- `n` (可选): 返回的最大仓库数量
- `last` (可选): 分页标记，从指定仓库名之后开始返回

**请求示例**:
```bash
curl http://localhost:4999/v2/_catalog
curl http://localhost:4999/v2/_catalog?n=10
curl http://localhost:4999/v2/_catalog?n=10&last=myrepo
```

**响应**:
```json
{
  "repositories": [
    "distribution",
    "myapp",
    "nginx"
  ]
}
```

**状态码**:
- `200 OK`: 成功

---

### 3. 获取标签列表 🔵

列出指定仓库的所有标签。

**UI 用途**: 显示某个镜像仓库的所有版本标签（TagList 页面）

**端点**: `GET /v2/<name>/tags/list`

**路径参数**:
- `<name>`: 仓库名称（可以包含斜杠，如 `library/nginx`）

**查询参数**:
- `n` (可选): 返回的最大标签数量
- `last` (可选): 分页标记

**请求示例**:
```bash
curl http://localhost:4999/v2/distribution/tags/list
```

**响应**:
```json
{
  "name": "distribution",
  "tags": [
    "latest",
    "3.0.0",
    "2.8.3"
  ]
}
```

**状态码**:
- `200 OK`: 成功
- `404 Not Found`: 仓库不存在

---

### 4. 获取 Manifest 🔵

获取镜像的 manifest 信息。

**UI 用途**: 
- 获取镜像的 digest、大小等元数据
- 获取 config blob 的 digest 以便进一步获取详细信息
- 显示镜像的层信息

**端点**: `GET /v2/<name>/manifests/<reference>`

**路径参数**:
- `<name>`: 仓库名称
- `<reference>`: 标签名或 digest

**请求头**:
- `Accept`: 指定 manifest 格式
  - `application/vnd.docker.distribution.manifest.v2+json` - Docker Manifest v2
  - `application/vnd.docker.distribution.manifest.list.v2+json` - Manifest List
  - `application/vnd.oci.image.manifest.v1+json` - OCI Image Manifest
  - `application/vnd.oci.image.index.v1+json` - OCI Image Index

**请求示例**:
```bash
# 获取 Docker Manifest v2
curl -H "Accept: application/vnd.docker.distribution.manifest.v2+json" \
  http://localhost:4999/v2/distribution/manifests/3.0.0

# 获取 OCI Manifest
curl -H "Accept: application/vnd.oci.image.manifest.v1+json" \
  http://localhost:4999/v2/distribution/manifests/3.0.0
```

**响应示例** (OCI Manifest):
```json
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "config": {
    "mediaType": "application/vnd.oci.image.config.v1+json",
    "size": 1234,
    "digest": "sha256:abc123..."
  },
  "layers": [
    {
      "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
      "size": 5678,
      "digest": "sha256:def456..."
    }
  ]
}
```

**响应头**:
- `Docker-Content-Digest`: Manifest 的 digest
- `Content-Type`: Manifest 的媒体类型

**状态码**:
- `200 OK`: 成功
- `404 Not Found`: Manifest 不存在

---

### 5. 上传 Manifest ⚪

上传或更新镜像的 manifest。

**UI 用途**: UI 通常不需要此功能（用于 docker push）

**端点**: `PUT /v2/<name>/manifests/<reference>`

**路径参数**:
- `<name>`: 仓库名称
- `<reference>`: 标签名

**请求头**:
- `Content-Type`: Manifest 的媒体类型

**请求体**: Manifest JSON

**响应头**:
- `Docker-Content-Digest`: 上传的 manifest digest
- `Location`: Manifest 的 URL

**状态码**:
- `201 Created`: 成功创建
- `400 Bad Request`: 无效的 manifest
- `401 Unauthorized`: 未授权

---

### 6. 删除 Manifest 🟢

删除指定的 manifest。

**UI 用途**: 可选功能，用于删除镜像标签

**端点**: `DELETE /v2/<name>/manifests/<digest>`

**路径参数**:
- `<name>`: 仓库名称
- `<digest>`: Manifest 的 digest（必须是 digest，不能是标签）

**请求示例**:
```bash
curl -X DELETE http://localhost:4999/v2/distribution/manifests/sha256:abc123...
```

**状态码**:
- `202 Accepted`: 删除请求已接受
- `404 Not Found`: Manifest 不存在
- `405 Method Not Allowed`: Registry 不支持删除操作

---

### 7. 获取 Blob 🔵

下载镜像层或配置 blob。

**UI 用途**: 
- 获取 config blob 以显示镜像的创建时间、架构等详细信息
- 可选：下载镜像层用于分析

**端点**: `GET /v2/<name>/blobs/<digest>`

**路径参数**:
- `<name>`: 仓库名称
- `<digest>`: Blob 的 digest

**请求示例**:
```bash
curl http://localhost:4999/v2/distribution/blobs/sha256:abc123...
```

**响应**: Blob 的二进制内容

**响应头**:
- `Content-Length`: Blob 大小
- `Docker-Content-Digest`: Blob 的 digest

**状态码**:
- `200 OK`: 成功
- `307 Temporary Redirect`: 重定向到实际的 blob 位置
- `404 Not Found`: Blob 不存在

---

### 8. 检查 Blob 是否存在 🟢

检查 blob 是否存在而不下载内容。

**UI 用途**: 可选，用于验证 blob 是否存在

**端点**: `HEAD /v2/<name>/blobs/<digest>`

**路径参数**:
- `<name>`: 仓库名称
- `<digest>`: Blob 的 digest

**响应头**:
- `Content-Length`: Blob 大小
- `Docker-Content-Digest`: Blob 的 digest

**状态码**:
- `200 OK`: Blob 存在
- `404 Not Found`: Blob 不存在

---

### 9. 删除 Blob ⚪

删除指定的 blob。

**UI 用途**: UI 通常不需要此功能

**端点**: `DELETE /v2/<name>/blobs/<digest>`

**路径参数**:
- `<name>`: 仓库名称
- `<digest>`: Blob 的 digest

**状态码**:
- `202 Accepted`: 删除请求已接受
- `404 Not Found`: Blob 不存在
- `405 Method Not Allowed`: Registry 不支持删除操作

---

### 10. 初始化 Blob 上传 ⚪

开始一个新的 blob 上传会话。

**UI 用途**: UI 通常不需要此功能（用于 docker push）

**端点**: `POST /v2/<name>/blobs/uploads/`

**路径参数**:
- `<name>`: 仓库名称

**查询参数**:
- `digest` (可选): 如果提供，执行单次上传

**响应头**:
- `Location`: 上传会话的 URL
- `Docker-Upload-UUID`: 上传会话 ID
- `Range`: 已上传的字节范围

**状态码**:
- `202 Accepted`: 上传会话已创建
- `201 Created`: 单次上传成功（当提供 digest 时）

---

### 11. 上传 Blob 数据块 ⚪

上传 blob 的数据块。

**UI 用途**: UI 通常不需要此功能（用于 docker push）

**端点**: `PATCH /v2/<name>/blobs/uploads/<uuid>`

**路径参数**:
- `<name>`: 仓库名称
- `<uuid>`: 上传会话 ID

**请求头**:
- `Content-Type: application/octet-stream`
- `Content-Length`: 数据块大小
- `Content-Range` (可选): 字节范围

**请求体**: 二进制数据

**响应头**:
- `Location`: 上传会话的 URL
- `Range`: 已上传的字节范围
- `Docker-Upload-UUID`: 上传会话 ID

**状态码**:
- `202 Accepted`: 数据块已接受

---

### 12. 完成 Blob 上传 ⚪

完成 blob 上传并提交。

**UI 用途**: UI 通常不需要此功能（用于 docker push）

**端点**: `PUT /v2/<name>/blobs/uploads/<uuid>`

**路径参数**:
- `<name>`: 仓库名称
- `<uuid>`: 上传会话 ID

**查询参数**:
- `digest`: Blob 的 digest（必需）

**请求头**:
- `Content-Length`: 最后数据块的大小（如果有）

**响应头**:
- `Location`: Blob 的 URL
- `Docker-Content-Digest`: Blob 的 digest

**状态码**:
- `201 Created`: Blob 上传成功
- `400 Bad Request`: Digest 不匹配

---

### 13. 取消 Blob 上传 ⚪

取消正在进行的上传会话。

**UI 用途**: UI 通常不需要此功能（用于 docker push）

**端点**: `DELETE /v2/<name>/blobs/uploads/<uuid>`

**路径参数**:
- `<name>`: 仓库名称
- `<uuid>`: 上传会话 ID

**状态码**:
- `204 No Content`: 上传会话已取消

---

### 14. 获取上传状态 ⚪

查询上传会话的状态。

**UI 用途**: UI 通常不需要此功能（用于 docker push）

**端点**: `GET /v2/<name>/blobs/uploads/<uuid>`

**路径参数**:
- `<name>`: 仓库名称
- `<uuid>`: 上传会话 ID

**响应头**:
- `Range`: 已上传的字节范围
- `Docker-Upload-UUID`: 上传会话 ID

**状态码**:
- `204 No Content`: 上传会话存在

---

## Dockery 使用的接口总结

### 必需接口 (🔵)

1. **GET /v2/_catalog** - 获取仓库列表
2. **GET /v2/<name>/tags/list** - 获取标签列表
3. **GET /v2/<name>/manifests/<reference>** - 获取 Manifest
4. **GET /v2/<name>/blobs/<digest>** - 获取 Blob（主要用于获取 config）

### 可选接口 (🟢)

1. **GET /v2/** - 版本检查/健康检查
2. **DELETE /v2/<name>/manifests/<digest>** - 删除镜像
3. **HEAD /v2/<name>/blobs/<digest>** - 检查 Blob 是否存在

### 不使用的接口 (⚪)

1. **PUT /v2/<name>/manifests/<reference>** - 上传 Manifest
2. **DELETE /v2/<name>/blobs/<digest>** - 删除 Blob
3. **POST /v2/<name>/blobs/uploads/** - 初始化上传
4. **PATCH /v2/<name>/blobs/uploads/<uuid>** - 上传数据块
5. **PUT /v2/<name>/blobs/uploads/<uuid>** - 完成上传
6. **DELETE /v2/<name>/blobs/uploads/<uuid>** - 取消上传
7. **GET /v2/<name>/blobs/uploads/<uuid>** - 获取上传状态

---

## 错误响应格式

所有错误响应都遵循以下格式：

```json
{
  "errors": [
    {
      "code": "ERROR_CODE",
      "message": "Error description",
      "detail": "Additional details"
    }
  ]
}
```

### 常见错误码

- `BLOB_UNKNOWN`: Blob 不存在
- `BLOB_UPLOAD_INVALID`: Blob 上传无效
- `BLOB_UPLOAD_UNKNOWN`: 上传会话不存在
- `DIGEST_INVALID`: Digest 格式无效
- `MANIFEST_BLOB_UNKNOWN`: Manifest 引用的 blob 不存在
- `MANIFEST_INVALID`: Manifest 格式无效
- `MANIFEST_UNKNOWN`: Manifest 不存在
- `MANIFEST_UNVERIFIED`: Manifest 未验证
- `NAME_INVALID`: 仓库名称无效
- `NAME_UNKNOWN`: 仓库不存在
- `SIZE_INVALID`: 大小无效
- `TAG_INVALID`: 标签名称无效
- `UNAUTHORIZED`: 未授权
- `DENIED`: 权限被拒绝
- `UNSUPPORTED`: 不支持的操作

---

## 认证

Registry 支持以下认证方式：

### 1. Basic Authentication

```bash
curl -u username:password http://localhost:4999/v2/_catalog
```

### 2. Bearer Token Authentication

当收到 401 响应时，检查 `WWW-Authenticate` 响应头：

```
WWW-Authenticate: Bearer realm="https://auth.example.com/token",service="registry.example.com",scope="repository:myrepo:pull,push"
```

然后向 realm 指定的 URL 请求 token：

```bash
curl "https://auth.example.com/token?service=registry.example.com&scope=repository:myrepo:pull,push"
```

响应：
```json
{
  "token": "eyJhbGc...",
  "access_token": "eyJhbGc...",
  "expires_in": 300
}
```

使用 token 访问 Registry：

```bash
curl -H "Authorization: Bearer eyJhbGc..." http://localhost:4999/v2/_catalog
```

---

## 最佳实践

### 1. 使用正确的 Accept Header

始终指定正确的 `Accept` header 以获取期望的 manifest 格式：

```bash
# 支持多种格式
curl -H "Accept: application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json" \
  http://localhost:4999/v2/distribution/manifests/3.0.0
```

### 2. 处理分页

使用 `n` 和 `last` 参数进行分页：

```bash
# 第一页
curl http://localhost:4999/v2/_catalog?n=100

# 下一页
curl http://localhost:4999/v2/_catalog?n=100&last=lastRepoName
```

### 3. 验证 Digest

始终验证下载的内容与 digest 匹配：

```bash
# 获取 digest
DIGEST=$(curl -I -H "Accept: application/vnd.oci.image.manifest.v1+json" \
  http://localhost:4999/v2/distribution/manifests/3.0.0 | \
  grep -i docker-content-digest | awk '{print $2}' | tr -d '\r')

# 使用 digest 获取内容
curl http://localhost:4999/v2/distribution/manifests/$DIGEST
```

### 4. 使用 HEAD 请求检查存在性

在下载大文件之前，使用 HEAD 请求检查是否存在：

```bash
curl -I http://localhost:4999/v2/distribution/blobs/sha256:abc123...
```

---

## 参考资料

- [Docker Registry HTTP API V2 规范](https://docs.docker.com/registry/spec/api/)
- [OCI Distribution Specification](https://github.com/opencontainers/distribution-spec)
- [Docker Registry 官方文档](https://docs.docker.com/registry/)
