import { BaseViewModel } from '@/lib/viewmodel/BaseViewModel';
import { userService, type UserView } from '@/services/user.service';
import { permissionService, type PermissionView } from '@/services/permission.service';
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

  // Password reset dialog (admin-driven — no old_password needed)
  pwTarget: UserView | null;
  pwForm: { newPassword: string };
  pwSubmitting: boolean;
  pwError: string | null;

  // Permissions drawer
  permsTarget: UserView | null;
  permsLoading: boolean;
  permsError: string | null;
  perms: PermissionView[];
  permsAddText: string;
  permsAddSubmitting: boolean;
  permsEditingId: number | null;
  permsEditText: string;
  permsRowBusyId: number | null;
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
      pwTarget: null,
      pwForm: { newPassword: '' },
      pwSubmitting: false,
      pwError: null,
      permsTarget: null,
      permsLoading: false,
      permsError: null,
      perms: [],
      permsAddText: '',
      permsAddSubmitting: false,
      permsEditingId: null,
      permsEditText: '',
      permsRowBusyId: null,
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
        deleteTarget: null,
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

  /** Change a user's role inline. Admin-demotion guard is enforced server-side. */
  async setRole(u: UserView, role: UserRole) {
    if (u.role === role) return;
    try {
      const updated = await userService.update(u.id, { role });
      this.$updateState({
        users: this.state.users.map((x) => (x.id === u.id ? updated : x)),
        error: null,
      });
    } catch (err) {
      this.$updateState({
        error: err instanceof ApiError ? err.message : 'Role change failed',
      });
    }
  }

  // --- Password reset (admin-driven) ---------------------------------

  askResetPassword(u: UserView) {
    this.$updateState({
      pwTarget: u,
      pwForm: { newPassword: '' },
      pwError: null,
    });
  }

  cancelResetPassword() {
    this.$updateState({ pwTarget: null, pwError: null });
  }

  setPwField<K extends keyof State['pwForm']>(k: K, v: State['pwForm'][K]) {
    this.$updateState({ pwForm: { ...this.state.pwForm, [k]: v } });
  }

  async submitResetPassword(): Promise<boolean> {
    const target = this.state.pwTarget;
    if (!target) return false;
    const { newPassword } = this.state.pwForm;
    if (newPassword.length < 8) {
      this.$updateState({ pwError: 'Password must be at least 8 characters' });
      return false;
    }
    this.$updateState({ pwSubmitting: true, pwError: null });
    try {
      await userService.setPassword(target.id, { new_password: newPassword });
      this.$updateState({ pwTarget: null, pwSubmitting: false });
      return true;
    } catch (err) {
      this.$updateState({
        pwSubmitting: false,
        pwError: err instanceof ApiError ? err.message : 'Password reset failed',
      });
      return false;
    }
  }

  // --- Permissions drawer --------------------------------------------

  async openPermissions(u: UserView) {
    this.$updateState({
      permsTarget: u,
      permsLoading: true,
      permsError: null,
      perms: [],
      permsAddText: '',
      permsEditingId: null,
      permsEditText: '',
    });
    try {
      const { items } = await permissionService.listForUser(u.id);
      this.$updateState({ perms: items, permsLoading: false });
    } catch (err) {
      this.$updateState({
        permsLoading: false,
        permsError: err instanceof ApiError ? err.message : 'Failed to load permissions',
      });
    }
  }

  closePermissions() {
    this.$updateState({
      permsTarget: null,
      perms: [],
      permsError: null,
      permsAddText: '',
      permsEditingId: null,
      permsEditText: '',
    });
  }

  setPermsAddText(text: string) {
    this.$updateState({ permsAddText: text });
  }

  async submitAddPatterns(): Promise<void> {
    const target = this.state.permsTarget;
    if (!target) return;
    // Split on comma and whitespace (incl. newlines); strip empties.
    const patterns = this.state.permsAddText
      .split(/[,\s]+/)
      .map((p) => p.trim())
      .filter(Boolean);
    if (patterns.length === 0) {
      this.$updateState({ permsError: 'Enter at least one pattern' });
      return;
    }
    this.$updateState({ permsAddSubmitting: true, permsError: null });
    try {
      await permissionService.grantBatch(target.id, patterns);
      // Refetch so the list reflects server-side ordering + any prior dups.
      const { items } = await permissionService.listForUser(target.id);
      this.$updateState({
        perms: items,
        permsAddText: '',
        permsAddSubmitting: false,
      });
    } catch (err) {
      this.$updateState({
        permsAddSubmitting: false,
        permsError: err instanceof ApiError ? err.message : 'Grant failed',
      });
    }
  }

  startEditPattern(p: PermissionView) {
    this.$updateState({ permsEditingId: p.id, permsEditText: p.repo_pattern });
  }

  cancelEditPattern() {
    this.$updateState({ permsEditingId: null, permsEditText: '' });
  }

  setPermsEditText(text: string) {
    this.$updateState({ permsEditText: text });
  }

  async submitEditPattern(): Promise<void> {
    const id = this.state.permsEditingId;
    if (id == null) return;
    const pattern = this.state.permsEditText.trim();
    if (!pattern) {
      this.$updateState({ permsError: 'Pattern cannot be empty' });
      return;
    }
    this.$updateState({ permsRowBusyId: id, permsError: null });
    try {
      await permissionService.update(id, pattern);
      this.$updateState({
        perms: this.state.perms.map((p) => (p.id === id ? { ...p, repo_pattern: pattern } : p)),
        permsEditingId: null,
        permsEditText: '',
        permsRowBusyId: null,
      });
    } catch (err) {
      this.$updateState({
        permsRowBusyId: null,
        permsError: err instanceof ApiError ? err.message : 'Update failed',
      });
    }
  }

  async revokePermission(p: PermissionView): Promise<void> {
    this.$updateState({ permsRowBusyId: p.id, permsError: null });
    try {
      await permissionService.revoke(p.id);
      this.$updateState({
        perms: this.state.perms.filter((x) => x.id !== p.id),
        permsRowBusyId: null,
      });
    } catch (err) {
      this.$updateState({
        permsRowBusyId: null,
        permsError: err instanceof ApiError ? err.message : 'Revoke failed',
      });
    }
  }
}
