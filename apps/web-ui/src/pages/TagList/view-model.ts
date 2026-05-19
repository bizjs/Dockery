/**
 * TagListViewModel - TagList 页面的 ViewModel
 * 管理 Docker 镜像标签列表的状态和业务逻辑
 */

import { BaseViewModel, type ViewModelLifecycle } from '@/lib/viewmodel/BaseViewModel';
import { listImageTags, deleteImageTag, type ImageInfo } from '@/services/registry.service';
import { compareTags } from './sort';

type SortField = 'tag' | 'size' | 'created';
type SortDirection = 'asc' | 'desc';

// applySort is the single ordering function for both the initial fetch
// and user-driven re-sorts. Operates on a plain array (not a Valtio
// proxy) so the comparator never reads through proxy traps — that
// matters because the proxied array items can briefly look stale on
// the microtask boundary between two consecutive $updateState writes.
// Returns a new array; never mutates the input.
function applySort(
  list: ImageInfo[],
  field: SortField | null,
  direction: SortDirection,
): ImageInfo[] {
  if (!field) return [...list];
  const copy = [...list];
  copy.sort((a, b) => {
    let cmp = 0;
    switch (field) {
      case 'tag':
        // Strict semver via compareTags: handles prereleases correctly
        // (v1.0.0-rc.1 < v1.0.0) and falls back to natural-order
        // compare for non-semver tags (latest, dev, dates).
        cmp = compareTags(a.tag, b.tag);
        break;
      case 'size':
        cmp = a.size - b.size;
        break;
      case 'created': {
        const da = a.created ? new Date(a.created).getTime() : 0;
        const db = b.created ? new Date(b.created).getTime() : 0;
        cmp = da - db;
        break;
      }
    }
    return direction === 'asc' ? cmp : -cmp;
  });
  return copy;
}

interface ViewState {
  image: string;
  tagList: ImageInfo[];
  loading: boolean;
  error: string | null;
  sortField: SortField | null;
  sortDirection: SortDirection;
  selectedTag: ImageInfo | null;
  isDrawerOpen: boolean;
  // single-tag delete
  deleteDialogOpen: boolean;
  tagToDelete: ImageInfo | null;
  deleting: boolean;
  // multi-select + bulk delete
  selectedTags: string[];
  lastSelectedTag: string | null;
  bulkDeleteDialogOpen: boolean;
  bulkDeleting: boolean;
  bulkDeleteProgress: number;
  bulkDeleteFailed: string[];
  // pagination
  page: number;
  pageSize: number;
}

export class TagListViewModel extends BaseViewModel<ViewState> implements ViewModelLifecycle {
  constructor() {
    super({
      image: '',
      tagList: [],
      loading: true,
      error: null,
      // Default to version-newest-first via the semver-aware compareTags
      // (see ./sort.ts) so `v0.0.10` correctly sits above `v0.0.9`.
      // Mirrors the backend's pickRepresentativeTag intent.
      sortField: 'tag',
      sortDirection: 'desc',
      selectedTag: null,
      isDrawerOpen: false,
      deleteDialogOpen: false,
      tagToDelete: null,
      deleting: false,
      selectedTags: [],
      lastSelectedTag: null,
      bulkDeleteDialogOpen: false,
      bulkDeleting: false,
      bulkDeleteProgress: 0,
      bulkDeleteFailed: [],
      page: 0,
      pageSize: 50,
    });
  }

  async setImageName(name: string) {
    this.state.image = name;
    await this.loadTags();
  }

  async $onMounted() {}

  private async loadTags() {
    try {
      this.$updateState({ loading: true, error: null });
      const tagList = await listImageTags(this.state.image);
      // Sort BEFORE committing to state so the table never flashes the
      // upstream order (which is undefined; distribution doesn't sort).
      // The previous two-step "commit then sort" briefly painted the
      // unsorted list and — with Valtio's array proxying — could leak
      // through if a snapshot was taken between the two writes.
      const sorted = applySort(tagList, this.state.sortField, this.state.sortDirection);
      this.$updateState({
        tagList: sorted,
        loading: false,
        page: 0,
        selectedTags: [],
        lastSelectedTag: null,
      });
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to fetch tags';
      this.$updateState({ error: errorMessage, loading: false });
    }
  }

  async refresh() {
    await this.loadTags();
  }

  get tagCount(): number {
    return this.state.tagList.length;
  }

  setSorting(field: SortField) {
    const { sortField, sortDirection, tagList } = this.state;
    const nextDir: SortDirection =
      sortField === field ? (sortDirection === 'asc' ? 'desc' : 'asc') : 'desc';
    // Sort on a plain-array copy of the current proxy state, then
    // commit field + direction + tagList in one $updateState. Combining
    // the writes makes the sort atomic to subscribers.
    const sorted = applySort([...tagList], field, nextDir);
    this.$updateState({
      sortField: field,
      sortDirection: nextDir,
      tagList: sorted,
      page: 0,
    });
  }

