/**
 * Docker Registry API Client
 * 封装所有 Docker Registry v2 API 接口
 */

import { config } from '@/config';
import { RegistryClient } from '@/lib/registry-client/RegistryClient';

/**
 * 默认的 Registry Client 实例
 */
const registryClient = new RegistryClient(config.registryUrl);

/**
 * 业务层封装：获取镜像的完整信息
 */
export interface ImageInfo {
  imageName: string;
  tag: string;
  digest: string;
  size: number;
  created?: string;
  architecture?: string;
  os?: string;
  layers: number;
  // 完整的配置信息
  id?: string;
  cmd?: string[];
  env?: string[];
  workingDir?: string;
  labels?: Record<string, string>;
  exposedPorts?: string[];
  history?: Array<{
    created?: string;
    created_by?: string;
    comment?: string;
    empty_layer?: boolean;
    size?: number;
    id?: string;
  }>;
}

/**
 * 获取镜像的完整信息
 */
export async function getImageInfo(repository: string, tag: string): Promise<ImageInfo> {
  // 获取 manifest
  const manifestResponse = await registryClient.getManifest(repository, tag);
  const manifest = manifestResponse.data;
  const digest = manifestResponse.headers.get('Docker-Content-Digest') || '';

  // 获取 config blob
  let created: string | undefined;
  let architecture: string | undefined;
  let os: string | undefined;
  let id: string | undefined;
  let cmd: string[] | undefined;
  let env: string[] | undefined;
  let workingDir: string | undefined;
  let labels: Record<string, string> | undefined;
  let exposedPorts: string[] | undefined;
  let history: ImageInfo['history'] | undefined;

  if (manifest.config?.digest) {
    try {
      const configResponse = await registryClient.getConfigBlob(repository, manifest.config.digest);
      const configData = configResponse.data;
      
      created = configData.created;
      architecture = configData.architecture;
      os = configData.os;
      id = configData.id;
      cmd = configData.config?.Cmd;
      env = configData.config?.Env;
      workingDir = configData.config?.WorkingDir;
      labels = configData.config?.Labels;
      
      // 处理 ExposedPorts
      if (configData.config?.ExposedPorts) {
        exposedPorts = Object.keys(configData.config.ExposedPorts);
      }
      
      // 处理 history，并关联 layer 大小
      if (configData.history && manifest.layers) {
        let layerIndex = 0;
        history = configData.history.map((h) => {
          const historyItem: NonNullable<ImageInfo['history']>[number] = {
            created: h.created,
            created_by: h.created_by,
            comment: h.comment,
            empty_layer: h.empty_layer,
          };
          
          // 如果不是空层，关联对应的 layer 信息
          if (!h.empty_layer && manifest.layers && layerIndex < manifest.layers.length) {
            const layer = manifest.layers[layerIndex];
            historyItem.size = layer.size;
            historyItem.id = layer.digest;
            layerIndex++;
          }
          
          return historyItem;
        });
      }
    } catch (error) {
      console.error('Failed to fetch config blob:', error);
    }
  }

  // 计算镜像总大小：config size + 所有 layers 的 size
  const configSize = manifest.config?.size || 0;
  const layersSize = manifest.layers?.reduce((total, layer) => total + (layer.size || 0), 0) || 0;
  const totalSize = configSize + layersSize;

  return {
    imageName: repository,
    tag,
    digest,
    size: totalSize,
    created,
    architecture,
    os,
    layers: manifest.layers?.length || 0,
    id,
    cmd,
    env,
    workingDir,
    labels,
    exposedPorts,
    history,
  };
}

/**
 * 批量获取标签信息
 */
export async function getTagsInfo(repository: string, tags: string[]): Promise<ImageInfo[]> {
  const promises = tags.map((tag) => getImageInfo(repository, tag));
  return Promise.all(promises);
}

/**
 * 搜索仓库
 */
export async function listRepositories(size: number, last: string): Promise<{ repo: string; tags: string[] }[]> {
  const response = await registryClient.listRepositories({ n: size, last });
  const repositories = response.data.repositories || [];

  const result: { repo: string; tags: string[] }[] = repositories.map((x) => ({ repo: x, tags: [] }));

  await Promise.all(
    result.map(async (repo) => {
      const res = await registryClient.listTags(repo.repo);
      repo.tags = res.data.tags || [];
      return repo;
    }),
  );
  return result;
}

export async function listImageTags(imageName: string) {
  const tagsResponse = await registryClient.listTags(imageName);
  const tags = tagsResponse.data.tags || [];

  console.log('Received tags:', tags.length);

  // 批量获取每个标签的详细信息
  const tagInfoPromises = tags.map(async (tag) => {
    try {
      return await getImageInfo(imageName, tag);
    } catch (err) {
      console.error(`Failed to fetch info for tag ${tag}:`, err);
      // 返回基本信息
      return {
        imageName: imageName,
        tag,
        digest: '',
        size: 0,
        layers: 0,
      } as ImageInfo;
    }
  });

  const tagInfos = await Promise.all(tagInfoPromises);

  return tagInfos;
}

/**
 * 删除镜像标签
 */
export async function deleteImageTag(repository: string, tag: string): Promise<void> {
  await registryClient.deleteTag(repository, tag);
}
