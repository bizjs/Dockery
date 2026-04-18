/**
 * CatelogViewModel - Catalog 页面的 ViewModel
 * 管理 Docker 镜像仓库列表的状态和业务逻辑
 */

import { BaseViewModel, type ViewModelLifecycle } from '@/lib/viewmodel/BaseViewModel';
import { listRepositories } from '@/services/registry.service';

/**
 * Catalog 状态接口
 */
interface ViewState {
  repositories: { repo: string; tags: string[] }[];
  filteredRepositories: { repo: string; tags: string[] }[];
  searchQuery: string;
  loading: boolean;
  error: string | null;
}

/**
 * CatalogViewModel 类
 */
export class CatalogViewModel extends BaseViewModel<ViewState> implements ViewModelLifecycle {
  constructor() {
    super({
      repositories: [],
      filteredRepositories: [],
      searchQuery: '',
      loading: true,
      error: null,
    });
  }

  /**
   * 生命周期：组件挂载（React 组件挂载时调用）
   */
  async $onMounted() {
    // 监听搜索查询变化，自动过滤仓库列表
    this.$watch('searchQuery', (query) => {
      this.filterRepositories(query);
    });

    await this.loadRepositories();
  }

  /**
   * Action: 加载仓库列表
   */
  private async loadRepositories() {
    try {
      this.$updateState({
        loading: true,
        error: null,
      });

      const repos = await listRepositories(1000, '');

      this.$updateState({
        repositories: repos,
        filteredRepositories: repos,
        loading: false,
      });
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to fetch repositories';
      this.$updateState({
        error: errorMessage,
        loading: false,
      });
    }
  }

  /**
   * Action: 设置搜索查询
   */
  setSearchQuery(query: string) {
    this.$updateState({
      searchQuery: query,
    });
  }

  /**
   * Action: 过滤仓库列表
   */
  private filterRepositories(query: string) {
    if (!query) {
      this.$updateState({
        filteredRepositories: this.state.repositories,
      });
      return;
    }

    const lowerQuery = query.toLowerCase();
    const filtered = this.state.repositories.filter((repo) => repo.repo.toLowerCase().includes(lowerQuery));
    this.$updateState({
      filteredRepositories: filtered,
    });
  }

  /**
   * Action: 刷新仓库列表
   */
  async refresh() {
    await this.loadRepositories();
  }
}
