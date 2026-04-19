/**
 * TagListViewModel - TagList 页面的 ViewModel
 * 管理 Docker 镜像标签列表的状态和业务逻辑
 */

import { BaseViewModel, type ViewModelLifecycle } from '@/lib/viewmodel/BaseViewModel';
import { listImageTags, deleteImageTag, type ImageInfo } from '@/services/registry.service';

/**
 * 排序字段类型
 */
type SortField = 'tag' | 'size' | 'created';

/**
 * 排序方向类型
 */
type SortDirection = 'asc' | 'desc';

/**
 * TagList 状态接口
 */
interface ViewState {
  image: string;
  tagList: ImageInfo[];
  loading: boolean;
  error: string | null;
  sortField: SortField | null;
  sortDirection: SortDirection;
  selectedTag: ImageInfo | null;
  isDrawerOpen: boolean;
  deleteDialogOpen: boolean;
  tagToDelete: ImageInfo | null;
  deleting: boolean;
}

/**
 * TagListViewModel 类
 */
export class TagListViewModel extends BaseViewModel<ViewState> implements ViewModelLifecycle {
  constructor() {
    super({
      image: '',
      tagList: [],
      loading: true,
      error: null,
      sortField: null,
      sortDirection: 'desc',
      selectedTag: null,
      isDrawerOpen: false,
      deleteDialogOpen: false,
      tagToDelete: null,
      deleting: false,
    });
  }

  async setImageName(name: string) {
    this.state.image = name;
    await this.loadTags();
  }

  /**
   * 生命周期：组件挂载（React 组件挂载时调用）
   */
  async $onMounted() {}

  /**
   * Action: 加载标签列表
   */
  private async loadTags() {
    try {
      this.$updateState({
        loading: true,
        error: null,
      });

      // 获取标签列表
      const tagList = await listImageTags(this.state.image);

      this.$updateState({
        tagList,
        loading: false,
      });

      console.log('Tag infos loaded:', tagList.length);
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to fetch tags';
      console.error('Failed to fetch tags:', err);
      this.$updateState({
        error: errorMessage,
        loading: false,
      });
    }
  }

  /**
   * Action: 刷新标签列表
   */
  async refresh() {
    await this.loadTags();
  }

  /**
   * 计算属性：标签数量
   */
  get tagCount(): number {
    return this.state.tagList.length;
  }

  /**
   * Action: 设置排序
   */
  setSorting(field: SortField) {
    const { sortField, sortDirection } = this.state;

    // 如果点击同一列，切换排序方向
    if (sortField === field) {
      this.$updateState({
        sortDirection: sortDirection === 'asc' ? 'desc' : 'asc',
      });
    } else {
      // 如果点击不同列，设置新的排序字段，默认降序
      this.$updateState({
        sortField: field,
        sortDirection: 'desc',
      });
    }

    this.sortTagList();
  }

  /**
   * 内部方法：对标签列表进行排序
   */
  private sortTagList() {
    const { sortField, sortDirection, tagList } = this.state;

    if (!sortField) {
      return;
    }

    const sorted = [...tagList].sort((a, b) => {
      let compareResult = 0;

      switch (sortField) {
        case 'tag':
          compareResult = a.tag.localeCompare(b.tag);
          break;
        case 'size':
          compareResult = a.size - b.size;
          break;
        case 'created': {
          const dateA = a.created ? new Date(a.created).getTime() : 0;
          const dateB = b.created ? new Date(b.created).getTime() : 0;
          compareResult = dateA - dateB;
          break;
        }
      }

      return sortDirection === 'asc' ? compareResult : -compareResult;
    });

    this.$updateState({
      tagList: sorted,
    });
  }

  /**
   * Action: 显示标签详情
   */
  openDrawer(tagInfo: ImageInfo) {
    this.$updateState({
      selectedTag: tagInfo,
      isDrawerOpen: true,
    });
  }

  /**
   * Action: 关闭抽屉
   */
  closeDrawer() {
    this.$updateState({
      isDrawerOpen: false,
      selectedTag: null,
    });
  }

  /**
   * Action: 打开删除确认对话框
   */
  openDeleteDialog(tagInfo: ImageInfo) {
    this.$updateState({
      deleteDialogOpen: true,
      tagToDelete: tagInfo,
    });
  }

  /**
   * Action: 关闭删除确认对话框
   */
  closeDeleteDialog() {
    this.$updateState({
      deleteDialogOpen: false,
      tagToDelete: null,
    });
  }

  /**
   * Action: 删除标签
   */
  async deleteTag() {
    const { tagToDelete, image } = this.state;
    if (!tagToDelete) return;

    try {
      this.$updateState({ deleting: true });

      await deleteImageTag(image, tagToDelete.tag);

      // 从列表中移除已删除的标签
      const updatedTagList = this.state.tagList.filter((tag) => tag.tag !== tagToDelete.tag);

      this.$updateState({
        tagList: updatedTagList,
        deleting: false,
        deleteDialogOpen: false,
        tagToDelete: null,
      });

      console.log('Tag deleted successfully:', tagToDelete.tag);
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to delete tag';
      console.error('Failed to delete tag:', err);
      this.$updateState({
        error: errorMessage,
        deleting: false,
      });
      throw err; // 重新抛出错误以便 UI 处理
    }
  }
}
