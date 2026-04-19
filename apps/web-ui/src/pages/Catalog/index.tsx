/**
 * Catelog Page
 * Display list of Docker images/repositories using view-model pattern
 */

import { useViewModel } from '@/lib/viewmodel';
import { CatalogViewModel } from './view-model';
import { SearchBar } from '@/components/common/SearchBar';
import { CatalogItem } from './CatalogItem';

export default function Catelog() {
  const vm = useViewModel(CatalogViewModel);
  const snapshot = vm.$useSnapshot();

  return (
    <div className="space-y-6">
      {/* Header with search */}
      <div className="flex items-center justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold">Images</h2>
          {!snapshot.loading && (
            <p className="text-sm text-muted-foreground mt-1">
              {snapshot.filteredRepositories.length}&nbsp;
              {snapshot.filteredRepositories.length === 1 ? 'image' : 'images'} available
            </p>
          )}
        </div>
        <div className="w-full max-w-md">
          <SearchBar value={snapshot.searchQuery} onChange={vm.setSearchQuery} placeholder="Search images..." />
        </div>
      </div>

      {/* Loading state */}
      {snapshot.loading && (
        <div className="text-center py-12">
          <p className="text-muted-foreground">Loading repositories...</p>
        </div>
      )}

      {/* Error state */}
      {snapshot.error && (
        <div className="text-center py-12">
          <p className="text-destructive">Error: {snapshot.error}</p>
        </div>
      )}

      {/* Empty state */}
      {!snapshot.loading && !snapshot.error && snapshot.filteredRepositories.length === 0 && (
        <div className="text-center py-12">
          <p className="text-muted-foreground">
            {snapshot.searchQuery ? 'No repositories found matching your search.' : 'No repositories available.'}
          </p>
        </div>
      )}

      {/* Image grid */}
      {!snapshot.loading && !snapshot.error && snapshot.filteredRepositories.length > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
          {snapshot.filteredRepositories.map((x) => (
            <CatalogItem key={x.repo} name={x.repo} description={x.repo} tagCount={x.tags.length} />
          ))}
        </div>
      )}
    </div>
  );
}
