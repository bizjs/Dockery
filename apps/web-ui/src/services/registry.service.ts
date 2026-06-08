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

/**
 * Aggregated Catalog view backed by the server-side repo_meta cache.
 * One request returns a page of repositories with their representative
 * tag's meta already resolved — no per-row fan-out from the browser.
 *
 * The cache is kept in sync by distribution webhooks + a periodic
 * reconciler; see apps/api/internal/biz/repo_meta.go.
 */

export interface OverviewPlatform {
  os?: string;
  architecture?: string;
  variant?: string;
}

export interface OverviewItem {
  repo: string;
  latest_tag?: string;
  tag_count: number;
  size: number;
  created?: string;
  platforms?: OverviewPlatform[];
  pull_count: number;
  last_pulled_at?: number;
  refreshed_at: number;
}

export interface OverviewResponse {
  items: OverviewItem[];
  total: number;
  page: number;
  page_size: number;
}

export type OverviewSortField = 'name' | 'updated' | 'size' | 'tags';
export type OverviewSortDirection = 'asc' | 'desc';

export interface OverviewParams {
  page?: number;
  pageSize?: number;
  sort?: OverviewSortField;
  direction?: OverviewSortDirection;
  q?: string;
}

export async function getOverview(p: OverviewParams = {}): Promise<OverviewResponse> {
  const qs = new URLSearchParams();
  if (p.page !== undefined) qs.set('page', String(p.page));
  if (p.pageSize !== undefined) qs.set('page_size', String(p.pageSize));
  if (p.sort) qs.set('sort', p.sort);
  if (p.direction) qs.set('direction', p.direction);
  if (p.q) qs.set('q', p.q);
  const suffix = qs.toString();
  return api.get<OverviewResponse>(
    `/api/registry/overview${suffix ? '?' + suffix : ''}`,
  );
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

/**
 * Fetch every tag's full ImageInfo for a repo in ONE aggregated request.
 *
 * dockery-api fans out the per-tag manifest + config fetches server-side
 * (parallel, connection-pooled) and returns them together, replacing the
 * browser's prior 2N+1 round-trips (1 tags list + N manifests + N config
 * blobs), which the 6-connection-per-host limit serialized into waves.
 * The response is JSON-shaped to match ImageInfo, so it's consumed as-is.
 */
export async function listImageTags(imageName: string): Promise<ImageInfo[]> {
  const resp = await api.get<{ items: ImageInfo[] }>(
    `/api/registry/${encodeURIComponent(imageName)}/tags/details`,
  );
  return resp.items ?? [];
}

/** Delete a tag (server resolves digest and issues DELETE by digest). */
export async function deleteImageTag(repository: string, tag: string): Promise<void> {
  await api.delete<null>(
    `/api/registry/${encodeURIComponent(repository)}/manifests/${encodeURIComponent(tag)}`,
  );
}
