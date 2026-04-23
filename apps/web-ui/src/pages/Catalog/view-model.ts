/**
 * CatalogViewModel — thin shell around the /api/registry/overview
 * endpoint. All filtering / sorting / pagination happens on the backend
 * against the repo_meta cache; the VM just holds the current query
 * params and the server's paged response. No more per-row meta fan-out.
 */

import { BaseViewModel, type ViewModelLifecycle } from '@/lib/viewmodel/BaseViewModel';
import {
  getOverview,
  type OverviewItem,
  type OverviewSortDirection,
  type OverviewSortField,
} from '@/services/registry.service';

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

export class CatalogViewModel extends BaseViewModel<ViewState> implements ViewModelLifecycle {
  // Incrementing request token so out-of-order responses (user types
  // fast, older request resolves later) don't clobber newer data.
  private reqSeq = 0;

  constructor() {
    super({
      items: [],
      total: 0,
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
    await this.fetch();
  }

  /**
   * Re-fetch from the server using the current params. Request-sequence
   * guarding ensures a stale in-flight response can't overwrite a
   * newer one after the user changes filter/sort mid-flight.
   */
  private async fetch() {
    const mySeq = ++this.reqSeq;
    this.$updateState({ loading: true, error: null });
    try {
      const resp = await getOverview({
        page: this.state.page,
        pageSize: this.state.pageSize,
        sort: this.state.sort,
        direction: this.state.sortDirection,
        q: this.state.searchQuery || undefined,
      });
      if (mySeq !== this.reqSeq) return;
      this.$updateState({
        items: resp.items,
        total: resp.total,
        loading: false,
      });
    } catch (err) {
      if (mySeq !== this.reqSeq) return;
      this.$updateState({
        loading: false,
        error: err instanceof Error ? err.message : 'Failed to load repositories',
      });
    }
  }

  setSearchQuery(query: string) {
    // Jump back to page 0 on any filter change so users don't land on
    // page 3 of a narrowed-down result set with nothing showing.
    this.$updateState({ searchQuery: query, page: 0 });
    void this.fetch();
  }

  toggleSort(field: SortField) {
    if (this.state.sort === field) {
      this.$updateState({
        sortDirection: this.state.sortDirection === 'asc' ? 'desc' : 'asc',
        page: 0,
      });
    } else {
      // Column-appropriate default direction: asc for name (A→Z),
      // desc for numeric / temporal columns (biggest/newest first).
      this.$updateState({
        sort: field,
        sortDirection: field === 'name' ? 'asc' : 'desc',
        page: 0,
      });
    }
    void this.fetch();
  }

  setPage(page: number) {
    this.$updateState({ page: Math.max(0, page) });
    void this.fetch();
  }

  setPageSize(pageSize: number) {
    this.$updateState({ pageSize, page: 0 });
    void this.fetch();
  }

  async refresh() {
    await this.fetch();
  }
}
