import { useEffect } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useViewModel } from '@/lib/viewmodel';
import { TagListViewModel } from './view-model';
import { TagTable } from './TagTable';
import { TagDetailDrawer } from './TagDetailDrawer';
import { ChevronLeft } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { formatBinarySize } from '@/utils';
import { toast } from 'sonner';
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

export default function TagList() {
  const { image } = useParams<{ image: string }>();
  const vm = useViewModel(TagListViewModel, { destroyOnUnmount: true });
  const snapshot = vm.$useSnapshot();

  useEffect(() => {
    const decodedImage = decodeURIComponent(image || '');
    vm.setImageName(decodedImage);
  }, [vm, image]);

  // Use view-model hook - pass a dummy instance if vm is null

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

      {/* Tags table */}
      {!snapshot.loading && !snapshot.error && snapshot.tagList.length > 0 && (
        <TagTable
          sortField={snapshot.sortField}
          sortDirection={snapshot.sortDirection}
          onSort={(field) => vm.setSorting(field)}
        >
          {snapshot.tagList.map((tagInfo) => (
            <tr key={tagInfo.tag} className="border-b">
              <td className="px-4 py-3">{tagInfo.tag}</td>
              <td className="px-4 py-3 text-muted-foreground w-32">{formatBinarySize(tagInfo.size)}</td>
              <td className="px-4 py-3 text-muted-foreground w-45 min-w-45">
                {tagInfo.created ? new Date(tagInfo.created).toLocaleString() : '-'}
              </td>
              <td className="px-4 py-3 text-muted-foreground font-mono text-xs">
                <div className="truncate">{tagInfo.digest ? tagInfo.digest : '-'}</div>
              </td>
              <td className="px-4 py-3 text-muted-foreground w-45">{tagInfo.architecture || '-'}</td>
              <td className="px-4 py-3 text-right">
                <div className="flex items-center justify-end gap-3">
                  <button
                    onClick={() => vm.openDrawer(tagInfo)}
                    className="text-primary hover:underline text-sm font-medium"
                  >
                    Detail
                  </button>
                  <button
                    onClick={() => vm.openDeleteDialog(tagInfo)}
                    className="text-destructive hover:underline text-sm font-medium"
                    title="Delete this tag"
                  >
                    Delete
                  </button>
                </div>
              </td>
            </tr>
          ))}
        </TagTable>
      )}

      {/* Tag Detail Drawer */}
      <TagDetailDrawer
        open={snapshot.isDrawerOpen}
        onClose={() => vm.closeDrawer()}
        tagInfo={snapshot.selectedTag}
        imageName={snapshot.image}
      />

      {/* Delete Confirmation Dialog */}
      <AlertDialog open={snapshot.deleteDialogOpen} onOpenChange={vm.closeDeleteDialog}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Tag</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to delete tag{' '}
              <span className="font-semibold text-foreground">
                {snapshot.tagToDelete?.tag}
              </span>{' '}
              from{' '}
              <span className="font-semibold text-foreground">{snapshot.image}</span>?
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
    </div>
  );
}
