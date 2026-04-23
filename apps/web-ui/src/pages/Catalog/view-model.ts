/**
 * CatalogViewModel — lists repos, fans out a per-repo manifest fetch so
 * each card can show size / last-updated / architecture without waiting
 * on the others. Meta loads are parallel and best-effort: a failing
 * repo still renders with just its name + tag count.
 */

import { BaseViewModel, type ViewModelLifecycle } from '@/lib/viewmodel/BaseViewModel';
import { getImageInfo, listRepositories, type ImageInfo } from '@/services/registry.service';

export type SortField = 'name' | 'updated' | 'size' | 'tags';
export type SortDirection = 'asc' | 'desc';

export interface RepoEntry {
  repo: string;
  tags: string[];
  /**
   * The tag whose manifest we fetched for the row's meta. `undefined`
   * only for repos with an empty tag list (shouldn't happen in
   * practice — GC sweeps empty repos).
   */
  latestTag?: string;
  /** `undefined` while loading, `null` if the meta fetch failed. */
  meta?: ImageInfo | null;
}

interface ViewState {
  repositories: RepoEntry[];
  searchQuery: string;
  sort: SortField;
  sortDirection: SortDirection;
  page: number;
  pageSize: number;
  loading: boolean;
  error: string | null;
}

/**
 * Pick a representative tag for the catalog card. Prefer `latest`; else
 * take the last tag in the list (upstream returns tags lexicographically,
 * so the last one is usually the newest-looking). This is a heuristic —
 * registries don't expose push-time, so we can't know the true latest.
 */
function pickRepresentativeTag(tags: string[]): string | null {
  if (tags.length === 0) return null;
  if (tags.includes('latest')) return 'latest';
  return tags[tags.length - 1];
}

export class CatalogViewModel extends BaseViewModel<ViewState> implements ViewModelLifecycle {
  constructor() {
    super({
      repositories: [],
      searchQuery: '',
      sort: 'name',
      sortDirection: 'asc',
      page: 0,
      pageSize: 50,
      loading: true,
      error: null,
    });
  }

  async $onMounted() {
    await this.loadRepositories();
  }

  private async loadRepositories() {
    try {
      this.$updateState({ loading: true, error: null });
      const repos = await listRepositories();
      const entries: RepoEntry[] = repos.map((r) => ({
        repo: r.repo,
        tags: r.tags,
        latestTag: pickRepresentativeTag(r.tags) ?? undefined,
      }));
      this.$updateState({ repositories: entries, loading: false });
      // Kick off per-repo meta fetches in parallel; don't block the grid.
      this.fanOutMetaFetches();
    } catch (err) {
      this.$updateState({
        error: err instanceof Error ? err.message : 'Failed to fetch repositories',
        loading: false,
      });
    }
  }

  /**
   * For each repo, fetch its representative tag's ImageInfo. Results
   * are written back into `repositories[i].meta` as they arrive.
   * Failures set meta=null; the card falls back to name-only rendering.
   */
  private fanOutMetaFetches() {
    this.state.repositories.forEach((entry, idx) => {
      if (!entry.latestTag) {
        this.updateMeta(idx, null);
        return;
      }
      getImageInfo(entry.repo, entry.latestTag)
        .then((info) => this.updateMeta(idx, info))
        .catch(() => this.updateMeta(idx, null));
    });
  }

  private updateMeta(idx: number, meta: ImageInfo | null) {
    const next = this.state.repositories.slice();
    if (!next[idx]) return;
    next[idx] = { ...next[idx], meta };
    this.$updateState({ repositories: next });
  }

  setSearchQuery(query: string) {
    // Jump back to page 0 on any filter change so users don't land on
    // page 3 of a narrowed-down result set with nothing showing.
    this.$updateState({ searchQuery: query, page: 0 });
  }

  /**
   * Click a column header: if it's the active sort, flip direction;
   * otherwise switch to it with a column-appropriate default
   * direction (name asc, everything else desc — biggest/newest first
   * is what users usually want on first click).
   */
  toggleSort(field: SortField) {
    if (this.state.sort === field) {
      this.$updateState({
        sortDirection: this.state.sortDirection === 'asc' ? 'desc' : 'asc',
        page: 0,
      });
      return;
    }
    this.$updateState({
      sort: field,
      sortDirection: field === 'name' ? 'asc' : 'desc',
      page: 0,
    });
  }

  setPage(page: number) {
    this.$updateState({ page: Math.max(0, page) });
  }

  setPageSize(pageSize: number) {
    this.$updateState({ pageSize, page: 0 });
  }

  async refresh() {
    await this.loadRepositories();
  }
}

/**
 * Pure derivation: filter by search query, then sort. Lives outside the
 * VM so the component can call it inside `useMemo` and re-derive when
 * the snapshot changes — Valtio class getters don't subscribe via
 * `$useSnapshot`, so using a getter here would give a stale result.
 */
export function filterAndSort(
  entries: readonly RepoEntry[],
  query: string,
  sort: SortField,
  direction: SortDirection,
): RepoEntry[] {
  const q = query.trim().toLowerCase();
  const filtered = q
    ? entries.filter((r) => r.repo.toLowerCase().includes(q))
    : entries.slice();

  // Comparators return their "natural" order (name asc, everything
  // else desc — biggest/newest first). `direction` flips the result.
  // Ties fall back to name asc so the order is stable.
  const naturalCmp = (a: RepoEntry, b: RepoEntry): number => {
    switch (sort) {
      case 'name':
        return a.repo.localeCompare(b.repo);
      case 'tags':
        return b.tags.length - a.tags.length || a.repo.localeCompare(b.repo);
      case 'size': {
        const sa = a.meta?.size ?? -1;
        const sb = b.meta?.size ?? -1;
        return sb - sa || a.repo.localeCompare(b.repo);
      }
      case 'updated': {
        const ua = a.meta?.created ? Date.parse(a.meta.created) : 0;
        const ub = b.meta?.created ? Date.parse(b.meta.created) : 0;
        return ub - ua || a.repo.localeCompare(b.repo);
      }
    }
  };
  const naturalDir: SortDirection = sort === 'name' ? 'asc' : 'desc';
  const flip = direction === naturalDir ? 1 : -1;
  filtered.sort((a, b) => flip * naturalCmp(a, b));
  return filtered;
}
