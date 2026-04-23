/**
 * Catalog Page — dense table view. Repos load first; per-row meta
 * (size / updated / arch) streams in behind via parallel manifest
 * fetches so rows don't block on the slowest repo. Clicking a row
 * navigates to that repo's tag list. Sort is driven by clicking
 * column headers (same pattern as TagTable).
 */

import { useMemo } from 'react';
import { ArrowDown, ArrowUp, ArrowUpDown, Package } from 'lucide-react';
import { useNavigate } from 'react-router-dom';

import { useViewModel } from '@/lib/viewmodel';
import {
  CatalogViewModel,
  filterAndSort,
  type SortField,
} from './view-model';
import { SearchBar } from '@/components/common/SearchBar';
import { formatBinarySize } from '@/utils';
import { compactArchLabel, formatPlatform } from '../TagList/platforms';
import type { ImageInfo } from '@/services/registry.service';
import { Button } from '@/components/ui/button';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';

const DATE_FORMAT: Intl.DateTimeFormatOptions = {
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hour12: false,
};

/** Tiny inline skeleton — avoids pulling in the shadcn Skeleton component. */
function SkeletonBar({ className }: { className?: string }) {
  return <div className={`h-3 animate-pulse rounded bg-muted ${className ?? ''}`} />;
}

function archLabel(meta: ImageInfo): { label: string; title: string } {
  if (meta.platforms && meta.platforms.length > 0) {
    return compactArchLabel(meta.platforms);
  }
  if (meta.os || meta.architecture) {
    const s = formatPlatform({ os: meta.os, architecture: meta.architecture });
    return { label: s, title: s };
  }
  return { label: '-', title: '' };
}

export default function Catalog() {
  const vm = useViewModel(CatalogViewModel);
  const snapshot = vm.$useSnapshot();
  const navigate = useNavigate();

  // Valtio class getters don't subscribe through $useSnapshot, so
  // filter + sort runs as a pure derivation in the component.
  const displayed = useMemo(
    () =>
      filterAndSort(
        snapshot.repositories,
        snapshot.searchQuery,
        snapshot.sort,
        snapshot.sortDirection,
      ),
    [snapshot.repositories, snapshot.searchQuery, snapshot.sort, snapshot.sortDirection],
  );

  const sortIcon = (field: SortField) => {
    if (snapshot.sort !== field) return <ArrowUpDown className="ml-2 h-4 w-4" />;
    return snapshot.sortDirection === 'asc' ? (
      <ArrowUp className="ml-2 h-4 w-4" />
    ) : (
      <ArrowDown className="ml-2 h-4 w-4" />
    );
  };

  const SortButton = ({
    field,
    label,
    align = 'left',
  }: {
    field: SortField;
    label: string;
    align?: 'left' | 'right';
  }) => (
    <Button
      variant="ghost"
      onClick={() => vm.toggleSort(field)}
      className={`h-8 px-3 ${align === 'left' ? '-ml-3' : '-mr-3'}`}
    >
      {label}
      {sortIcon(field)}
    </Button>
  );

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold">Images</h2>
          {!snapshot.loading && (
            <p className="text-sm text-muted-foreground mt-1">
              {displayed.length}&nbsp;
              {displayed.length === 1 ? 'image' : 'images'} available
            </p>
          )}
        </div>
        <div className="w-full max-w-md">
          <SearchBar
            value={snapshot.searchQuery}
            onChange={vm.setSearchQuery}
            placeholder="Search images..."
          />
        </div>
      </div>

      {snapshot.error && (
        <div className="text-center py-12">
          <p className="text-destructive">Error: {snapshot.error}</p>
        </div>
      )}

      {!snapshot.error && (
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="px-4">
                  <SortButton field="name" label="Image" />
                </TableHead>
                <TableHead className="w-40 px-4">Latest</TableHead>
                <TableHead className="w-24 px-4 text-right">
                  <SortButton field="tags" label="Tags" align="right" />
                </TableHead>
                <TableHead className="w-32 px-4 text-right">
                  <SortButton field="size" label="Size" align="right" />
                </TableHead>
                <TableHead className="w-[200px] px-4">
                  <SortButton field="updated" label="Updated" />
                </TableHead>
                <TableHead className="w-56 px-4">Architecture</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {snapshot.loading && (
                <TableRow>
                  <TableCell colSpan={6} className="text-center py-8 text-muted-foreground">
                    Loading…
                  </TableCell>
                </TableRow>
              )}
              {!snapshot.loading && displayed.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6} className="text-center py-8 text-muted-foreground">
                    {snapshot.searchQuery
                      ? 'No repositories found matching your search.'
                      : 'No repositories available.'}
                  </TableCell>
                </TableRow>
              )}
              {!snapshot.loading &&
                displayed.map((r) => {
                  const meta = r.meta;
                  const arch = meta ? archLabel(meta) : null;
                  return (
                    <TableRow
                      key={r.repo}
                      className="cursor-pointer"
                      onClick={() => navigate(`/tag-list/${encodeURIComponent(r.repo)}`)}
                    >
                      <TableCell className="px-4 font-medium">
                        <div className="flex items-center gap-2">
                          <Package className="h-4 w-4 text-primary shrink-0" />
                          <span className="truncate">{r.repo}</span>
                        </div>
                      </TableCell>
                      <TableCell
                        className="px-4 font-mono text-sm text-muted-foreground truncate"
                        title={r.latestTag}
                      >
                        {r.latestTag ?? '-'}
                      </TableCell>
                      <TableCell className="px-4 text-right text-sm text-muted-foreground tabular-nums">
                        {r.tags.length}
                      </TableCell>
                      <TableCell className="px-4 text-right text-sm text-muted-foreground tabular-nums">
                        {meta === undefined ? (
                          <SkeletonBar className="w-16 ml-auto" />
                        ) : meta ? (
                          formatBinarySize(meta.size)
                        ) : (
                          '-'
                        )}
                      </TableCell>
                      <TableCell className="px-4 text-sm text-muted-foreground tabular-nums">
                        {meta === undefined ? (
                          <SkeletonBar className="w-32" />
                        ) : meta?.created ? (
                          new Date(meta.created).toLocaleString(undefined, DATE_FORMAT)
                        ) : (
                          '-'
                        )}
                      </TableCell>
                      <TableCell
                        className="px-4 text-sm text-muted-foreground font-mono"
                        title={arch?.title}
                      >
                        {meta === undefined ? (
                          <SkeletonBar className="w-28" />
                        ) : (
                          arch?.label ?? '-'
                        )}
                      </TableCell>
                    </TableRow>
                  );
                })}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}
