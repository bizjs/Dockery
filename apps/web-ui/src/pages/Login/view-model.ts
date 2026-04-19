import { BaseViewModel } from '@/lib/viewmodel/BaseViewModel';
import { currentUserViewModel } from '@/hooks/use-current-user';
import { ApiError } from '@/services/api';

interface State {
  username: string;
  password: string;
  submitting: boolean;
  error: string | null;
}

export class LoginViewModel extends BaseViewModel<State> {
  constructor() {
    super({ username: '', password: '', submitting: false, error: null });
  }

  setUsername(username: string) {
    this.$updateState({ username, error: null });
  }

  setPassword(password: string) {
    this.$updateState({ password, error: null });
  }

  /** Returns true on success so the page component can navigate. */
  async submit(): Promise<boolean> {
    const { username, password } = this.state;
    if (!username || !password) {
      this.$updateState({ error: 'Username and password are required' });
      return false;
    }
    this.$updateState({ submitting: true, error: null });
    try {
      await currentUserViewModel.login(username, password);
      this.$updateState({ submitting: false });
      return true;
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : 'Login failed';
      this.$updateState({ submitting: false, error: msg });
      return false;
    }
  }
}
