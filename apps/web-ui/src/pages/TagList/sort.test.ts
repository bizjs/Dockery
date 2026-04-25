import { describe, expect, it } from 'vitest';

import { compareTags } from './sort';

// Sort the input ascending via compareTags and assert the order.
function sorted(tags: string[]): string[] {
  return [...tags].sort(compareTags);
}

describe('compareTags', () => {
  it('orders semver numerically, not lexicographically', () => {
    // Exact bug from the user report: lex would put v0.0.10 before
    // v0.0.9 (because '1' < '9'); semver gets it right.
    expect(sorted(['v0.0.9', 'v0.0.10'])).toEqual(['v0.0.9', 'v0.0.10']);
    expect(sorted(['v1.10.0', 'v1.2.0', 'v1.2.10'])).toEqual([
      'v1.2.0',
      'v1.2.10',
      'v1.10.0',
    ]);
  });

  it('places prereleases below their release', () => {
    expect(sorted(['v1.0.0', 'v1.0.0-rc.1', 'v1.0.0-rc.2'])).toEqual([
      'v1.0.0-rc.1',
      'v1.0.0-rc.2',
      'v1.0.0',
    ]);
  });

  it('accepts both v-prefixed and bare semver', () => {
    expect(sorted(['0.0.10', 'v0.0.9'])).toEqual(['v0.0.9', '0.0.10']);
  });

  it('ranks non-semver tags below semver in ascending order', () => {
    // `latest` / `main` / `dev` are not semver — they end up at the
    // bottom (asc) so descending sort floats them above the semver list.
    expect(sorted(['v1.2.3', 'latest', 'v0.5.0', 'dev'])).toEqual([
      'dev',
      'latest',
      'v0.5.0',
      'v1.2.3',
    ]);
  });

  it('falls back to natural-order compare for non-semver tags', () => {
    expect(sorted(['20260424', '20260425', '20260423'])).toEqual([
      '20260423',
      '20260424',
      '20260425',
    ]);
    expect(sorted(['main', 'dev', 'staging'])).toEqual(['dev', 'main', 'staging']);
  });

  it('returns 0 for equal tags', () => {
    expect(compareTags('v1.0.0', 'v1.0.0')).toBe(0);
    expect(compareTags('latest', 'latest')).toBe(0);
  });
});
