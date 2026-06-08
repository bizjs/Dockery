import { ViewModelBase } from 'bizify';
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

export class UsersViewModel extends ViewModelBase<State> {
  protected $data(): State {
    return {
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
    };
  }

  protected onMount() {
    void this.refresh();
  }

  async refresh() {
    Object.assign(this.data, { loading: true, error: null });
    try {
      const { items } = await userService.list();
      Object.assign(this.data, { users: items, loading: false });
    } catch (err) {
      Object.assign(this.data, {
        loading: false,
        error: err instanceof ApiError ? err.message : 'Failed to load users',
      });
    }
  }

  // --- Create dialog --------------------------------------------------

  openCreate() {
    Object.assign(this.data, {
      createOpen: true,
      createForm: blankCreate(),
      createError: null,
    });
  }

  closeCreate() {
    Object.assign(this.data, { createOpen: false, createError: null });
  }

  setCreateField<K extends keyof State['createForm']>(k: K, v: State['createForm'][K]) {
    this.data.createForm = { ...this.data.createForm, [k]: v };
  }

  async submitCreate(): Promise<boolean> {
    const { username, password, role } = this.data.createForm;
    if (!username || !password) {
      this.data.createError = 'Username and password required';
      return false;
    }
    if (password.length < 8) {
      this.data.createError = 'Password must be at least 8 characters';
      return false;
    }
    Object.assign(this.data, { createSubmitting: true, createError: null });
    try {
      const created = await userService.create({ username, password, role });
      Object.assign(this.data, {
        users: [...this.data.users, created],
        createOpen: false,
        createSubmitting: false,
      });
      return true;
    } catch (err) {
      Object.assign(this.data, {
        createSubmitting: false,
        createError: err instanceof ApiError ? err.message : 'Create failed',
      });
      return false;
    }
  }

  // --- Delete dialog --------------------------------------------------

  askDelete(u: UserView) {
    this.data.deleteTarget = u;
  }

  cancelDelete() {
    this.data.deleteTarget = null;
  }

  async confirmDelete(): Promise<void> {
    const target = this.data.deleteTarget;
    if (!target) return;
    this.data.deleteSubmitting = true;
    try {
      await userService.remove(target.id);
      Object.assign(this.data, {
        users: this.data.users.filter((u) => u.id !== target.id),
        deleteTarget: null,
        deleteSubmitting: false,
      });
    } catch (err) {
      Object.assign(this.data, {
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
      this.data.users = this.data.users.map((x) => (x.id === u.id ? updated : x));
    } catch (err) {
      this.data.error = err instanceof ApiError ? err.message : 'Update failed';
    }
  }

  /** Change a user's role inline. Admin-demotion guard is enforced server-side. */
  async setRole(u: UserView, role: UserRole) {
    if (u.role === role) return;
    try {
      const updated = await userService.update(u.id, { role });
      Object.assign(this.data, {
        users: this.data.users.map((x) => (x.id === u.id ? updated : x)),
        error: null,
      });
    } catch (err) {
      this.data.error = err instanceof ApiError ? err.message : 'Role change failed';
    }
  }

  // --- Password reset (admin-driven) ---------------------------------

  askResetPassword(u: UserView) {
    Object.assign(this.data, {
      pwTarget: u,
      pwForm: { newPassword: '' },
      pwError: null,
    });
  }

  cancelResetPassword() {
    Object.assign(this.data, { pwTarget: null, pwError: null });
  }

  setPwField<K extends keyof State['pwForm']>(k: K, v: State['pwForm'][K]) {
    this.data.pwForm = { ...this.data.pwForm, [k]: v };
  }

  async submitResetPassword(): Promise<boolean> {
    const target = this.data.pwTarget;
    if (!target) return false;
    const { newPassword } = this.data.pwForm;
    if (newPassword.length < 8) {
      this.data.pwError = 'Password must be at least 8 characters';
      return false;
    }
    Object.assign(this.data, { pwSubmitting: true, pwError: null });
    try {
      await userService.setPassword(target.id, { new_password: newPassword });
      Object.assign(this.data, { pwTarget: null, pwSubmitting: false });
      return true;
    } catch (err) {
      Object.assign(this.data, {
        pwSubmitting: false,
        pwError: err instanceof ApiError ? err.message : 'Password reset failed',
      });
      return false;
    }
  }

  // --- Permissions drawer --------------------------------------------

  async openPermissions(u: UserView) {
    Object.assign(this.data, {
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
      Object.assign(this.data, { perms: items, permsLoading: false });
    } catch (err) {
      Object.assign(this.data, {
        permsLoading: false,
        permsError: err instanceof ApiError ? err.message : 'Failed to load permissions',
      });
    }
  }

  closePermissions() {
    Object.assign(this.data, {
      permsTarget: null,
      perms: [],
      permsError: null,
      permsAddText: '',
      permsEditingId: null,
      permsEditText: '',
    });
  }

  setPermsAddText(text: string) {
    this.data.permsAddText = text;
  }

  async submitAddPatterns(): Promise<void> {
    const target = this.data.permsTarget;
    if (!target) return;
    // Split on comma and whitespace (incl. newlines); strip empties.
    const patterns = this.data.permsAddText
      .split(/[,\s]+/)
      .map((p) => p.trim())
      .filter(Boolean);
    if (patterns.length === 0) {
      this.data.permsError = 'Enter at least one pattern';
      return;
    }
    Object.assign(this.data, { permsAddSubmitting: true, permsError: null });
    try {
      await permissionService.grantBatch(target.id, patterns);
      // Refetch so the list reflects server-side ordering + any prior dups.
      const { items } = await permissionService.listForUser(target.id);
      Object.assign(this.data, {
        perms: items,
        permsAddText: '',
        permsAddSubmitting: false,
      });
    } catch (err) {
      Object.assign(this.data, {
        permsAddSubmitting: false,
        permsError: err instanceof ApiError ? err.message : 'Grant failed',
      });
    }
  }

  startEditPattern(p: PermissionView) {
    Object.assign(this.data, { permsEditingId: p.id, permsEditText: p.repo_pattern });
  }

  cancelEditPattern() {
    Object.assign(this.data, { permsEditingId: null, permsEditText: '' });
  }

  setPermsEditText(text: string) {
    this.data.permsEditText = text;
  }

  async submitEditPattern(): Promise<void> {
    const id = this.data.permsEditingId;
    if (id == null) return;
    const pattern = this.data.permsEditText.trim();
    if (!pattern) {
      this.data.permsError = 'Pattern cannot be empty';
      return;
    }
    Object.assign(this.data, { permsRowBusyId: id, permsError: null });
    try {
      await permissionService.update(id, pattern);
      Object.assign(this.data, {
        perms: this.data.perms.map((p) => (p.id === id ? { ...p, repo_pattern: pattern } : p)),
        permsEditingId: null,
        permsEditText: '',
        permsRowBusyId: null,
      });
    } catch (err) {
      Object.assign(this.data, {
        permsRowBusyId: null,
        permsError: err instanceof ApiError ? err.message : 'Update failed',
      });
    }
  }

  async revokePermission(p: PermissionView): Promise<void> {
    Object.assign(this.data, { permsRowBusyId: p.id, permsError: null });
    try {
      await permissionService.revoke(p.id);
      Object.assign(this.data, {
        perms: this.data.perms.filter((x) => x.id !== p.id),
        permsRowBusyId: null,
      });
    } catch (err) {
      Object.assign(this.data, {
        permsRowBusyId: null,
        permsError: err instanceof ApiError ? err.message : 'Revoke failed',
      });
    }
  }
}
