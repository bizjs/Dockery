# Dockery

`Dockery` is a minimal Docker Registry stack combining Docker Distribution, a pluggable auth-service, and a modern web UI — without the complexity of Harbor.

[中文文档](./README_CN.md)

## Features

- ✅ **Docker Registry v3.0.0**: Official Docker Distribution
- ✅ **Modern Web UI**: React + TypeScript with shadcn/ui
- ✅ **Full Management**: Browse, view details, and delete images
- ✅ **Nginx Proxy**: Unified access through a single port
- ✅ **Easy Configuration**: Environment variable support
- ✅ **Production Ready**: Docker Compose deployment

## Architecture

### Network Architecture
- Web UI serves through nginx on port 3000
- Registry API proxied through nginx at `/v2/` path
- No CORS issues, unified domain access
- Registry service runs internally on port 5000

### Port Mapping
- **Web UI**: External port 3000 → Internal port 80 (nginx)
- **Registry**: Internal port 5000 (not exposed by default)

## Quick Start

### Prerequisites

- Docker Engine 20.10+
- Docker Compose 2.0+

### 1. Configure Docker Daemon

Edit Docker configuration (on macOS, use Docker Desktop settings):

```json
{
  "insecure-registries": ["localhost:3000"]
}
```

Restart Docker after configuration.

### 2. Start Services

```bash
# Build and start all services
docker-compose up -d

# View logs
docker-compose logs -f

# View specific service logs
docker-compose logs -f web-ui
docker-compose logs -f registry
```

### 3. Access Web UI

Open browser: http://localhost:3000

### 4. Push Images to Registry

```bash
# Tag image
docker tag your-image:tag localhost:3000/your-image:tag

# Push image (through nginx proxy)
docker push localhost:3000/your-image:tag
```

### 5. Pull Images

```bash
# Pull from registry
docker pull localhost:3000/your-image:tag
```

## Management Commands

### Stop Services
```bash
docker-compose down
```

### Stop and Remove Data
```bash
docker-compose down -v
```

### Rebuild UI
```bash
docker-compose build web-ui
docker-compose up -d web-ui
```

### View Service Status
```bash
docker-compose ps
```

## Configuration

### Registry Configuration

Environment variables in `docker-compose.yaml`:

```yaml
registry:
  environment:
    REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY: /var/lib/registry
    REGISTRY_STORAGE_DELETE_ENABLED: 'true'
```

### Web UI Configuration

The UI uses environment variables for configuration:

```yaml
web-ui:
  environment:
    - REGISTRY_URL=http://registry:5000
```

You can customize the registry URL without rebuilding the image.

### Nginx Configuration

The nginx configuration uses templates with environment variable substitution:

- Template file: `web-ui/nginx.conf.template`
- Uses `${REGISTRY_URL}` for dynamic configuration
- Automatically processed on container startup

## Web UI Features

### Image Management
- ✅ Browse all images and tags
- ✅ View image details (size, creation time, architecture)
- ✅ View complete image configuration (Cmd, Env, Labels, Ports)
- ✅ View image layer history
- ✅ Sort by tag, size, or creation time
- ✅ Copy Docker pull commands
- ✅ Delete image tags (with confirmation dialog)
- ✅ Responsive design, mobile-friendly

### Technical Details
- **Framework**: React 19 + TypeScript
- **Build Tool**: Vite
- **UI Components**: shadcn/ui + Tailwind CSS
- **State Management**: Valtio
- **HTTP Server**: Nginx (Alpine)

## Troubleshooting

### UI Cannot Connect to Registry

1. Check if services are running: `docker-compose ps`
2. Check network connection: `docker network inspect dockery-net`
3. View UI logs: `docker-compose logs web-ui`
4. View Registry logs: `docker-compose logs registry`

### Cannot Push Images

1. Ensure Docker daemon is configured with insecure registry:
   ```json
   {
     "insecure-registries": ["localhost:3000"]
   }
   ```
2. Restart Docker daemon
3. Verify the registry is accessible: `curl http://localhost:3000/v2/`

### Delete Function Not Working

Ensure Registry has delete enabled (already configured in docker-compose.yaml):
```yaml
REGISTRY_STORAGE_DELETE_ENABLED: 'true'
```

## Production Deployment

### 1. Use HTTPS

Configure SSL certificates:

```yaml
web-ui:
  ports:
    - "443:443"
  volumes:
    - ./ssl:/etc/nginx/ssl
```

Update nginx configuration to use SSL.

### 2. Add Authentication

Configure Registry authentication (Basic Auth or Token Auth).

### 3. Configure Storage Backend

Use cloud storage (S3, Azure Blob, etc.) instead of local filesystem:

```yaml
registry:
  environment:
    REGISTRY_STORAGE: s3
    REGISTRY_STORAGE_S3_BUCKET: your-bucket
    REGISTRY_STORAGE_S3_REGION: us-east-1
```

### 4. Set Resource Limits

```yaml
web-ui:
  deploy:
    resources:
      limits:
        cpus: '1'
        memory: 512M
```

## Development

### Run UI in Development Mode

```bash
cd web-ui
pnpm install
pnpm run dev
```

Then access: http://localhost:5173

### Build UI

```bash
cd web-ui
pnpm run build
```

## Environment Variables

### Registry

| Variable | Default | Description |
|----------|---------|-------------|
| `REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY` | `/var/lib/registry` | Storage directory |
| `REGISTRY_STORAGE_DELETE_ENABLED` | `true` | Enable delete operations |

### Web UI

| Variable | Default | Description |
|----------|---------|-------------|
| `REGISTRY_URL` | `http://registry:5000` | Registry backend URL |

## License

See [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Support

If you encounter any issues, please create an issue on GitHub.
