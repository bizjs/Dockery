import { useEffect, useMemo } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useViewModel } from '@/lib/viewmodel';
import { TagListViewModel } from './view-model';
import { TagTable } from './TagTable';
import { TagDetailDrawer } from './TagDetailDrawer';
import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight, Trash2, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { formatBinarySize } from '@/utils';
import { compactArchLabel } from './platforms';
import { toast } from 'sonner';
import { currentUserViewModel } from '@/hooks/use-current-user';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';

const PAGE_SIZE_OPTIONS = [25, 50, 100, 200];

export default function TagList() {
  const { image } = useParams<{ image: string }>();
  const vm = useViewModel(TagListViewModel, { destroyOnUnmount: true });
  const snapshot = vm.$useSnapshot();
  const meSnap = useViewModel(currentUserViewModel).$useSnapshot();
  // Role `view` can only pull; hide delete affordances to match server policy.
  const canDelete = meSnap.user?.role === 'admin' || meSnap.user?.role === 'write';

  useEffect(() => {
    const decodedImage = decodeURIComponent(image || '');
    vm.setImageName(decodedImage);
  }, [vm, image]);

  // Slice the sorted list for the current page. Kept in the component
  // because Valtio class getters don't subscribe via $useSnapshot.
  const pageCount = useMemo(
    () => Math.max(1, Math.ceil(snapshot.tagList.length / snapshot.pageSize)),
    [snapshot.tagList.length, snapshot.pageSize],
  );
  const pagedTags = useMemo(() => {
    const start = snapshot.page * snapshot.pageSize;
    return snapshot.tagList.slice(start, start + snapshot.pageSize);
  }, [snapshot.tagList, snapshot.page, snapshot.pageSize]);

  const selectedSet = useMemo(() => new Set(snapshot.selectedTags), [snapshot.selectedTags]);
  const pageTagNames = useMemo(() => pagedTags.map((t) => t.tag), [pagedTags]);
  const allOnPageSelected =
    pageTagNames.length > 0 && pageTagNames.every((t) => selectedSet.has(t));
  const someOnPageSelected =
    !allOnPageSelected && pageTagNames.some((t) => selectedSet.has(t));

  if (!snapshot.image) {
    return (
      <div className="text-center py-12">
        <p className="text-destructive">No image specified</p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-4">
        <Link to="/">
          <Button variant="ghost" size="sm">
            <ChevronLeft className="h-4 w-4 mr-1" />
            Back
          </Button>
        </Link>
        <div className="flex-1">
          <h2 className="text-2xl font-bold">{snapshot.image}</h2>
          {!snapshot.loading && (
            <p className="text-sm text-muted-foreground mt-1">
              {snapshot.tagList.length} {snapshot.tagList.length === 1 ? 'tag' : 'tags'} available
            </p>
          )}
        </div>
      </div>

      {/* Loading state */}
      {snapshot.loading && (
        <div className="text-center py-12">
          <p className="text-muted-foreground">Loading tags...</p>
        </div>
      )}

      {/* Error state */}
      {snapshot.error && (
        <div className="text-center py-12">
          <p className="text-destructive">Error: {snapshot.error}</p>
        </div>
      )}

      {/* Empty state */}
      {!snapshot.loading && !snapshot.error && snapshot.tagList.length === 0 && (
        <div className="text-center py-12">
          <p className="text-muted-foreground">No tags available for this image.</p>
        </div>
      )}

      {/* Bulk-selection toolbar */}
      {canDelete && snapshot.selectedTags.length > 0 && (
        <div className="flex items-center justify-between gap-3 rounded-md border bg-muted/40 px-4 py-2">
          <div className="text-sm">
            <span className="font-semibold">{snapshot.selectedTags.length}</span>
            <span className="text-muted-foreground"> selected</span>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => vm.clearSelection()}
              className="ml-2 h-7 px-2 text-xs"
            >
              <X className="h-3 w-3 mr-1" />
              Clear
            </Button>
          </div>
          <Button
            variant="destructive"
            size="sm"
            onClick={() => vm.openBulkDeleteDialog()}
            className="h-8"
          >
            <Trash2 className="h-4 w-4 mr-1" />
            Delete {snapshot.selectedTags.length}
          </Button>
        </div>
      )}

      {/* Tags table */}
      {!snapshot.loading && !snapshot.error && snapshot.tagList.length > 0 && (
        <>
          <TagTable
            sortField={snapshot.sortField}
            sortDirection={snapshot.sortDirection}
            onSort={(field) => vm.setSorting(field)}
            showSelectionColumn={canDelete}
            allOnPageSelected={allOnPageSelected}
            someOnPageSelected={someOnPageSelected}
            onToggleSelectPage={() => vm.toggleSelectPage(pageTagNames)}
          >
            {pagedTags.map((tagInfo) => (
              <tr key={tagInfo.tag} className="border-b">
                {canDelete && (
                  <td className="px-4 py-3 w-10">
                    <Checkbox
                      checked={selectedSet.has(tagInfo.tag)}
                      onClick={(e) =>
                        vm.toggleTagSelection(tagInfo.tag, { shift: e.shiftKey })
                      }
                      aria-label={`Select tag ${tagInfo.tag}`}
                    />
                  </td>
                )}
                <td className="px-4 py-3">{tagInfo.tag}</td>
                <td className="px-4 py-3 text-muted-foreground w-32">
                  {formatBinarySize(tagInfo.size)}
                </td>
                <td className="px-4 py-3 text-muted-foreground w-45 min-w-45">
                  {tagInfo.created ? new Date(tagInfo.created).toLocaleString() : '-'}
                </td>
                <td className="px-4 py-3 text-muted-foreground font-mono text-xs">
                  <div className="truncate">{tagInfo.digest ? tagInfo.digest : '-'}</div>
                </td>
                <td className="px-4 py-3 text-muted-foreground w-45">
                  {tagInfo.platforms && tagInfo.platforms.length > 0 ? (
                    (() => {
                      const { label, title } = compactArchLabel(tagInfo.platforms);
                      return <span title={title}>{label}</span>;
                    })()
                  ) : (
                    <>{tagInfo.architecture || '-'}</>
                  )}
                </td>
                <td className="px-4 py-3 text-right">
                  <div className="flex items-center justify-end gap-3">
                    <button
                      onClick={() => vm.openDrawer(tagInfo)}
                      className="text-primary hover:underline text-sm font-medium"
                    >
                      Detail
                    </button>
                    {canDelete && (
                      <button
                        onClick={() => vm.openDeleteDialog(tagInfo)}
                        className="text-destructive hover:underline text-sm font-medium"
                        title="Delete this tag"
                      >
                        Delete
                      </button>
                    )}
                  </div>
                </td>
              </tr>
            ))}
          </TagTable>

          {/* Pagination */}
          <div className="flex items-center justify-between gap-4 px-1 text-sm">
            <div className="text-muted-foreground">
              Showing{' '}
              <span className="font-medium text-foreground">
                {snapshot.page * snapshot.pageSize + 1}–
                {Math.min((snapshot.page + 1) * snapshot.pageSize, snapshot.tagList.length)}
              </span>{' '}
              of <span className="font-medium text-foreground">{snapshot.tagList.length}</span>
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
                  disabled={snapshot.page === 0}
                  aria-label="First page"
                >
                  <ChevronsLeft className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  onClick={() => vm.setPage(snapshot.page - 1)}
                  disabled={snapshot.page === 0}
                  aria-label="Previous page"
                >
                  <ChevronLeft className="h-4 w-4" />
                </Button>
                <span className="px-2 text-muted-foreground">
                  Page <span className="font-medium text-foreground">{snapshot.page + 1}</span> /{' '}
                  {pageCount}
                </span>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  onClick={() => vm.setPage(snapshot.page + 1)}
                  disabled={snapshot.page >= pageCount - 1}
                  aria-label="Next page"
                >
                  <ChevronRight className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  onClick={() => vm.setPage(pageCount - 1)}
                  disabled={snapshot.page >= pageCount - 1}
                  aria-label="Last page"
                >
                  <ChevronsRight className="h-4 w-4" />
                </Button>
              </div>
            </div>
          </div>
        </>
      )}

      {/* Tag Detail Drawer */}
      <TagDetailDrawer
        open={snapshot.isDrawerOpen}
        onClose={() => vm.closeDrawer()}
        tagInfo={snapshot.selectedTag}
        imageName={snapshot.image}
      />

      {/* Single-tag Delete Confirmation Dialog */}
      <AlertDialog open={snapshot.deleteDialogOpen} onOpenChange={vm.closeDeleteDialog}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Tag</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to delete tag{' '}
              <span className="font-semibold text-foreground">{snapshot.tagToDelete?.tag}</span>{' '}
              from <span className="font-semibold text-foreground">{snapshot.image}</span>?
              <br />
              <br />
              This action cannot be undone. The tag will be permanently removed from the registry.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={snapshot.deleting}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={async (e) => {
                e.preventDefault();
                try {
                  await vm.deleteTag();
                  toast.success('Tag deleted successfully!');
                } catch {
                  toast.error('Failed to delete tag. Please try again.');
                }
              }}
              disabled={snapshot.deleting}
              className="bg-destructive hover:bg-destructive/90"
            >
              {snapshot.deleting ? 'Deleting...' : 'Delete'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Bulk Delete Confirmation Dialog */}
      <AlertDialog
        open={snapshot.bulkDeleteDialogOpen}
        onOpenChange={(open) => !open && !snapshot.bulkDeleting && vm.closeBulkDeleteDialog()}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              Delete {snapshot.selectedTags.length}{' '}
              {snapshot.selectedTags.length === 1 ? 'tag' : 'tags'}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              {snapshot.bulkDeleting ? (
                <>
                  Deleting{' '}
                  <span className="font-semibold text-foreground">
                    {snapshot.bulkDeleteProgress}
                  </span>{' '}
                  of <span className="font-semibold text-foreground">
                    {snapshot.selectedTags.length}
                  </span>
                  ...
                </>
              ) : snapshot.bulkDeleteFailed.length > 0 ? (
                <>
                  <span className="text-destructive font-medium">
                    {snapshot.bulkDeleteFailed.length} tag
                    {snapshot.bulkDeleteFailed.length === 1 ? '' : 's'} failed to delete.
                  </span>{' '}
                  The failures remain selected — close this dialog and retry.
                </>
              ) : (
                <>
                  This will permanently remove{' '}
                  <span className="font-semibold text-foreground">
                    {snapshot.selectedTags.length}
                  </span>{' '}
                  {snapshot.selectedTags.length === 1 ? 'tag' : 'tags'} from{' '}
                  <span className="font-semibold text-foreground">{snapshot.image}</span>. This
                  action cannot be undone.
                </>
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={snapshot.bulkDeleting}>
              {snapshot.bulkDeleteFailed.length > 0 ? 'Close' : 'Cancel'}
            </AlertDialogCancel>
            {snapshot.bulkDeleteFailed.length === 0 && (
              <AlertDialogAction
                onClick={async (e) => {
                  e.preventDefault();
                  const { deleted, failed } = await vm.bulkDelete();
                  if (failed.length === 0) {
                    toast.success(`Deleted ${deleted} ${deleted === 1 ? 'tag' : 'tags'}.`);
                  } else {
                    toast.error(
                      `Deleted ${deleted}, ${failed.length} failed. See dialog for details.`,
                    );
                  }
                }}
                disabled={snapshot.bulkDeleting}
                className="bg-destructive hover:bg-destructive/90"
              >
                {snapshot.bulkDeleting
                  ? `Deleting ${snapshot.bulkDeleteProgress}/${snapshot.selectedTags.length}...`
                  : `Delete ${snapshot.selectedTags.length}`}
              </AlertDialogAction>
            )}
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
