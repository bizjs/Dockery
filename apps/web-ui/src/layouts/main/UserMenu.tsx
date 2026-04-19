import { useState } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { LogOut, User as UserIcon, Users as UsersIcon, Key, Wrench, ScrollText } from 'lucide-react';

import { useViewModel } from '@/lib/viewmodel';
import { currentUserViewModel } from '@/hooks/use-current-user';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { userService } from '@/services/user.service';
import { ApiError } from '@/services/api';

/** Header-embedded user menu: shows current user, admin links, self-service
 *  password change, and logout. */
export function UserMenu() {
  const vm = useViewModel(currentUserViewModel);
  const s = vm.$useSnapshot();
  const navigate = useNavigate();

  const [pwOpen, setPwOpen] = useState(false);
  const [oldPw, setOldPw] = useState('');
  const [newPw, setNewPw] = useState('');
  const [pwError, setPwError] = useState<string | null>(null);
  const [pwSubmitting, setPwSubmitting] = useState(false);
  const [pwSuccess, setPwSuccess] = useState(false);

  if (!s.user) {
    return null;
  }
  // Capture locally so TypeScript narrows inside async handlers below.
  const user = s.user;

  async function doLogout() {
    await vm.logout();
    navigate('/login', { replace: true });
  }

  function openPwDialog() {
    setOldPw('');
    setNewPw('');
    setPwError(null);
    setPwSuccess(false);
    setPwOpen(true);
  }

  async function submitPwChange(e: React.FormEvent) {
    e.preventDefault();
    if (newPw.length < 8) {
      setPwError('New password must be at least 8 characters');
      return;
    }
    if (!oldPw) {
      setPwError('Current password required');
      return;
    }
    setPwSubmitting(true);
    setPwError(null);
    try {
      await userService.setPassword(user.id, { old_password: oldPw, new_password: newPw });
      setPwSuccess(true);
      // Clear form but keep the dialog open briefly so the user sees the success state.
      setOldPw('');
      setNewPw('');
      setTimeout(() => setPwOpen(false), 800);
    } catch (err) {
      setPwError(err instanceof ApiError ? err.message : 'Password change failed');
    } finally {
      setPwSubmitting(false);
    }
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="sm" className="gap-2">
            <UserIcon className="h-4 w-4" />
            <span>{user.username}</span>
            <span className="text-xs text-header-accent-text">({user.role})</span>
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-52">
          <DropdownMenuLabel>Signed in as {user.username}</DropdownMenuLabel>
          <DropdownMenuSeparator />
          {vm.isAdmin && (
            <>
              <DropdownMenuItem asChild>
                <Link to="/admin/users" className="cursor-pointer">
                  <UsersIcon className="h-4 w-4 mr-2" />
                  Manage users
                </Link>
              </DropdownMenuItem>
              <DropdownMenuItem asChild>
                <Link to="/admin/maintenance" className="cursor-pointer">
                  <Wrench className="h-4 w-4 mr-2" />
                  Maintenance
                </Link>
              </DropdownMenuItem>
              <DropdownMenuItem asChild>
                <Link to="/admin/audit" className="cursor-pointer">
                  <ScrollText className="h-4 w-4 mr-2" />
                  Audit log
                </Link>
              </DropdownMenuItem>
            </>
          )}
          <DropdownMenuItem onSelect={openPwDialog}>
            <Key className="h-4 w-4 mr-2" />
            Change password
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={doLogout}>
            <LogOut className="h-4 w-4 mr-2" />
            Sign out
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <Dialog open={pwOpen} onOpenChange={setPwOpen}>
        <DialogContent>
          <form onSubmit={submitPwChange}>
            <DialogHeader>
              <DialogTitle>Change password</DialogTitle>
              <DialogDescription>
                Enter your current password and choose a new one (≥ 8 characters).
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="self-old-password">Current password</Label>
                <Input
                  id="self-old-password"
                  type="password"
                  autoComplete="current-password"
                  value={oldPw}
                  onChange={(e) => setOldPw(e.target.value)}
                  disabled={pwSubmitting}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="self-new-password">New password</Label>
                <Input
                  id="self-new-password"
                  type="password"
                  autoComplete="new-password"
                  value={newPw}
                  onChange={(e) => setNewPw(e.target.value)}
                  disabled={pwSubmitting}
                  minLength={8}
                  required
                />
              </div>
              {pwError && <p className="text-sm text-destructive">{pwError}</p>}
              {pwSuccess && <p className="text-sm text-emerald-600">Password updated.</p>}
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="ghost"
                onClick={() => setPwOpen(false)}
                disabled={pwSubmitting}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={pwSubmitting}>
                {pwSubmitting ? 'Saving…' : 'Change password'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  );
}
