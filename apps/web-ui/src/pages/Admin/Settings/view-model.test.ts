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

    expect(updatePolicy).toHaveBeenCalledWith(true, 0);
    expect(updatePolicy).toHaveBeenCalledTimes(1);
    expect(peek(vm).data.policy).toEqual(current);
    expect(peek(vm).data.unknown).toBe(false);
    expect(peek(vm).data.error).toContain('another administrator');
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
