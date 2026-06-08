import { ViewModelBase } from 'bizify';
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

export class AuditViewModel extends ViewModelBase<State> {
  protected $data(): State {
    return {
      form: blankForm(),
      applied: blankForm(),
      limit: 50,
      offset: 0,
      loading: true,
      error: null,
      items: [],
      total: 0,
      expanded: new Set(),
    };
  }

  protected onMount() {
    void this.reload();
  }

  // --- Form controls ---------------------------------------------------

  setField<K extends keyof FormState>(k: K, v: FormState[K]) {
    this.data.form = { ...this.data.form, [k]: v };
  }

  /** Apply current form + reset to first page. */
  async applyFilters(): Promise<void> {
    Object.assign(this.data, {
      applied: { ...this.data.form },
      offset: 0,
    });
    await this.reload();
  }

  async clearFilters(): Promise<void> {
    Object.assign(this.data, {
      form: blankForm(),
      applied: blankForm(),
      offset: 0,
    });
    await this.reload();
  }

  async setLimit(n: number): Promise<void> {
    Object.assign(this.data, { limit: n, offset: 0 });
    await this.reload();
  }

  // --- Pagination ------------------------------------------------------

  async nextPage(): Promise<void> {
    const next = this.data.offset + this.data.limit;
    if (next >= this.data.total) return;
    this.data.offset = next;
    await this.reload();
  }

  async prevPage(): Promise<void> {
    const prev = Math.max(0, this.data.offset - this.data.limit);
    if (prev === this.data.offset) return;
    this.data.offset = prev;
    await this.reload();
  }

  // --- Row actions -----------------------------------------------------

  toggleExpand(id: number) {
    const next = new Set(this.data.expanded);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    this.data.expanded = next;
  }

  // --- Fetch -----------------------------------------------------------

  async reload(): Promise<void> {
    Object.assign(this.data, { loading: true, error: null });
    try {
      const { items, total } = await auditService.list({
        actor: this.data.applied.actor || undefined,
        action: this.data.applied.action || undefined,
        since: dtToUnix(this.data.applied.since) || undefined,
        until: dtToUnix(this.data.applied.until) || undefined,
        limit: this.data.limit,
        offset: this.data.offset,
      });
      Object.assign(this.data, {
        items,
        total,
        loading: false,
        expanded: new Set(), // collapse on page change — detail rarely matters after filter swap
      });
    } catch (err) {
      Object.assign(this.data, {
        loading: false,
        error: err instanceof ApiError ? err.message : 'Failed to load audit log',
      });
    }
  }
}
