import { api } from './api';

/** Response shape returned by POST /api/admin/gc.
 *  success=false means the stop/gc/restart sequence failed somewhere;
 *  error holds a short reason, output_tail the command output. */
export interface GCResult {
  success: boolean;
  duration_ms: number;
  error?: string;
  /** Last ~40 lines of supervisorctl / `registry garbage-collect` output. */
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
