import { BaseViewModel } from '@/lib/viewmodel/BaseViewModel';
import { userService, type UserView } from '@/services/user.service';
import type { UserRole } from '@/services/auth.service';
import { ApiError } from '@/services/api';

interface State {
  users: UserView[];
  loading: boolean;
  error: string | null;

  // Create-user dialog
  createOpen: boolean;
  createForm: { username: string; password: string; role: UserRole };
  createSubmitting: boolean;
  createError: string | null;

  // Delete confirmation
  deleteTarget: UserView | null;
  deleteSubmitting: boolean;
}

const blankCreate = (): State['createForm'] => ({ username: '', password: '', role: 'view' });

export class UsersViewModel extends BaseViewModel<State> {
  constructor() {
    super({
      users: [],
      loading: true,
      error: null,
      createOpen: false,
      createForm: blankCreate(),
      createSubmitting: false,
      createError: null,
      deleteTarget: null,
      deleteSubmitting: false,
    });
  }

  async $onMounted() {
    await this.refresh();
  }

  async refresh() {
    this.$updateState({ loading: true, error: null });
    try {
      const { items } = await userService.list();
      this.$updateState({ users: items, loading: false });
    } catch (err) {
      this.$updateState({
        loading: false,
        error: err instanceof ApiError ? err.message : 'Failed to load users',
      });
    }
  }

  // --- Create dialog --------------------------------------------------

  openCreate() {
    this.$updateState({
      createOpen: true,
      createForm: blankCreate(),
      createError: null,
    });
  }

  closeCreate() {
    this.$updateState({ createOpen: false, createError: null });
  }

  setCreateField<K extends keyof State['createForm']>(k: K, v: State['createForm'][K]) {
    this.$updateState({ createForm: { ...this.state.createForm, [k]: v } });
  }

  async submitCreate(): Promise<boolean> {
    const { username, password, role } = this.state.createForm;
    if (!username || !password) {
      this.$updateState({ createError: 'Username and password required' });
      return false;
    }
    if (password.length < 8) {
      this.$updateState({ createError: 'Password must be at least 8 characters' });
      return false;
    }
    this.$updateState({ createSubmitting: true, createError: null });
    try {
      const created = await userService.create({ username, password, role });
      this.$updateState({
        users: [...this.state.users, created],
        createOpen: false,
        createSubmitting: false,
      });
      return true;
    } catch (err) {
      this.$updateState({
        createSubmitting: false,
        createError: err instanceof ApiError ? err.message : 'Create failed',
      });
      return false;
    }
  }

  // --- Delete dialog --------------------------------------------------

  askDelete(u: UserView) {
    this.$updateState({ deleteTarget: u });
  }

  cancelDelete() {
    this.$updateState({ deleteTarget: null });
  }

  async confirmDelete(): Promise<void> {
    const target = this.state.deleteTarget;
    if (!target) return;
    this.$updateState({ deleteSubmitting: true });
    try {
      await userService.remove(target.id);
      this.$updateState({
        users: this.state.users.filter((u) => u.id !== target.id),
        deleteTarget: null,
        deleteSubmitting: false,
      });
    } catch (err) {
      this.$updateState({
        deleteSubmitting: false,
        error: err instanceof ApiError ? err.message : 'Delete failed',
      });
    }
  }

  // --- Row actions ----------------------------------------------------

  async toggleDisabled(u: UserView) {
    try {
      const updated = await userService.update(u.id, { disabled: !u.disabled });
      this.$updateState({
        users: this.state.users.map((x) => (x.id === u.id ? updated : x)),
      });
    } catch (err) {
      this.$updateState({
        error: err instanceof ApiError ? err.message : 'Update failed',
      });
    }
  }
}
