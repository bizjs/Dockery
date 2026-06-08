/**
 * CatalogViewModel — thin shell around the /api/registry/overview
 * endpoint. All filtering / sorting / pagination happens on the backend
 * against the repo_meta cache; the VM just holds the current query
 * params and the server's paged response. No more per-row meta fan-out.
 */

import { ViewModelBase } from 'bizify';
import {
  getOverview,
  type OverviewItem,
  type OverviewSortDirection,
  type OverviewSortField,
} from '@/services/registry.service';
import { CATALOG_DEFAULTS, parseCatalogParams } from './url-state';

export type SortField = OverviewSortField;
export type SortDirection = OverviewSortDirection;

interface ViewState {
  items: OverviewItem[];
  total: number;
  searchQuery: string;
  sort: SortField;
  sortDirection: SortDirection;
  page: number;
  pageSize: number;
  loading: boolean;
  error: string | null;
}

const SEARCH_DEBOUNCE_MS = 300;

export class CatalogViewModel extends ViewModelBase<ViewState> {
  // Incrementing request token so out-of-order responses (user types
  // fast, older request resolves later) don't clobber newer data.
  private reqSeq = 0;
  // Pending debounced search fetch, if any — reset on every keystroke.
  private searchTimer?: ReturnType<typeof setTimeout>;

  protected $data(): ViewState {
    // Hydrate search / sort / pagination from the URL so the state
    // survives drilling into a tag-list and back, a hard refresh, and
    // bookmarks. parseCatalogParams drops invalid values, so anything
    // missing or malformed falls back to CATALOG_DEFAULTS — which carry
    // the newest-first default that opens the page on recent activity.
    // (Switching columns reverts to each column's natural direction;
    // see toggleSort.)
    return {
      items: [],
      total: 0,
      ...CATALOG_DEFAULTS,
      ...parseCatalogParams(window.location.search),
      loading: true,
      error: null,
    };
  }

  protected onMount() {
    void this.fetch();
  }

  /**
   * Re-fetch from the server using the current params. Request-sequence
   * guarding ensures a stale in-flight response can't overwrite a
   * newer one after the user changes filter/sort mid-flight.
   */
  private async fetch() {
    const mySeq = ++this.reqSeq;
    Object.assign(this.data, { loading: true, error: null });
    try {
      const resp = await getOverview({
        page: this.data.page,
        pageSize: this.data.pageSize,
        sort: this.data.sort,
        direction: this.data.sortDirection,
        q: this.data.searchQuery || undefined,
      });
      if (mySeq !== this.reqSeq) return;
      Object.assign(this.data, {
        items: resp.items,
        total: resp.total,
        loading: false,
      });
    } catch (err) {
      if (mySeq !== this.reqSeq) return;
      Object.assign(this.data, {
        loading: false,
        error: err instanceof Error ? err.message : 'Failed to load repositories',
      });
    }
  }

  setSearchQuery(query: string) {
    // State updates immediately so the input stays responsive; the
    // actual fetch is debounced to avoid hammering the backend on
    // every keystroke. Jump back to page 0 on any filter change so
    // users don't land on page 3 of a narrowed-down result with
    // nothing showing.
    Object.assign(this.data, { searchQuery: query, page: 0 });
    if (this.searchTimer) clearTimeout(this.searchTimer);
    this.searchTimer = setTimeout(() => {
      void this.fetch();
    }, SEARCH_DEBOUNCE_MS);
  }

  toggleSort(field: SortField) {
    if (this.data.sort === field) {
      Object.assign(this.data, {
        sortDirection: this.data.sortDirection === 'asc' ? 'desc' : 'asc',
        page: 0,
      });
    } else {
      // Column-appropriate default direction: asc for name (A→Z),
      // desc for numeric / temporal columns (biggest/newest first).
      Object.assign(this.data, {
        sort: field,
        sortDirection: field === 'name' ? 'asc' : 'desc',
        page: 0,
      });
    }
    this.cancelSearchDebounce();
    void this.fetch();
  }

  setPage(page: number) {
    this.data.page = Math.max(0, page);
    this.cancelSearchDebounce();
    void this.fetch();
  }

  setPageSize(pageSize: number) {
    Object.assign(this.data, { pageSize, page: 0 });
    this.cancelSearchDebounce();
    void this.fetch();
  }

  async refresh() {
    this.cancelSearchDebounce();
    await this.fetch();
  }

  /** Cancel any pending debounced search fetch. Called before an
   * immediate fetch so we don't race a stale debounced request against
   * the new one. */
  private cancelSearchDebounce() {
    if (this.searchTimer) {
      clearTimeout(this.searchTimer);
      this.searchTimer = undefined;
    }
  }
}
