/**
 * Catalog Page — dense table view backed by /api/registry/overview.
 * The server-side repo_meta cache is kept in sync by distribution
 * webhooks + a periodic reconciler, so every interaction (sort,
 * search, pagination) is a single HTTP call with fully-populated rows.
 */

import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
  Package,
} from 'lucide-react';
import { useNavigate } from 'react-router-dom';

import { useViewModel } from '@/lib/viewmodel';
import { CatalogViewModel, type SortField } from './view-model';
import { SearchBar } from '@/components/common/SearchBar';
import { formatBinarySize, formatDateTime } from '@/utils';
import { compactArchLabel, formatPlatform } from '../TagList/platforms';
import type { OverviewItem, OverviewPlatform } from '@/services/registry.service';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';

const PAGE_SIZE_OPTIONS = [25, 50, 100, 200];

function archLabel(item: OverviewItem): { label: string; title: string } {
  const platforms = item.platforms ?? [];
  if (platforms.length > 0) {
    // compactArchLabel already filters platform.os === 'unknown' (BuildKit
    // attestations), so we can pass the raw list through.
    return compactArchLabel(platforms);
  }
  return { label: '-', title: '' };
}

function singlePlatformLabel(p: OverviewPlatform): string {
  return formatPlatform({
    os: p.os,
    architecture: p.architecture,
    variant: p.variant,
  });
}

export default function Catalog() {
  const vm = useViewModel(CatalogViewModel);
  const snapshot = vm.$useSnapshot();
  const navigate = useNavigate();

  const pageCount = Math.max(1, Math.ceil(snapshot.total / snapshot.pageSize));
  const currentPage = Math.min(snapshot.page, pageCount - 1);

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
              {snapshot.total}&nbsp;
              {snapshot.total === 1 ? 'image' : 'images'} available
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
              {!snapshot.loading && snapshot.items.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6} className="text-center py-8 text-muted-foreground">
                    {snapshot.searchQuery
                      ? 'No repositories found matching your search.'
                      : 'No repositories available.'}
                  </TableCell>
                </TableRow>
              )}
              {!snapshot.loading &&
                snapshot.items.map((r) => {
                  const arch =
                    r.platforms && r.platforms.length > 1
                      ? archLabel(r as OverviewItem)
                      : r.platforms && r.platforms.length === 1
                        ? {
                            label: singlePlatformLabel(r.platforms[0]),
                            title: singlePlatformLabel(r.platforms[0]),
                          }
                        : { label: '-', title: '' };
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
                        title={r.latest_tag}
                      >
                        {r.latest_tag ?? '-'}
                      </TableCell>
                      <TableCell className="px-4 text-right text-sm text-muted-foreground tabular-nums">
                        {r.tag_count}
                      </TableCell>
                      <TableCell className="px-4 text-right text-sm text-muted-foreground tabular-nums">
                        {formatBinarySize(r.size)}
                      </TableCell>
                      <TableCell className="px-4 text-sm text-muted-foreground tabular-nums">
                        {formatDateTime(r.created)}
                      </TableCell>
                      <TableCell
                        className="px-4 text-sm text-muted-foreground font-mono"
                        title={arch.title}
                      >
                        {arch.label}
                      </TableCell>
                    </TableRow>
                  );
                })}
            </TableBody>
          </Table>
        </div>
      )}

      {!snapshot.loading && !snapshot.error && snapshot.total > 0 && (
        <div className="flex items-center justify-between text-sm">
          <div className="text-muted-foreground">
            Showing{' '}
            <span className="font-medium text-foreground">
              {currentPage * snapshot.pageSize + 1}–
              {Math.min((currentPage + 1) * snapshot.pageSize, snapshot.total)}
            </span>{' '}
            of <span className="font-medium text-foreground">{snapshot.total}</span>
          </div>
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-2">
              <span className="text-muted-foreground">Rows per page</span>
              <Select
                value={String(snapshot.pageSize)}
                onValueChange={(v) => vm.setPageSize(Number(v))}
              >
                <SelectTrigger className="h-8 w-20">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {PAGE_SIZE_OPTIONS.map((n) => (
                    <SelectItem key={n} value={String(n)}>
                      {n}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-center gap-1">
              <Button
                variant="ghost"
                size="icon"
                className="h-8 w-8"
                onClick={() => vm.setPage(0)}
                disabled={currentPage === 0}
                aria-label="First page"
              >
                <ChevronsLeft className="h-4 w-4" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                className="h-8 w-8"
                onClick={() => vm.setPage(currentPage - 1)}
                disabled={currentPage === 0}
                aria-label="Previous page"
              >
                <ChevronLeft className="h-4 w-4" />
              </Button>
              <span className="px-2 text-muted-foreground">
                Page <span className="font-medium text-foreground">{currentPage + 1}</span> /{' '}
                {pageCount}
              </span>
              <Button
                variant="ghost"
                size="icon"
                className="h-8 w-8"
                onClick={() => vm.setPage(currentPage + 1)}
                disabled={currentPage >= pageCount - 1}
                aria-label="Next page"
              >
                <ChevronRight className="h-4 w-4" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                className="h-8 w-8"
                onClick={() => vm.setPage(pageCount - 1)}
                disabled={currentPage >= pageCount - 1}
                aria-label="Last page"
              >
                <ChevronsRight className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
