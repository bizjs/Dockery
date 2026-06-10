import { ViewModelBase } from 'bizify';
import { authService, type CurrentUser } from '@/services/auth.service';
import { ApiError } from '@/services/api';

interface State {
  user: CurrentUser | null;
  loading: boolean;
  /** Set once the first /me call resolves (regardless of outcome) so
   *  route guards can distinguish "still checking" from "known not
   *  logged in". */
  initialized: boolean;
}

/**
 * CurrentUserViewModel is a singleton that holds the authenticated
 * user. On first mount it hits /api/auth/me. Login/Logout flows mutate
 * this state so the header and route guards reactively update.
 *
 * It's a module-level singleton (not created via `useViewModel`), so
 * bizify's `onMount` lifecycle never fires for it — the `/me` bootstrap
 * is triggered explicitly via `bootstrap()` from AuthGuard instead.
 */
export class CurrentUserViewModel extends ViewModelBase<State> {
  private bootstrapped = false;
  // Request-sequence token (same pattern as CatalogViewModel): login()
  // and logout() bump it so a still-in-flight /me from bootstrap can't
  // land late and overwrite the fresher auth state.
  private reqSeq = 0;

  protected $data(): State {
    return { user: null, loading: false, initialized: false };
  }

  /** Idempotent first-load of /me. Safe to call from multiple mounting
   *  guards / StrictMode double-invokes — only the first wins. */
  bootstrap(): void {
    if (this.bootstrapped) return;
    this.bootstrapped = true;
    void this.refresh();
  }

  async refresh(): Promise<void> {
    const mySeq = ++this.reqSeq;
    this.data.loading = true;
    try {
      const user = await authService.me();
      if (mySeq !== this.reqSeq) return;
      Object.assign(this.data, { user, loading: false, initialized: true });
    } catch (err) {
      if (mySeq !== this.reqSeq) return;
      if (err instanceof ApiError && err.status === 401) {
        Object.assign(this.data, { user: null, loading: false, initialized: true });
        return;
      }
      Object.assign(this.data, { loading: false, initialized: true });
      throw err;
    }
  }

  async login(username: string, password: string): Promise<void> {
    const user = await authService.login(username, password);
    // Invalidate any in-flight /me so its (possibly 401) result can't
    // overwrite the fresh login state.
    this.reqSeq++;
    Object.assign(this.data, { user, initialized: true });
  }

  async logout(): Promise<void> {
    try {
      await authService.logout();
    } finally {
      // Same invalidation in reverse: a stale /me must not resurrect
      // the user after an explicit logout.
      this.reqSeq++;
      this.data.user = null;
    }
  }

  get isAdmin(): boolean {
    return this.data.user?.role === 'admin';
  }
}

// Single shared instance — header, routes, and login page all observe it.
export const currentUserViewModel = new CurrentUserViewModel();
