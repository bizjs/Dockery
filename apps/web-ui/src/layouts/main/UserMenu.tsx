import { useNavigate, Link } from 'react-router-dom';
import { LogOut, User as UserIcon, Users as UsersIcon } from 'lucide-react';

import { useViewModel } from '@/lib/viewmodel';
import { currentUserViewModel } from '@/hooks/use-current-user';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';

/** Header-embedded user menu: shows current user and admin links. */
export function UserMenu() {
  const vm = useViewModel(currentUserViewModel);
  const s = vm.$useSnapshot();
  const navigate = useNavigate();

  if (!s.user) {
    return null;
  }

  async function doLogout() {
    await vm.logout();
    navigate('/login', { replace: true });
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="sm" className="gap-2">
          <UserIcon className="h-4 w-4" />
          <span>{s.user.username}</span>
          <span className="text-xs text-header-accent-text">({s.user.role})</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-48">
        <DropdownMenuLabel>Signed in as {s.user.username}</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {vm.isAdmin && (
          <DropdownMenuItem asChild>
            <Link to="/admin/users" className="cursor-pointer">
              <UsersIcon className="h-4 w-4 mr-2" />
              Manage users
            </Link>
          </DropdownMenuItem>
        )}
        <DropdownMenuItem onClick={doLogout}>
          <LogOut className="h-4 w-4 mr-2" />
          Sign out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
