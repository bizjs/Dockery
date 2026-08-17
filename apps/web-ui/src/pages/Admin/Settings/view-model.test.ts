import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError } from '@/services/api';
import type { RegistryPolicy } from '@/services/settings.service';
import { SettingsViewModel } from './view-model';

vi.mock('@/services/settings.service', () => ({
  settingsService: {
    getRegistryPolicy: vi.fn(),
    updateRegistryPolicy: vi.fn(),
  },
}));
vi.mock('sonner', () => ({ toast: { success: vi.fn() } }));

import { settingsService } from '@/services/settings.service';

const getPolicy = vi.mocked(settingsService.getRegistryPolicy);
const updatePolicy = vi.mocked(settingsService.updateRegistryPolicy);

const disabledPolicy: RegistryPolicy = {
  prevent_tag_overwrite: false,
  overwrite_exclusions: [],
  version: 0,
  updated_at: 1_786_900_000,
  updated_by: 'system',
};

interface SettingsState {
  loading: boolean;
  saving: boolean;
  error: string | null;
  unknown: boolean;
  policy: RegistryPolicy | null;
  pendingValue: boolean | null;
  exclusionsDraft: string;
  pendingExclusions: string[] | null;
}

const peek = (vm: SettingsViewModel) =>
  vm as unknown as { data: SettingsState };

beforeEach(() => {
  getPolicy.mockReset();
  updatePolicy.mockReset();
});

describe('SettingsViewModel', () => {
  it('loads the authoritative default-off policy', async () => {
    getPolicy.mockResolvedValue(disabledPolicy);
    const vm = new SettingsViewModel();

    await vm.reload();

    expect(peek(vm).data.policy).toEqual(disabledPolicy);
    expect(peek(vm).data.unknown).toBe(false);
    expect(peek(vm).data.loading).toBe(false);
  });

  it('reloads after a version conflict instead of retrying stale data', async () => {
    const current = { ...disabledPolicy, prevent_tag_overwrite: true, version: 1, updated_by: 'other-admin' };
    getPolicy.mockResolvedValueOnce(disabledPolicy).mockResolvedValueOnce(current);
    updatePolicy.mockRejectedValue(new ApiError(409, 40902, 'conflict'));
    const vm = new SettingsViewModel();
    await vm.reload();

    vm.requestChange(true);
    await vm.confirmChange();

    expect(updatePolicy).toHaveBeenCalledWith(true, [], 0);
    expect(updatePolicy).toHaveBeenCalledTimes(1);
    expect(peek(vm).data.policy).toEqual(current);
    expect(peek(vm).data.unknown).toBe(false);
    expect(peek(vm).data.error).toContain('another administrator');
  });

  it('normalizes and persists exact overwrite exceptions', async () => {
    const enabledPolicy: RegistryPolicy = {
      ...disabledPolicy,
      prevent_tag_overwrite: true,
      version: 1,
      updated_by: 'admin',
    };
    const updated: RegistryPolicy = {
      ...enabledPolicy,
      prevent_tag_overwrite: true,
      overwrite_exclusions: ['latest', 'nightly'],
      version: 2,
      updated_by: 'admin',
    };
    getPolicy.mockResolvedValue(enabledPolicy);
    updatePolicy.mockResolvedValue(updated);
    const vm = new SettingsViewModel();
    await vm.reload();

    vm.setExclusionsDraft('nightly, latest, latest');
    vm.requestExclusionsChange();
    expect(peek(vm).data.pendingExclusions).toEqual(['latest', 'nightly']);
    await vm.confirmExclusionsChange();

    expect(updatePolicy).toHaveBeenCalledWith(true, ['latest', 'nightly'], 1);
    expect(peek(vm).data.policy).toEqual(updated);
    expect(peek(vm).data.exclusionsDraft).toBe('latest, nightly');
  });

  it('rejects wildcard overwrite exceptions before PATCH', async () => {
    getPolicy.mockResolvedValue({ ...disabledPolicy, prevent_tag_overwrite: true, version: 1 });
    const vm = new SettingsViewModel();
    await vm.reload();

    vm.setExclusionsDraft('release-*');
    vm.requestExclusionsChange();

    expect(updatePolicy).not.toHaveBeenCalled();
    expect(peek(vm).data.pendingExclusions).toBeNull();
    expect(peek(vm).data.error).toContain('Invalid tag name');
  });

  it('does not allow editing exceptions while protection is disabled', async () => {
    getPolicy.mockResolvedValue(disabledPolicy);
    const vm = new SettingsViewModel();
    await vm.reload();

    vm.setExclusionsDraft('latest');
    vm.requestExclusionsChange();

    expect(peek(vm).data.exclusionsDraft).toBe('');
    expect(peek(vm).data.pendingExclusions).toBeNull();
    expect(updatePolicy).not.toHaveBeenCalled();
  });

  it('marks the switch unknown when PATCH and reconciliation GET both fail', async () => {
    getPolicy.mockResolvedValueOnce(disabledPolicy).mockRejectedValueOnce(new Error('offline'));
    updatePolicy.mockRejectedValue(new ApiError(503, 50301, 'timeout'));
    const vm = new SettingsViewModel();
    await vm.reload();

    vm.requestChange(true);
    await vm.confirmChange();

    expect(peek(vm).data.policy).toEqual(disabledPolicy);
    expect(peek(vm).data.unknown).toBe(true);
    expect(peek(vm).data.error).toContain('result is unknown');
  });
});
