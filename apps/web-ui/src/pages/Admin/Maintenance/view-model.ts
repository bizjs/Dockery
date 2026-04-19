import { BaseViewModel } from '@/lib/viewmodel/BaseViewModel';
import { maintenanceService, type GCResult } from '@/services/maintenance.service';
import { ApiError } from '@/services/api';

interface State {
  confirmOpen: boolean;
  running: boolean;
  /** Most recent successful GC response, kept for post-run display. */
  lastResult: GCResult | null;
  error: string | null;
}

/** MaintenanceViewModel tracks the single-flight GC action and its outcome.
 *  The page is admin-only (route guard) so we don't re-check roles here. */
export class MaintenanceViewModel extends BaseViewModel<State> {
  constructor() {
    super({
      confirmOpen: false,
      running: false,
      lastResult: null,
      error: null,
    });
  }

  openConfirm() {
    this.$updateState({ confirmOpen: true, error: null });
  }

  closeConfirm() {
    if (this.state.running) return;
    this.$updateState({ confirmOpen: false });
  }

  async triggerGC(): Promise<void> {
    this.$updateState({
      confirmOpen: false,
      running: true,
      error: null,
    });
    try {
      const result = await maintenanceService.triggerGC();
      this.$updateState({ running: false, lastResult: result });
    } catch (err) {
      let message = 'Garbage collection failed';
      if (err instanceof ApiError) {
        if (err.status === 409) {
          message = 'Another garbage collection is already in progress. Try again shortly.';
        } else {
          message = err.message;
        }
      }
      this.$updateState({ running: false, error: message });
    }
  }
}
