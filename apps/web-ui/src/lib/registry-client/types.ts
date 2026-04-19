/**
 * Registry API 响应类型
 */
export interface RegistryResponse<T> {
  data: T;
  headers: Headers;
  status: number;
}

/**
 * 仓库列表响应
 */
export interface CatalogResponse {
  repositories: string[];
}

/**
 * 标签列表响应
 */
export interface TagsResponse {
  name: string;
  tags: string[];
}

/**
 * Manifest 响应
 */
export interface ManifestResponse {
  schemaVersion: number;
  mediaType?: string;
  config?: {
    mediaType: string;
    size: number;
    digest: string;
  };
  layers?: Array<{
    mediaType: string;
    size: number;
    digest: string;
  }>;
  manifests?: Array<{
    mediaType: string;
    size: number;
    digest: string;
    platform?: {
      architecture: string;
      os: string;
    };
  }>;
}

/**
 * Config Blob 响应（镜像配置）
 */
export interface ConfigBlobResponse {
  created?: string;
  architecture?: string;
  os?: string;
  config?: {
    Env?: string[];
    Cmd?: string[];
    WorkingDir?: string;
    Labels?: Record<string, string>;
    ExposedPorts?: Record<string, object>;
  };
  rootfs?: {
    type: string;
    diff_ids: string[];
  };
  history?: Array<{
    created?: string;
    created_by?: string;
    comment?: string;
    empty_layer?: boolean;
  }>;
  id?: string;
}

/**
 * 分页参数
 */
export interface PaginationParams {
  n?: number;
  last?: string;
}
