import type React from 'react';
import { Trash2, Plus } from 'lucide-react';

import { useViewModel } from '@/lib/viewmodel';
import { UsersViewModel } from './view-model';
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';

function formatDate(ts: number) {
  return new Date(ts * 1000).toLocaleString();
}

export default function UsersPage() {
  const vm = useViewModel(UsersViewModel);
  const s = vm.$useSnapshot();

  async function onCreateSubmit(e: React.SubmitEvent) {
    e.preventDefault();
    await vm.submitCreate();
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
              <TableHead className="w-24">Role</TableHead>
              <TableHead className="w-32">Status</TableHead>
              <TableHead className="w-48">Created</TableHead>
              <TableHead className="w-32 text-right">Actions</TableHead>
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
            {s.users.map((u) => (
              <TableRow key={u.id}>
                <TableCell className="font-mono text-xs">{u.id}</TableCell>
                <TableCell className="font-medium">{u.username}</TableCell>
                <TableCell>
                  <span className="inline-flex items-center rounded-full bg-muted px-2 py-0.5 text-xs font-mono">
                    {u.role}
                  </span>
                </TableCell>
                <TableCell>
                  <label className="inline-flex items-center gap-2 text-xs">
                    <Switch checked={!u.disabled} onCheckedChange={() => vm.toggleDisabled(u)} />
                    <span>{u.disabled ? 'disabled' : 'active'}</span>
                  </label>
                </TableCell>
                <TableCell className="text-xs text-muted-foreground">{formatDate(u.created_at)}</TableCell>
                <TableCell className="text-right">
                  <Button variant="ghost" size="sm" onClick={() => vm.askDelete(u)}>
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
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
