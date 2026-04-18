import { RegistryError } from './RegistryError';
import type {
  CatalogResponse,
  ConfigBlobResponse,
  ManifestResponse,
  PaginationParams,
  RegistryResponse,
  TagsResponse,
} from './types';

/**
 * Docker Registry Client
 */
export class RegistryClient {
  private baseUrl: string;

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl;
  }

  /**
   * 构建完整的 URL
   */
  private buildUrl(path: string): string {
    return `${this.baseUrl}${path}`;
  }

  /**
   * 处理 API 响应
   */
  private async handleResponse<T>(response: Response): Promise<RegistryResponse<T>> {
    if (!response.ok) {
      let errorMessage = `HTTP ${response.status}: ${response.statusText}`;
      let errorCode: string | undefined;
      let errorDetails: unknown;

      try {
        const errorData = await response.json();
        if (errorData.errors && errorData.errors.length > 0) {
          const firstError = errorData.errors[0];
          errorCode = firstError.code;
          errorMessage = firstError.message || errorMessage;
          errorDetails = errorData.errors;
        }
      } catch {
        // 无法解析错误响应，使用默认错误消息
      }

      throw new RegistryError(errorMessage, errorCode, response.status, errorDetails);
    }

    const data = await response.json();
    return {
      data,
      headers: response.headers,
      status: response.status,
    };
  }

  /**
   * 1. 检查 Registry 版本
   */
  async checkVersion(): Promise<boolean> {
    try {
      const response = await fetch(this.buildUrl('/v2/'));
      return response.ok;
    } catch {
      return false;
    }
  }

  /**
   * 2. 获取仓库列表
   */
  async listRepositories(params?: PaginationParams): Promise<RegistryResponse<CatalogResponse>> {
    const queryParams = new URLSearchParams();
    if (params?.n) queryParams.append('n', params.n.toString());
    if (params?.last) queryParams.append('last', params.last);

    const url = this.buildUrl(`/v2/_catalog${queryParams.toString() ? `?${queryParams}` : ''}`);
    const response = await fetch(url);
    return this.handleResponse<CatalogResponse>(response);
  }

  /**
   * 3. 获取标签列表
   */
  async listTags(repository: string, params?: PaginationParams): Promise<RegistryResponse<TagsResponse>> {
    const queryParams = new URLSearchParams();
    if (params?.n) queryParams.append('n', params.n.toString());
    if (params?.last) queryParams.append('last', params.last);

    const url = this.buildUrl(`/v2/${repository}/tags/list${queryParams.toString() ? `?${queryParams}` : ''}`);
    const response = await fetch(url);
    return this.handleResponse<TagsResponse>(response);
  }

  /**
   * 4. 获取 Manifest
   */
  async getManifest(repository: string, reference: string): Promise<RegistryResponse<ManifestResponse>> {
    const url = this.buildUrl(`/v2/${repository}/manifests/${reference}`);
    const response = await fetch(url, {
      headers: {
        Accept:
          'application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.index.v1+json',
      },
    });
    return this.handleResponse<ManifestResponse>(response);
  }

  /**
   * 获取 Manifest 的 Digest
   */
  async getManifestDigest(repository: string, reference: string): Promise<string | null> {
    const url = this.buildUrl(`/v2/${repository}/manifests/${reference}`);
    const response = await fetch(url, {
      method: 'HEAD',
      headers: {
        Accept: 'application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json',
      },
    });

    if (!response.ok) {
      throw new RegistryError(`Failed to get manifest digest: ${response.statusText}`, undefined, response.status);
    }

    return response.headers.get('Docker-Content-Digest');
  }

  /**
   * 6. 删除 Manifest（删除标签）
   */
  async deleteManifest(repository: string, digest: string): Promise<void> {
    const url = this.buildUrl(`/v2/${repository}/manifests/${digest}`);
    const response = await fetch(url, {
      method: 'DELETE',
    });

    if (!response.ok) {
      throw new RegistryError(`Failed to delete manifest: ${response.statusText}`, undefined, response.status);
    }
  }

  /**
   * 删除标签（先获取 digest 再删除）
   */
  async deleteTag(repository: string, tag: string): Promise<void> {
    // 先获取 digest
    const digest = await this.getManifestDigest(repository, tag);
    if (!digest) {
      throw new RegistryError('Failed to get manifest digest for deletion');
    }

    // 删除 manifest
    await this.deleteManifest(repository, digest);
  }

  /**
   * 7. 获取 Blob
   */
  async getBlob<T = unknown>(repository: string, digest: string): Promise<RegistryResponse<T>> {
    const url = this.buildUrl(`/v2/${repository}/blobs/${digest}`);
    const response = await fetch(url);
    return this.handleResponse<T>(response);
  }

  /**
   * 获取 Config Blob（镜像配置）
   */
  async getConfigBlob(repository: string, digest: string): Promise<RegistryResponse<ConfigBlobResponse>> {
    return this.getBlob<ConfigBlobResponse>(repository, digest);
  }

  /**
   * 8. 检查 Blob 是否存在
   */
  async checkBlobExists(repository: string, digest: string): Promise<boolean> {
    const url = this.buildUrl(`/v2/${repository}/blobs/${digest}`);
    const response = await fetch(url, { method: 'HEAD' });
    return response.ok;
  }

  /**
   * 获取 Blob 大小
   */
  async getBlobSize(repository: string, digest: string): Promise<number | null> {
    const url = this.buildUrl(`/v2/${repository}/blobs/${digest}`);
    const response = await fetch(url, { method: 'HEAD' });

    if (!response.ok) {
      return null;
    }

    const contentLength = response.headers.get('Content-Length');
    return contentLength ? parseInt(contentLength, 10) : null;
  }
}
