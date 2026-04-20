/**
 * Registry data access — all calls go through dockery-api at
 * /api/registry/*, NOT straight to /v2/*. dockery-api is responsible
 * for:
 *   - session-based authentication (HttpOnly cookie)
 *   - per-user repo_permissions filtering (catalog, tags)
 *   - minting short-lived JWTs for the upstream registry
 *   - two-step delete (tag → digest → DELETE)
 *
 * The UI therefore never needs to know about Docker token auth.
 */

import { api } from './api';

export interface PlatformEntry {
  os?: string;
  architecture?: string;
  variant?: string;
  digest: string;
  size: number;
}

export interface ImageInfo {
  imageName: string;
  tag: string;
  digest: string;
  /** Total size. Single-arch: config + layers. Multi-arch: sum of manifests[i].size. */
  size: number;
  created?: string;
  architecture?: string;
  os?: string;
  layers: number;
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
  /**
   * Present only when the tag points to a manifest list / OCI image index.
   * Per-platform fields (architecture, os, layers, cmd, env, …) at the top
   * level are left empty in that case — they belong to each child manifest,
   * not the list itself.
   */
  platforms?: PlatformEntry[];
}

interface CatalogResponse {
  repositories?: string[];
}

interface TagsResponse {
  name: string;
  tags?: string[];
}

// --- Manifest shapes returned by /api/registry/{name}/manifests/{ref} ---
// The endpoint can return either a single-arch manifest (v2+json) OR a
// manifest list / OCI image index (list.v2+json / image.index.v1+json).
// We accept a union; `manifests` is non-null only for the list case.

interface ManifestEntry {
  mediaType?: string;
  digest?: string;
  size?: number;
  platform?: {
    architecture?: string;
    os?: string;
    variant?: string;
    'os.version'?: string;
  };
}

interface ManifestV2 {
  schemaVersion?: number;
  mediaType?: string;
  // Single-arch fields
  config?: { mediaType?: string; size?: number; digest?: string };
  layers?: Array<{ mediaType?: string; size?: number; digest?: string }>;
  // Manifest list fields
  manifests?: ManifestEntry[];
}

const MANIFEST_LIST_MEDIA_TYPES = new Set([
  'application/vnd.docker.distribution.manifest.list.v2+json',
  'application/vnd.oci.image.index.v1+json',
]);

function isManifestList(m: ManifestV2): boolean {
  if (Array.isArray(m.manifests)) return true;
  return m.mediaType ? MANIFEST_LIST_MEDIA_TYPES.has(m.mediaType) : false;
}

interface ConfigBlob {
  id?: string;
  created?: string;
  architecture?: string;
  os?: string;
  config?: {
    Cmd?: string[];
    Env?: string[];
    WorkingDir?: string;
    Labels?: Record<string, string>;
    ExposedPorts?: Record<string, unknown>;
  };
  history?: Array<{
    created?: string;
    created_by?: string;
    comment?: string;
    empty_layer?: boolean;
  }>;
}

/** List repositories visible to the current session user. */
export async function listRepositories(): Promise<{ repo: string; tags: string[] }[]> {
  // ?? (not destructure default) because the upstream distribution
  // registry returns {"repositories": null} / {"tags": null} when the
  // set is empty — e.g. after deleting the last tag of a repo. A
  // destructure default only covers `undefined`, so tags would stay
  // null and crash at .length / .map downstream.
  const response = await api.get<CatalogResponse>('/api/registry/catalog');
  const repositories = response.repositories ?? [];
  const results = await Promise.all(
    repositories.map(async (repo) => {
      try {
        const tagsResp = await api.get<TagsResponse>(
          `/api/registry/${encodeURIComponent(repo)}/tags`,
        );
        return { repo, tags: tagsResp.tags ?? [] };
      } catch {
        return { repo, tags: [] };
      }
    }),
  );
  return results;
}

