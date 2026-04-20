import type { PlatformEntry } from '@/services/registry.service';

/** `linux/amd64`, `linux/arm/v7`, or `unknown` when everything is empty. */
export function formatPlatform(p: Pick<PlatformEntry, 'os' | 'architecture' | 'variant'>): string {
  const parts: string[] = [];
  if (p.os) parts.push(p.os);
  if (p.architecture) parts.push(p.architecture);
  if (p.variant) parts.push(p.variant);
  return parts.length > 0 ? parts.join('/') : 'unknown';
}

/**
 * Compact label for the Architecture cell of a multi-arch tag:
 *   1 platform  → `linux/amd64`
 *   2 platforms → `linux/amd64, linux/arm64`
 *   3+          → `linux/amd64 +2 more`
 * The `title` is the newline-joined full list, for hover tooltip.
 */
export function compactArchLabel(
  platforms: ReadonlyArray<Pick<PlatformEntry, 'os' | 'architecture' | 'variant'>>,
): { label: string; title: string } {
  const formatted = platforms.map(formatPlatform);
  const title = formatted.join('\n');
  if (formatted.length === 0) return { label: '-', title: '' };
  if (formatted.length === 1) return { label: formatted[0], title };
  if (formatted.length === 2) return { label: formatted.join(', '), title };
  return { label: `${formatted[0]} +${formatted.length - 1} more`, title };
}
