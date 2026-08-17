import { api } from './api';

export interface RegistryPolicy {
  prevent_tag_overwrite: boolean;
  overwrite_exclusions: string[];
  version: number;
  /** Unix seconds. */
  updated_at: number;
  updated_by: string;
}

export const settingsService = {
  getRegistryPolicy(): Promise<RegistryPolicy> {
    return api.get<RegistryPolicy>('/api/admin/registry-policy');
  },

  updateRegistryPolicy(
    preventTagOverwrite: boolean,
    overwriteExclusions: string[],
    version: number,
  ): Promise<RegistryPolicy> {
    return api.patch<RegistryPolicy>('/api/admin/registry-policy', {
      prevent_tag_overwrite: preventTagOverwrite,
      overwrite_exclusions: overwriteExclusions,
      version,
    });
  },
};
