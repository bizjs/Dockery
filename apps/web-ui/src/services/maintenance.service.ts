import { api } from './api';

/** Response shape returned by POST /api/admin/gc. */
export interface GCResult {
  started: boolean;
  duration_ms: number;
  /** Last ~20 lines of `registry garbage-collect` output. Empty for no-op runs. */
  output_tail?: string;
}

export const maintenanceService = {
  /**
   * Kick off a garbage-collect cycle. Blocks until the registry has been
   * stopped, GC has finished, and the registry has been restarted — can
   * take minutes on large registries. The caller should show a clear
   * "running" state and be prepared for a long-running fetch.
   *
   * Server rejects concurrent calls with 409 (ApiError.status === 409).
   */
  triggerGC: () => api.post<GCResult>('/api/admin/gc'),
};
