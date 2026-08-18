import { ViewModelBase } from 'bizify';
import { toast } from 'sonner';

import { ApiError } from '@/services/api';
import { settingsService, type RegistryPolicy } from '@/services/settings.service';

const tagPattern = /^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$/;

interface State {
  loading: boolean;
  saving: boolean;
  error: string | null;
  /** True when the last mutation result could not be confirmed by GET. */
  unknown: boolean;
  policy: RegistryPolicy | null;
  pendingValue: boolean | null;
  exclusionsDraft: string;
  pendingExclusions: string[] | null;
}

function normalizeExclusions(raw: string): string[] {
  const tags = raw
    .split(/[,\n]/)
    .map((tag) => tag.trim())
    .filter(Boolean);
  if (tags.length > 128) {
    throw new Error('At most 128 overwrite exceptions are allowed.');
  }
  const invalid = tags.find((tag) => !tagPattern.test(tag));
  if (invalid) {
    throw new Error(`Invalid tag name: ${invalid}`);
  }
  return [...new Set(tags)].sort();
}

function exclusionsDraft(policy: RegistryPolicy): string {
  return policy.overwrite_exclusions.join(', ');
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
      exclusionsDraft: '',
      pendingExclusions: null,
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

  setExclusionsDraft(value: string) {
    if (!this.data.saving && this.data.policy?.prevent_tag_overwrite) {
      this.data.exclusionsDraft = value;
    }
  }

  requestExclusionsChange() {
    const policy = this.data.policy;
    if (this.data.loading || this.data.saving || !policy?.prevent_tag_overwrite) return;
    try {
      const normalized = normalizeExclusions(this.data.exclusionsDraft);
      if (JSON.stringify(normalized) === JSON.stringify(policy.overwrite_exclusions)) {
        Object.assign(this.data, { exclusionsDraft: normalized.join(', '), error: null });
        return;
      }
      Object.assign(this.data, { pendingExclusions: normalized, error: null });
    } catch (err) {
      this.data.error = err instanceof Error ? err.message : 'Invalid overwrite exceptions';
    }
  }

  cancelExclusionsChange() {
    if (!this.data.saving) this.data.pendingExclusions = null;
  }

  async reload(): Promise<void> {
    Object.assign(this.data, { loading: true, error: null });
    try {
      const policy = await settingsService.getRegistryPolicy();
      Object.assign(this.data, {
        loading: false,
        policy,
        exclusionsDraft: exclusionsDraft(policy),
        unknown: false,
      });
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

    await this.persistPolicy(
      value,
      [...policy.overwrite_exclusions],
      value ? 'Tag overwrite protection enabled' : 'Tag overwrite protection disabled',
    );
  }

  async confirmExclusionsChange(): Promise<void> {
    const policy = this.data.policy;
    const exclusions = this.data.pendingExclusions;
    if (!policy || exclusions === null || this.data.saving) return;
    await this.persistPolicy(
      policy.prevent_tag_overwrite,
      exclusions,
      exclusions.length === 0 ? 'All overwrite exceptions removed' : 'Overwrite exceptions updated',
    );
  }

  private async persistPolicy(
    preventTagOverwrite: boolean,
    overwriteExclusions: string[],
    successMessage: string,
  ): Promise<void> {
    const policy = this.data.policy;
    if (!policy) return;

    Object.assign(this.data, {
      saving: true,
      pendingValue: null,
      pendingExclusions: null,
      error: null,
    });
    try {
      const updated = await settingsService.updateRegistryPolicy(
        preventTagOverwrite,
        overwriteExclusions,
        policy.version,
      );
      Object.assign(this.data, {
        saving: false,
        policy: updated,
        exclusionsDraft: exclusionsDraft(updated),
        unknown: false,
      });
      toast.success(successMessage);
    } catch (err) {
      const conflict = err instanceof ApiError && err.status === 409;
      // A timed-out or failed PATCH may still have reached the server. Always
      // reload the authoritative state instead of visually rolling back.
      try {
        const current = await settingsService.getRegistryPolicy();
        Object.assign(this.data, {
          saving: false,
          policy: current,
          exclusionsDraft: exclusionsDraft(current),
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
