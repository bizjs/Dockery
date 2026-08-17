import { ViewModelBase } from 'bizify';
import { toast } from 'sonner';

import { ApiError } from '@/services/api';
import { settingsService, type RegistryPolicy } from '@/services/settings.service';

interface State {
  loading: boolean;
  saving: boolean;
  error: string | null;
  /** True when the last mutation result could not be confirmed by GET. */
  unknown: boolean;
  policy: RegistryPolicy | null;
  pendingValue: boolean | null;
}

export class SettingsViewModel extends ViewModelBase<State> {
  protected $data(): State {
    return {
      loading: true,
      saving: false,
      error: null,
      unknown: false,
      policy: null,
      pendingValue: null,
    };
  }

  protected onMount() {
    void this.reload();
  }

  requestChange(value: boolean) {
    if (this.data.loading || this.data.saving || !this.data.policy) return;
    this.data.pendingValue = value;
  }

  cancelChange() {
    if (!this.data.saving) this.data.pendingValue = null;
  }

  async reload(): Promise<void> {
    Object.assign(this.data, { loading: true, error: null });
    try {
      const policy = await settingsService.getRegistryPolicy();
      Object.assign(this.data, { loading: false, policy, unknown: false });
    } catch (err) {
      Object.assign(this.data, {
        loading: false,
        unknown: true,
        error: err instanceof ApiError ? err.message : 'Failed to load registry settings',
      });
    }
  }

  async confirmChange(): Promise<void> {
    const policy = this.data.policy;
    const value = this.data.pendingValue;
    if (!policy || value === null || this.data.saving) return;

    Object.assign(this.data, { saving: true, pendingValue: null, error: null });
    try {
      const updated = await settingsService.updateRegistryPolicy(value, policy.version);
      Object.assign(this.data, { saving: false, policy: updated });
      toast.success(value ? 'Tag overwrite protection enabled' : 'Tag overwrite protection disabled');
    } catch (err) {
      const conflict = err instanceof ApiError && err.status === 409;
      // A timed-out or failed PATCH may still have reached the server. Always
      // reload the authoritative state instead of visually rolling back.
      try {
        const current = await settingsService.getRegistryPolicy();
        Object.assign(this.data, {
          saving: false,
          policy: current,
          unknown: false,
          error: conflict
            ? 'Policy was changed by another administrator. Review the current value and try again.'
            : err instanceof ApiError
              ? `Update failed: ${err.message}. The current server value has been reloaded.`
              : 'Update failed. The current server value has been reloaded.',
        });
      } catch {
        Object.assign(this.data, {
          saving: false,
          unknown: true,
          error: 'The update result is unknown and the current server value could not be loaded. Retry before making another change.',
        });
      }
    }
  }
}
