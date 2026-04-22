import type React from 'react';
import { Trash2, Plus, Key, Shield, X } from 'lucide-react';

import { useViewModel } from '@/lib/viewmodel';
import { UsersViewModel } from './view-model';
import { currentUserViewModel } from '@/hooks/use-current-user';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import type { UserRole } from '@/services/auth.service';

function formatDate(ts: number) {
  return new Date(ts * 1000).toLocaleString();
}

export default function UsersPage() {
  const vm = useViewModel(UsersViewModel);
  const s = vm.$useSnapshot();
  const meVm = useViewModel(currentUserViewModel);
  const me = meVm.$useSnapshot().user;

  async function onCreateSubmit(e: React.SubmitEvent) {
    e.preventDefault();
    await vm.submitCreate();
  }

  async function onResetPwSubmit(e: React.SubmitEvent) {
    e.preventDefault();
    await vm.submitResetPassword();
  }

  async function onAddPatternsSubmit(e: React.SubmitEvent) {
    e.preventDefault();
    await vm.submitAddPatterns();
  }

  async function onEditPatternSubmit(e: React.SubmitEvent) {
    e.preventDefault();
    await vm.submitEditPattern();
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold">Users</h2>
          <p className="text-sm text-muted-foreground mt-1">
            {s.users.length} total. Role drives what actions a user may perform.
          </p>
        </div>
        <Button onClick={() => vm.openCreate()}>
          <Plus className="h-4 w-4 mr-2" />
          New user
        </Button>
      </div>

      {s.error && (
        <div className="rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {s.error}
        </div>
      )}

      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-16">ID</TableHead>
              <TableHead>Username</TableHead>
              <TableHead className="w-32">Role</TableHead>
              <TableHead className="w-32">Status</TableHead>
              <TableHead className="w-48">Created</TableHead>
              <TableHead className="w-48 text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {s.loading && (
              <TableRow>
                <TableCell colSpan={6} className="text-center py-8 text-muted-foreground">
                  Loading…
                </TableCell>
              </TableRow>
            )}
            {!s.loading && s.users.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} className="text-center py-8 text-muted-foreground">
                  No users.
                </TableCell>
              </TableRow>
            )}
            {s.users.map((u) => {
              const isSelf = me?.id === u.id;
              return (
                <TableRow key={u.id}>
                  <TableCell className="font-mono text-xs">{u.id}</TableCell>
                  <TableCell className="font-medium">
                    {u.username}
                    {isSelf && (
                      <span className="ml-2 text-xs text-muted-foreground">(you)</span>
                    )}
                  </TableCell>
                  <TableCell>
                    <Select
                      value={u.role}
                      onValueChange={(v) => vm.setRole(u, v as UserRole)}
                      disabled={isSelf}
                    >
                      <SelectTrigger className="h-8 w-28 text-xs font-mono">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="admin">admin</SelectItem>
                        <SelectItem value="write">write</SelectItem>
                        <SelectItem value="view">view</SelectItem>
                      </SelectContent>
                    </Select>
                  </TableCell>
                  <TableCell>
                    <label className="inline-flex items-center gap-2 text-xs">
                      <Switch
                        checked={!u.disabled}
                        onCheckedChange={() => vm.toggleDisabled(u)}
                        disabled={isSelf}
                      />
                      <span>{u.disabled ? 'disabled' : 'active'}</span>
                    </label>
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">{formatDate(u.created_at)}</TableCell>
                  <TableCell className="text-right">
                    <div className="inline-flex items-center gap-1">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => vm.openPermissions(u)}
                        disabled={u.role === 'admin'}
                        title={u.role === 'admin' ? 'admin has full access' : 'Manage repo permissions'}
                      >
                        <Shield className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => vm.askResetPassword(u)}
                        title="Reset password"
                      >
                        <Key className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => vm.askDelete(u)}
                        disabled={isSelf}
                        title={isSelf ? 'Cannot delete yourself' : 'Delete user'}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </div>

      {/* Create dialog */}
      <Dialog open={s.createOpen} onOpenChange={(open) => (open ? vm.openCreate() : vm.closeCreate())}>
        <DialogContent>
          <form onSubmit={onCreateSubmit}>
            <DialogHeader>
              <DialogTitle>Create user</DialogTitle>
              <DialogDescription>
                Role determines capabilities. <strong>admin</strong> has full access; <strong>write</strong> can
                pull/push/delete on granted repos; <strong>view</strong> can only pull.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="new-username">Username</Label>
                <Input
                  id="new-username"
                  value={s.createForm.username}
                  onChange={(e) => vm.setCreateField('username', e.target.value)}
                  disabled={s.createSubmitting}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="new-password">Password</Label>
                <Input
                  id="new-password"
                  type="password"
                  value={s.createForm.password}
                  onChange={(e) => vm.setCreateField('password', e.target.value)}
                  disabled={s.createSubmitting}
                  minLength={8}
                  required
                />
                <p className="text-xs text-muted-foreground">At least 8 characters.</p>
              </div>
              <div className="space-y-2">
                <Label>Role</Label>
                <Select
                  value={s.createForm.role}
                  onValueChange={(v) => vm.setCreateField('role', v as typeof s.createForm.role)}
                  disabled={s.createSubmitting}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="admin">admin</SelectItem>
                    <SelectItem value="write">write</SelectItem>
                    <SelectItem value="view">view</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              {s.createError && <p className="text-sm text-destructive">{s.createError}</p>}
            </div>
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={() => vm.closeCreate()} disabled={s.createSubmitting}>
                Cancel
              </Button>
              <Button type="submit" disabled={s.createSubmitting}>
                {s.createSubmitting ? 'Creating…' : 'Create'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Reset password dialog */}
      <Dialog
        open={!!s.pwTarget}
        onOpenChange={(open) => !open && vm.cancelResetPassword()}
      >
        <DialogContent>
          <form onSubmit={onResetPwSubmit}>
            <DialogHeader>
              <DialogTitle>Reset password</DialogTitle>
              <DialogDescription>
                Set a new password for <strong>{s.pwTarget?.username}</strong>. The user will need
                to sign in again on next request.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="reset-new-password">New password</Label>
                <Input
                  id="reset-new-password"
                  type="password"
                  value={s.pwForm.newPassword}
                  onChange={(e) => vm.setPwField('newPassword', e.target.value)}
                  disabled={s.pwSubmitting}
                  minLength={8}
                  required
                />
                <p className="text-xs text-muted-foreground">At least 8 characters.</p>
              </div>
              {s.pwError && <p className="text-sm text-destructive">{s.pwError}</p>}
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="ghost"
                onClick={() => vm.cancelResetPassword()}
                disabled={s.pwSubmitting}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={s.pwSubmitting}>
                {s.pwSubmitting ? 'Saving…' : 'Reset password'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Permissions drawer (sheet) */}
      <Sheet open={!!s.permsTarget} onOpenChange={(open) => !open && vm.closePermissions()}>
        <SheetContent side="right" className="w-full sm:max-w-lg overflow-y-auto">
          <SheetHeader>
            <SheetTitle>
              Permissions — <span className="font-mono">{s.permsTarget?.username}</span>
            </SheetTitle>
            <SheetDescription>
              Repo patterns matched by this user. Role <strong>{s.permsTarget?.role}</strong>{' '}
              determines allowed actions on matched repos. With no patterns,
              the user is unrestricted and the role applies to every repo —
              add patterns to limit access.
            </SheetDescription>
          </SheetHeader>

          <div className="space-y-4 py-4">
            {s.permsError && (
              <div className="rounded-md border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                {s.permsError}
              </div>
            )}

            {/* Add patterns */}
            <form onSubmit={onAddPatternsSubmit} className="space-y-2">
              <Label htmlFor="perms-add">Add pattern(s)</Label>
              <Input
                id="perms-add"
                placeholder="alice/*  shared/app  team/api/*"
                value={s.permsAddText}
                onChange={(e) => vm.setPermsAddText(e.target.value)}
                disabled={s.permsAddSubmitting}
              />
              <div className="flex items-center justify-between">
                <p className="text-xs text-muted-foreground">
                  Separate with spaces or commas. Supports <code>*</code>, <code>alice/*</code>,{' '}
                  <code>alice/app</code>.
                </p>
                <Button type="submit" size="sm" disabled={s.permsAddSubmitting}>
                  {s.permsAddSubmitting ? 'Adding…' : 'Add'}
                </Button>
              </div>
            </form>

            {/* List */}
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Pattern</TableHead>
                    <TableHead className="w-40">Added</TableHead>
                    <TableHead className="w-20 text-right"></TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {s.permsLoading && (
                    <TableRow>
                      <TableCell colSpan={3} className="text-center py-6 text-muted-foreground">
                        Loading…
                      </TableCell>
                    </TableRow>
                  )}
                  {!s.permsLoading && s.perms.length === 0 && (
                    <TableRow>
                      <TableCell colSpan={3} className="text-center py-6 text-muted-foreground">
                        No patterns. User is unrestricted — role applies to every repo.
                      </TableCell>
                    </TableRow>
                  )}
                  {s.perms.map((p) => {
                    const isEditing = s.permsEditingId === p.id;
                    const busy = s.permsRowBusyId === p.id;
                    return (
                      <TableRow key={p.id}>
                        <TableCell>
                          {isEditing ? (
                            <form onSubmit={onEditPatternSubmit} className="flex items-center gap-1">
                              <Input
                                value={s.permsEditText}
                                onChange={(e) => vm.setPermsEditText(e.target.value)}
                                className="h-8 font-mono text-xs"
                                autoFocus
                                disabled={busy}
                              />
                              <Button type="submit" size="sm" variant="outline" disabled={busy}>
                                Save
                              </Button>
                              <Button
                                type="button"
                                size="sm"
                                variant="ghost"
                                onClick={() => vm.cancelEditPattern()}
                                disabled={busy}
                              >
                                <X className="h-4 w-4" />
                              </Button>
                            </form>
                          ) : (
                            <button
                              type="button"
                              onClick={() => vm.startEditPattern(p)}
                              className="font-mono text-xs text-left hover:underline"
                            >
                              {p.repo_pattern}
                            </button>
                          )}
                        </TableCell>
                        <TableCell className="text-xs text-muted-foreground">
                          {formatDate(p.created_at)}
                        </TableCell>
                        <TableCell className="text-right">
                          {!isEditing && (
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => vm.revokePermission(p)}
                              disabled={busy}
                            >
                              <Trash2 className="h-4 w-4" />
                            </Button>
                          )}
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </div>
          </div>
        </SheetContent>
      </Sheet>

      {/* Delete confirmation */}
      <AlertDialog open={!!s.deleteTarget} onOpenChange={(open) => !open && vm.cancelDelete()}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete user?</AlertDialogTitle>
            <AlertDialogDescription>
              This permanently removes <strong>{s.deleteTarget?.username}</strong> and all their repo
              permissions. The user will be logged out on their next request.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={s.deleteSubmitting}>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={() => vm.confirmDelete()} disabled={s.deleteSubmitting}>
              {s.deleteSubmitting ? 'Deleting…' : 'Delete'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