  openDrawer(tagInfo: ImageInfo) {
    this.$updateState({ selectedTag: tagInfo, isDrawerOpen: true });
  }

  closeDrawer() {
    this.$updateState({ isDrawerOpen: false, selectedTag: null });
  }

  openDeleteDialog(tagInfo: ImageInfo) {
    this.$updateState({ deleteDialogOpen: true, tagToDelete: tagInfo });
  }

  closeDeleteDialog() {
    this.$updateState({ deleteDialogOpen: false, tagToDelete: null });
  }

  async deleteTag() {
    const { tagToDelete, image } = this.state;
    if (!tagToDelete) return;
    try {
      this.$updateState({ deleting: true });
      await deleteImageTag(image, tagToDelete.tag);
      this.$updateState({
        tagList: this.state.tagList.filter((t) => t.tag !== tagToDelete.tag),
        selectedTags: this.state.selectedTags.filter((t) => t !== tagToDelete.tag),
        deleting: false,
        deleteDialogOpen: false,
        tagToDelete: null,
      });
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to delete tag';
      this.$updateState({ error: errorMessage, deleting: false });
      throw err;
    }
  }

  /**
   * Toggle selection for a single tag. When `shift` is true, select the
   * inclusive range between the last-clicked tag and this one (always
   * additive, matching joxit's Shift+Click behaviour).
   */
  toggleTagSelection(tag: string, opts?: { shift?: boolean }) {
    const { tagList, selectedTags, lastSelectedTag } = this.state;

    if (opts?.shift && lastSelectedTag && lastSelectedTag !== tag) {
      const currIdx = tagList.findIndex((t) => t.tag === tag);
      const prevIdx = tagList.findIndex((t) => t.tag === lastSelectedTag);
      if (currIdx >= 0 && prevIdx >= 0) {
        const [from, to] = currIdx < prevIdx ? [currIdx, prevIdx] : [prevIdx, currIdx];
        const next = new Set(selectedTags);
        for (let i = from; i <= to; i++) next.add(tagList[i].tag);
        this.$updateState({ selectedTags: Array.from(next), lastSelectedTag: tag });
        return;
      }
    }

    const next = new Set(selectedTags);
    if (next.has(tag)) next.delete(tag);
    else next.add(tag);
    this.$updateState({ selectedTags: Array.from(next), lastSelectedTag: tag });
  }

  /** Toggle "select all on current page" — adds if any unchecked, removes if all checked. */
  toggleSelectPage(pageTags: string[]) {
    if (pageTags.length === 0) return;
    const { selectedTags } = this.state;
    const current = new Set(selectedTags);
    const allSelected = pageTags.every((t) => current.has(t));
    if (allSelected) {
      pageTags.forEach((t) => current.delete(t));
    } else {
      pageTags.forEach((t) => current.add(t));
    }
    this.$updateState({ selectedTags: Array.from(current) });
  }

  clearSelection() {
    this.$updateState({ selectedTags: [], lastSelectedTag: null });
  }

  openBulkDeleteDialog() {
    this.$updateState({ bulkDeleteDialogOpen: true, bulkDeleteFailed: [] });
  }

  closeBulkDeleteDialog() {
    this.$updateState({ bulkDeleteDialogOpen: false });
  }

  async bulkDelete(): Promise<{ deleted: number; failed: string[] }> {
    const { selectedTags, image } = this.state;
    if (selectedTags.length === 0) return { deleted: 0, failed: [] };

    this.$updateState({ bulkDeleting: true, bulkDeleteProgress: 0, bulkDeleteFailed: [] });

    const failed: string[] = [];
    const succeeded: string[] = [];
    for (let i = 0; i < selectedTags.length; i++) {
      const tag = selectedTags[i];
      try {
        await deleteImageTag(image, tag);
        succeeded.push(tag);
      } catch {
        failed.push(tag);
      }
      this.$updateState({ bulkDeleteProgress: i + 1 });
    }

    const succeededSet = new Set(succeeded);
    this.$updateState({
      tagList: this.state.tagList.filter((t) => !succeededSet.has(t.tag)),
      selectedTags: failed, // keep failed ones selected so user can retry
      lastSelectedTag: null,
      bulkDeleting: false,
      bulkDeleteDialogOpen: failed.length > 0,
      bulkDeleteFailed: failed,
    });

    return { deleted: succeeded.length, failed };
  }

  setPage(page: number) {
    const clamped = Math.max(0, Math.min(page, this.pageCount - 1));
    this.$updateState({ page: clamped });
  }

  setPageSize(pageSize: number) {
    this.$updateState({ pageSize, page: 0 });
  }

  get pageCount(): number {
    const { tagList, pageSize } = this.state;
    if (pageSize <= 0) return 1;
    return Math.max(1, Math.ceil(tagList.length / pageSize));
  }

  get pagedTagList(): ImageInfo[] {
    const { tagList, page, pageSize } = this.state;
    if (pageSize <= 0) return tagList;
    const start = page * pageSize;
    return tagList.slice(start, start + pageSize);
  }
}
