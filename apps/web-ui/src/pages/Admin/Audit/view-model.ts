import { BaseViewModel } from '@/lib/viewmodel/BaseViewModel';
import { auditService, type AuditEntry, type AuditAction } from '@/services/audit.service';
import { ApiError } from '@/services/api';

interface FormState {
  /** free-text actor (substring match, case-insensitive on the server) */
  actor: string;
  /** action enum; '' = any */
  action: AuditAction | '';
  /** datetime-local string, e.g. '2026-04-18T10:30'. Empty = no bound. */
  since: string;
  until: string;
}

interface State {
  form: FormState;
  /** Values actually sent on the last fetch — used for pagination. */
  applied: FormState;
  limit: number;
  offset: number;

  loading: boolean;
  error: string | null;
  items: AuditEntry[];
  total: number;

  /** IDs of rows whose detail JSON is expanded. */
  expanded: Set<number>;
}

const blankForm = (): FormState => ({ actor: '', action: '', since: '', until: '' });

/** Convert a datetime-local string (in the user's timezone) to unix seconds.
 *  Empty/invalid → 0 (server treats 0 as "no bound"). */
function dtToUnix(dt: string): number {
  if (!dt) return 0;
  const ms = new Date(dt).getTime();
  return Number.isFinite(ms) ? Math.floor(ms / 1000) : 0;
}

export class AuditViewModel extends BaseViewModel<State> {
  constructor() {
    super({
      form: blankForm(),
      applied: blankForm(),
      limit: 50,
      offset: 0,
      loading: true,
      error: null,
      items: [],
      total: 0,
      expanded: new Set(),
    });
  }

  async $onMounted() {
    await this.reload();
  }

  // --- Form controls ---------------------------------------------------

  setField<K extends keyof FormState>(k: K, v: FormState[K]) {
    this.$updateState({ form: { ...this.state.form, [k]: v } });
  }

  /** Apply current form + reset to first page. */
  async applyFilters(): Promise<void> {
    this.$updateState({
      applied: { ...this.state.form },
      offset: 0,
    });
    await this.reload();
  }

  async clearFilters(): Promise<void> {
    this.$updateState({
      form: blankForm(),
      applied: blankForm(),
      offset: 0,
    });
    await this.reload();
  }

  async setLimit(n: number): Promise<void> {
    this.$updateState({ limit: n, offset: 0 });
    await this.reload();
  }

  // --- Pagination ------------------------------------------------------

  async nextPage(): Promise<void> {
    const next = this.state.offset + this.state.limit;
    if (next >= this.state.total) return;
    this.$updateState({ offset: next });
    await this.reload();
  }

  async prevPage(): Promise<void> {
    const prev = Math.max(0, this.state.offset - this.state.limit);
    if (prev === this.state.offset) return;
    this.$updateState({ offset: prev });
    await this.reload();
  }

  // --- Row actions -----------------------------------------------------

  toggleExpand(id: number) {
    const next = new Set(this.state.expanded);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    this.$updateState({ expanded: next });
  }

  // --- Fetch -----------------------------------------------------------

  async reload(): Promise<void> {
    this.$updateState({ loading: true, error: null });
    try {
      const { items, total } = await auditService.list({
        actor: this.state.applied.actor || undefined,
        action: this.state.applied.action || undefined,
        since: dtToUnix(this.state.applied.since) || undefined,
        until: dtToUnix(this.state.applied.until) || undefined,
        limit: this.state.limit,
        offset: this.state.offset,
      });
      this.$updateState({
        items,
        total,
        loading: false,
        expanded: new Set(), // collapse on page change — detail rarely matters after filter swap
      });
    } catch (err) {
      this.$updateState({
        loading: false,
        error: err instanceof ApiError ? err.message : 'Failed to load audit log',
      });
    }
  }
}
