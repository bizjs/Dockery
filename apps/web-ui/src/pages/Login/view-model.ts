import { ViewModelBase } from 'bizify';
import { currentUserViewModel } from '@/hooks/use-current-user';
import { ApiError } from '@/services/api';

interface State {
  username: string;
  password: string;
  submitting: boolean;
  error: string | null;
}

export class LoginViewModel extends ViewModelBase<State> {
  protected $data(): State {
    return { username: '', password: '', submitting: false, error: null };
  }

  setUsername(username: string) {
    Object.assign(this.data, { username, error: null });
  }

  setPassword(password: string) {
    Object.assign(this.data, { password, error: null });
  }

  /** Returns true on success so the page component can navigate. */
  async submit(): Promise<boolean> {
    const { username, password } = this.data;
    if (!username || !password) {
      this.data.error = 'Username and password are required';
      return false;
    }
    Object.assign(this.data, { submitting: true, error: null });
    try {
      await currentUserViewModel.login(username, password);
      this.data.submitting = false;
      return true;
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : 'Login failed';
      Object.assign(this.data, { submitting: false, error: msg });
      return false;
    }
  }
}
