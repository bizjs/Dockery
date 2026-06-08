/**
 * URL <-> CatalogViewModel state codec. The Catalog query string is the
 * single source of truth for search / sort / pagination so the state
 * survives drilling into a tag-list and coming back (browser history),
 * a hard refresh, and bookmark/share. Only non-default values are
 * serialized, keeping the URL clean (`/`) when nothing is customized.
 *
 * Kept router-agnostic on purpose: it deals in plain query strings, so
 * the ViewModel can hydrate from `window.location.search` without
 * importing react-router (matches the project's router-agnostic VM
 * convention).
 */

import type { SortField, SortDirection } from './view-model';

export interface CatalogUrlState {
  searchQuery: string;
  sort: SortField;
  sortDirection: SortDirection;
  page: number;
  pageSize: number;
}

export const CATALOG_DEFAULTS: CatalogUrlState = {
  searchQuery: '',
  sort: 'updated',
  sortDirection: 'desc',
  page: 0,
  pageSize: 50,
};

/** The page-size choices offered by the Catalog's <Select> and the only
 *  sizes a URL may carry — any other value would leave the dropdown with
 *  no matching item, so parse clamps unknown sizes back to the default.
 *  Single source of truth, imported by index.tsx for the dropdown. */
export const PAGE_SIZE_OPTIONS = [25, 50, 100, 200];
const VALID_SORTS: SortField[] = ['name', 'updated', 'size', 'tags'];

/**
 * Parse a Catalog query string into a partial state. Invalid or missing
 * params are simply omitted (the caller merges over CATALOG_DEFAULTS),
 * so a malformed URL degrades to defaults instead of throwing.
 */
export function parseCatalogParams(search: string): Partial<CatalogUrlState> {
  const params = new URLSearchParams(search);
  const out: Partial<CatalogUrlState> = {};

  const q = params.get('q');
  if (q) out.searchQuery = q;

  const sort = params.get('sort');
  if (sort && (VALID_SORTS as string[]).includes(sort)) {
    out.sort = sort as SortField;
  }

  const dir = params.get('dir');
  if (dir === 'asc' || dir === 'desc') out.sortDirection = dir;

  // Guard on presence: Number(null) is 0, which would otherwise pass
  // the integer check and set page: 0 for a missing param.
  const pageRaw = params.get('page');
  if (pageRaw !== null) {
    const page = Number(pageRaw);
    if (Number.isInteger(page) && page >= 0) out.page = page;
  }

  const sizeRaw = params.get('size');
  if (sizeRaw !== null) {
    const size = Number(sizeRaw);
    if (PAGE_SIZE_OPTIONS.includes(size)) out.pageSize = size;
  }

  return out;
}

/**
 * Serialize state into a URLSearchParams, omitting any field that
 * equals its default so the URL stays minimal.
 */
export function serializeCatalogParams(state: CatalogUrlState): URLSearchParams {
  const params = new URLSearchParams();
  if (state.searchQuery) params.set('q', state.searchQuery);
  if (state.sort !== CATALOG_DEFAULTS.sort) params.set('sort', state.sort);
  if (state.sortDirection !== CATALOG_DEFAULTS.sortDirection) {
    params.set('dir', state.sortDirection);
  }
  if (state.page !== CATALOG_DEFAULTS.page) params.set('page', String(state.page));
  if (state.pageSize !== CATALOG_DEFAULTS.pageSize) {
    params.set('size', String(state.pageSize));
  }
  return params;
}
