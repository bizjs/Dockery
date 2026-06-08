import { describe, expect, it } from 'vitest';

import {
  CATALOG_DEFAULTS,
  parseCatalogParams,
  serializeCatalogParams,
  type CatalogUrlState,
} from './url-state';

describe('parseCatalogParams', () => {
  it('returns empty object for an empty query', () => {
    expect(parseCatalogParams('')).toEqual({});
  });

  it('parses every valid param', () => {
    expect(parseCatalogParams('?q=nginx&sort=size&dir=asc&page=2&size=100')).toEqual({
      searchQuery: 'nginx',
      sort: 'size',
      sortDirection: 'asc',
      page: 2,
      pageSize: 100,
    });
  });

  it('drops invalid values so they fall back to defaults', () => {
    // bogus sort field, bad direction, negative page, unsupported size
    expect(parseCatalogParams('?sort=bogus&dir=sideways&page=-1&size=33')).toEqual({});
  });

  it('treats an empty q as absent', () => {
    expect(parseCatalogParams('?q=')).toEqual({});
  });
});

describe('serializeCatalogParams', () => {
  it('omits fields equal to their default', () => {
    expect(serializeCatalogParams(CATALOG_DEFAULTS).toString()).toBe('');
  });

  it('serializes only non-default fields', () => {
    const state: CatalogUrlState = {
      ...CATALOG_DEFAULTS,
      searchQuery: 'app',
      sort: 'name',
    };
    expect(serializeCatalogParams(state).toString()).toBe('q=app&sort=name');
  });
});

describe('round-trip', () => {
  it('parse(serialize(state)) reconstructs a non-default state', () => {
    const state: CatalogUrlState = {
      searchQuery: 'redis',
      sort: 'tags',
      sortDirection: 'asc',
      page: 3,
      pageSize: 200,
    };
    const search = serializeCatalogParams(state).toString();
    expect({ ...CATALOG_DEFAULTS, ...parseCatalogParams(search) }).toEqual(state);
  });
});