/** Fetch full ImageInfo (manifest + config blob) for one tag. */
export async function getImageInfo(repository: string, tag: string): Promise<ImageInfo> {
  const manifest = await api.get<ManifestV2>(
    `/api/registry/${encodeURIComponent(repository)}/manifests/${encodeURIComponent(tag)}`,
  );

  // Multi-arch short-circuit: a manifest list doesn't have a config blob
  // or layers of its own. We surface the per-platform list and the sum
  // of per-platform sizes; drilling into a specific platform's layers /
  // config can be done later via the drawer on demand.
  if (isManifestList(manifest)) {
    const entries = manifest.manifests ?? [];
    const platforms: PlatformEntry[] = entries.map((e) => ({
      os: e.platform?.os,
      architecture: e.platform?.architecture,
      variant: e.platform?.variant,
      digest: e.digest ?? '',
      size: e.size ?? 0,
    }));
    const totalSize = platforms.reduce((s, p) => s + p.size, 0);
    return {
      imageName: repository,
      tag,
      digest: '',
      size: totalSize,
      layers: 0,
      platforms,
    };
  }

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

  const configDigest = manifest.config?.digest;
  if (configDigest) {
    try {
      const cfg = await api.get<ConfigBlob>(
        `/api/registry/${encodeURIComponent(repository)}/blobs/${encodeURIComponent(configDigest)}`,
      );
      created = cfg.created;
      architecture = cfg.architecture;
      os = cfg.os;
      id = cfg.id;
      cmd = cfg.config?.Cmd;
      env = cfg.config?.Env;
      workingDir = cfg.config?.WorkingDir;
      labels = cfg.config?.Labels;
      if (cfg.config?.ExposedPorts) {
        exposedPorts = Object.keys(cfg.config.ExposedPorts);
      }

      // cfg.history and manifest.layers can legitimately be null in
      // minimal manifests — guard before .map / .length.
      if (Array.isArray(cfg.history) && Array.isArray(manifest.layers)) {
        let li = 0;
        history = cfg.history.map((h) => {
          const entry: NonNullable<ImageInfo['history']>[number] = {
            created: h.created,
            created_by: h.created_by,
            comment: h.comment,
            empty_layer: h.empty_layer,
          };
          if (!h.empty_layer && manifest.layers && li < manifest.layers.length) {
            const layer = manifest.layers[li];
            entry.size = layer.size;
            entry.id = layer.digest;
            li++;
          }
          return entry;
        });
      }
    } catch (err) {
      console.warn('config blob fetch failed:', err);
    }
  }

  const configSize = manifest.config?.size ?? 0;
  const layersSize = (manifest.layers ?? []).reduce((s, l) => s + (l.size ?? 0), 0);

  return {
    imageName: repository,
    tag,
    // Docker-Content-Digest is set as a response header by the proxy
    // but we don't surface it through the envelope — the delete flow
    // resolves digest server-side from the tag, so UI rarely needs it.
    digest: '',
    size: configSize + layersSize,
    created,
    architecture,
    os,
    layers: manifest.layers?.length ?? 0,
    id,
    cmd,
    env,
    workingDir,
    labels,
    exposedPorts,
    history,
  };
}

/** Fetch every tag's ImageInfo for a repo. */
export async function listImageTags(imageName: string): Promise<ImageInfo[]> {
  const resp = await api.get<TagsResponse>(
    `/api/registry/${encodeURIComponent(imageName)}/tags`,
  );
  const tags = resp.tags ?? [];
  const infos = await Promise.all(
    tags.map(async (tag) => {
      try {
        return await getImageInfo(imageName, tag);
      } catch (err) {
        console.warn(`failed to fetch info for ${imageName}:${tag}`, err);
        return {
          imageName,
          tag,
          digest: '',
          size: 0,
          layers: 0,
        } satisfies ImageInfo;
      }
    }),
  );
  return infos;
}

/** Delete a tag (server resolves digest and issues DELETE by digest). */
export async function deleteImageTag(repository: string, tag: string): Promise<void> {
  await api.delete<null>(
    `/api/registry/${encodeURIComponent(repository)}/manifests/${encodeURIComponent(tag)}`,
  );
}
