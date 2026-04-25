import { compareLoose, valid } from 'semver';

/**
 * compareTags is the ascending comparator for Docker image tags.
 *
 *   1. If both tags parse as semver (with the conventional `v` prefix
 *      tolerated via the loose option), use semver order — that gives
 *      `v0.0.10 > v0.0.9` and the right prerelease handling
 *      (`v1.0.0-rc.1 < v1.0.0`).
 *   2. If exactly one is semver, the semver tag ranks higher; that
 *      pushes "floating" labels like `latest` / `main` / `dev` below
 *      the version list when sorted descending.
 *   3. If neither is semver, fall back to natural-order compare so
 *      date-style tags (`20260425` > `20260424`) still order sanely.
 *
 * Returns the standard comparator triple (-1 / 0 / +1).
 */
export function compareTags(a: string, b: string): number {
  const va = valid(a, { loose: true });
  const vb = valid(b, { loose: true });
  if (va !== null && vb !== null) {
    return compareLoose(va, vb);
  }
  if (va !== null) return 1;
  if (vb !== null) return -1;
  return a.localeCompare(b, undefined, { numeric: true, sensitivity: 'base' });
}
