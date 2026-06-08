/**
 * TagListViewModel - TagList 页面的 ViewModel
 * 管理 Docker 镜像标签列表的状态和业务逻辑
 */

import { ViewModelBase } from 'bizify';
import { listImageTags, deleteImageTag, type ImageInfo } from '@/services/registry.service';
import { compareTags } from './sort';

type SortField = 'tag' | 'size' | 'created';
type SortDirection = 'asc' | 'desc';

// applySort is the single ordering function for both the initial fetch
// and user-driven re-sorts. Operates on a plain array (not a Valtio
// proxy) so the comparator never reads through proxy traps — that
// matters because the proxied array items can briefly look stale on
// the microtask boundary between two consecutive state writes.
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

export class TagListViewModel extends ViewModelBase<ViewState> {
  protected $data(): ViewState {
    return {
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
    };
  }

  async setImageName(name: string) {
    this.data.image = name;
    await this.loadTags();
  }

  private async loadTags() {
    try {
      Object.assign(this.data, { loading: true, error: null });
      const tagList = await listImageTags(this.data.image);
      // Sort BEFORE committing to state so the table never flashes the
      // upstream order (which is undefined; distribution doesn't sort).
      // The previous two-step "commit then sort" briefly painted the
      // unsorted list and — with Valtio's array proxying — could leak
      // through if a snapshot was taken between the two writes.
      const sorted = applySort(tagList, this.data.sortField, this.data.sortDirection);
      Object.assign(this.data, {
        tagList: sorted,
        loading: false,
        page: 0,
        selectedTags: [],
        lastSelectedTag: null,
      });
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to fetch tags';
      Object.assign(this.data, { error: errorMessage, loading: false });
    }
  }

  async refresh() {
    await this.loadTags();
  }

  get tagCount(): number {
    return this.data.tagList.length;
  }

  setSorting(field: SortField) {
    const { sortField, sortDirection, tagList } = this.data;
    const nextDir: SortDirection =
      sortField === field ? (sortDirection === 'asc' ? 'desc' : 'asc') : 'desc';
    // Sort on a plain-array copy of the current proxy state, then
    // commit field + direction + tagList together. Combining the
    // writes makes the sort atomic to subscribers.
    const sorted = applySort([...tagList], field, nextDir);
    Object.assign(this.data, {
      sortField: field,
      sortDirection: nextDir,
      tagList: sorted,
      page: 0,
    });
  }

  openDrawer(tagInfo: ImageInfo) {
    Object.assign(this.data, { selectedTag: tagInfo, isDrawerOpen: true });
  }

  closeDrawer() {
    Object.assign(this.data, { isDrawerOpen: false, selectedTag: null });
  }

  openDeleteDialog(tagInfo: ImageInfo) {
    Object.assign(this.data, { deleteDialogOpen: true, tagToDelete: tagInfo });
  }

  closeDeleteDialog() {
    Object.assign(this.data, { deleteDialogOpen: false, tagToDelete: null });
  }

  async deleteTag() {
    const { tagToDelete, image } = this.data;
    if (!tagToDelete) return;
    try {
      this.data.deleting = true;
      await deleteImageTag(image, tagToDelete.tag);
      Object.assign(this.data, {
        tagList: this.data.tagList.filter((t) => t.tag !== tagToDelete.tag),
        selectedTags: this.data.selectedTags.filter((t) => t !== tagToDelete.tag),
        deleting: false,
        deleteDialogOpen: false,
        tagToDelete: null,
      });
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to delete tag';
      Object.assign(this.data, { error: errorMessage, deleting: false });
      throw err;
    }
  }

  /**
   * Toggle selection for a single tag. When `shift` is true, select the
   * inclusive range between the last-clicked tag and this one (always
   * additive, matching joxit's Shift+Click behaviour).
   */
  toggleTagSelection(tag: string, opts?: { shift?: boolean }) {
    const { tagList, selectedTags, lastSelectedTag } = this.data;

    if (opts?.shift && lastSelectedTag && lastSelectedTag !== tag) {
      const currIdx = tagList.findIndex((t) => t.tag === tag);
      const prevIdx = tagList.findIndex((t) => t.tag === lastSelectedTag);
      if (currIdx >= 0 && prevIdx >= 0) {
        const [from, to] = currIdx < prevIdx ? [currIdx, prevIdx] : [prevIdx, currIdx];
        const next = new Set(selectedTags);
        for (let i = from; i <= to; i++) next.add(tagList[i].tag);
        Object.assign(this.data, { selectedTags: Array.from(next), lastSelectedTag: tag });
        return;
      }
    }

    const next = new Set(selectedTags);
    if (next.has(tag)) next.delete(tag);
    else next.add(tag);
    Object.assign(this.data, { selectedTags: Array.from(next), lastSelectedTag: tag });
  }

  /** Toggle "select all on current page" — adds if any unchecked, removes if all checked. */
  toggleSelectPage(pageTags: string[]) {
    if (pageTags.length === 0) return;
    const { selectedTags } = this.data;
    const current = new Set(selectedTags);
    const allSelected = pageTags.every((t) => current.has(t));
    if (allSelected) {
      pageTags.forEach((t) => current.delete(t));
    } else {
      pageTags.forEach((t) => current.add(t));
    }
    this.data.selectedTags = Array.from(current);
  }

  clearSelection() {
    Object.assign(this.data, { selectedTags: [], lastSelectedTag: null });
  }

  openBulkDeleteDialog() {
    Object.assign(this.data, { bulkDeleteDialogOpen: true, bulkDeleteFailed: [] });
  }

  closeBulkDeleteDialog() {
    this.data.bulkDeleteDialogOpen = false;
  }

  async bulkDelete(): Promise<{ deleted: number; failed: string[] }> {
    const { selectedTags, image } = this.data;
    if (selectedTags.length === 0) return { deleted: 0, failed: [] };

    Object.assign(this.data, { bulkDeleting: true, bulkDeleteProgress: 0, bulkDeleteFailed: [] });

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
      this.data.bulkDeleteProgress = i + 1;
    }

    const succeededSet = new Set(succeeded);
    Object.assign(this.data, {
      tagList: this.data.tagList.filter((t) => !succeededSet.has(t.tag)),
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
    this.data.page = clamped;
  }

  setPageSize(pageSize: number) {
    Object.assign(this.data, { pageSize, page: 0 });
  }

  get pageCount(): number {
    const { tagList, pageSize } = this.data;
    if (pageSize <= 0) return 1;
    return Math.max(1, Math.ceil(tagList.length / pageSize));
  }

  get pagedTagList(): ImageInfo[] {
    const { tagList, page, pageSize } = this.data;
    if (pageSize <= 0) return tagList;
    const start = page * pageSize;
    return tagList.slice(start, start + pageSize);
  }
}
