import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { CatalogViewModel } from './view-model';

// Mock the entire service module — getOverview is the only import the
// VM uses, and we replace it with a controllable spy.
vi.mock('@/services/registry.service', () => ({
  getOverview: vi.fn(),
}));

// Import the mocked fn after vi.mock so the spy reference is stable.
import { getOverview } from '@/services/registry.service';
const getOverviewMock = vi.mocked(getOverview);

const emptyResponse = { items: [], total: 0, page: 0, page_size: 50 };

beforeEach(() => {
  vi.useFakeTimers();
  getOverviewMock.mockReset();
  getOverviewMock.mockResolvedValue(emptyResponse);
});

afterEach(() => {
  vi.useRealTimers();
});

// Constructing the VM calls $onMounted → fetch in the test-runner's
// microtask; wait for it to settle so the beforeAll fetch is out of
// the way before each test exercises the real behaviour.
async function newVM(): Promise<CatalogViewModel> {
  const vm = new CatalogViewModel();
  // $onMounted has to be invoked manually in test — it's normally
  // called by useViewModel which the component layer hooks up.
  await vm.$onMounted?.();
  getOverviewMock.mockClear();
  return vm;
}

describe('CatalogViewModel', () => {
  describe('setSearchQuery', () => {
    it('debounces fetch calls on rapid typing', async () => {
      const vm = await newVM();

      vm.setSearchQuery('a');
      vm.setSearchQuery('al');
      vm.setSearchQuery('ali');
      vm.setSearchQuery('alic');
      vm.setSearchQuery('alice');

      // State updates are synchronous so the input box stays responsive.
      expect(vm.state.searchQuery).toBe('alice');
      // Fetch must NOT have fired yet.
      expect(getOverviewMock).not.toHaveBeenCalled();

      // Advance past the 300 ms debounce window.
      await vi.advanceTimersByTimeAsync(300);

      // Exactly one fetch, with the final search term.
      expect(getOverviewMock).toHaveBeenCalledTimes(1);
      expect(getOverviewMock).toHaveBeenCalledWith(
        expect.objectContaining({ q: 'alice' }),
      );
    });

    it('resets page to 0 when search changes', async () => {
      const vm = await newVM();
      // Jump to a non-zero page first.
      vm.setPage(3);
      expect(vm.state.page).toBe(3);
      await vi.advanceTimersByTimeAsync(0);

      vm.setSearchQuery('foo');
      expect(vm.state.page).toBe(0);
    });
  });

  describe('toggleSort', () => {
    it('flips direction when clicking the active column', async () => {
      const vm = await newVM();
      // Default is { sort: 'updated', direction: 'desc' } — newest first.
      expect(vm.state.sort).toBe('updated');
      expect(vm.state.sortDirection).toBe('desc');

      vm.toggleSort('updated');
      expect(vm.state.sortDirection).toBe('asc');

      vm.toggleSort('updated');
      expect(vm.state.sortDirection).toBe('desc');
    });

    it('switches to column-appropriate default direction when changing columns', async () => {
      const vm = await newVM();
      // Size → desc (biggest first)
      vm.toggleSort('size');
      expect(vm.state.sort).toBe('size');
      expect(vm.state.sortDirection).toBe('desc');

      // Back to name → asc
      vm.toggleSort('name');
      expect(vm.state.sort).toBe('name');
      expect(vm.state.sortDirection).toBe('asc');

      // Updated → desc (newest first)
      vm.toggleSort('updated');
      expect(vm.state.sortDirection).toBe('desc');

      // Tags → desc (most first)
      vm.toggleSort('tags');
      expect(vm.state.sortDirection).toBe('desc');
    });

    it('resets page to 0 on sort change', async () => {
      const vm = await newVM();
      vm.setPage(2);
      expect(vm.state.page).toBe(2);

      vm.toggleSort('size');
      expect(vm.state.page).toBe(0);
    });

    it('cancels a pending debounced search fetch', async () => {
      const vm = await newVM();
      vm.setSearchQuery('alice');
      // Immediately toggle sort — the pending debounced search should
      // be cancelled, leaving only toggleSort's immediate fetch.
      vm.toggleSort('size');

      // Advance past the debounce; if cancel failed, a second fetch
      // would fire here.
      await vi.advanceTimersByTimeAsync(300);

      expect(getOverviewMock).toHaveBeenCalledTimes(1);
      expect(getOverviewMock).toHaveBeenCalledWith(
        expect.objectContaining({ sort: 'size', direction: 'desc' }),
      );
    });
  });

  describe('setPageSize', () => {
    it('resets page to 0', async () => {
      const vm = await newVM();
      vm.setPage(5);
      expect(vm.state.page).toBe(5);

      vm.setPageSize(100);
      expect(vm.state.page).toBe(0);
      expect(vm.state.pageSize).toBe(100);
    });
  });

  describe('setPage', () => {
    it('clamps negative page to 0', async () => {
      const vm = await newVM();
      vm.setPage(-5);
      expect(vm.state.page).toBe(0);
    });

    it('fires fetch immediately without debounce', async () => {
      const vm = await newVM();
      vm.setPage(1);
      // Synchronous — no debounce, fetch is awaited on microtask queue.
      await vi.runAllTimersAsync();
      expect(getOverviewMock).toHaveBeenCalledTimes(1);
      expect(getOverviewMock).toHaveBeenCalledWith(
        expect.objectContaining({ page: 1 }),
      );
    });
  });

  describe('refresh', () => {
    it('fires fetch with current params and cancels pending debounce', async () => {
      const vm = await newVM();
      vm.setSearchQuery('foo');
      // Don't wait for debounce; refresh should cancel it.
      await vm.refresh();
      await vi.advanceTimersByTimeAsync(300);

      expect(getOverviewMock).toHaveBeenCalledTimes(1);
      expect(getOverviewMock).toHaveBeenCalledWith(
        expect.objectContaining({ q: 'foo' }),
      );
    });
  });

});
