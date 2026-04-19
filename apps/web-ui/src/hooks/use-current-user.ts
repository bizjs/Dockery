import { BaseViewModel } from '@/lib/viewmodel/BaseViewModel';
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
 */
export class CurrentUserViewModel extends BaseViewModel<State> {
  constructor() {
    super({ user: null, loading: false, initialized: false });
  }

  async $onMounted() {
    // Only bootstrap once. Multiple components may useViewModel this
    // singleton; the $onMounted fires per mount, so guard with
    // initialized flag.
    if (this.state.initialized) return;
    await this.refresh();
  }

  async refresh(): Promise<void> {
    this.$updateState({ loading: true });
    try {
      const user = await authService.me();
      this.$updateState({ user, loading: false, initialized: true });
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        this.$updateState({ user: null, loading: false, initialized: true });
        return;
      }
      this.$updateState({ loading: false, initialized: true });
      throw err;
    }
  }

  async login(username: string, password: string): Promise<void> {
    const user = await authService.login(username, password);
    this.$updateState({ user, initialized: true });
  }

  async logout(): Promise<void> {
    try {
      await authService.logout();
    } finally {
      this.$updateState({ user: null });
    }
  }

  get isAdmin(): boolean {
    return this.state.user?.role === 'admin';
  }
}

// Single shared instance — header, routes, and login page all observe it.
export const currentUserViewModel = new CurrentUserViewModel();
