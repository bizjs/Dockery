import { type ReactNode, useEffect } from 'react';
import { Navigate, useLocation } from 'react-router-dom';

import { currentUserViewModel } from '@/hooks/use-current-user';

interface Props {
  children: ReactNode;
  /** Require role === "admin" in addition to being logged in. */
  adminOnly?: boolean;
}

/**
 * AuthGuard blocks route rendering until the current-user status is
 * known (avoids a login-screen flash on refresh), then either renders
 * the children or redirects.
 */
export function AuthGuard({ children, adminOnly }: Props) {
  // The singleton isn't created via useViewModel, so bizify's onMount
  // never fires for it — kick off the idempotent /me bootstrap here.
  useEffect(() => {
    currentUserViewModel.bootstrap();
  }, []);
  const state = currentUserViewModel.useSnapshot();
  const location = useLocation();

  if (!state.initialized) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <p className="text-sm text-muted-foreground">Loading…</p>
      </div>
    );
  }

  if (!state.user) {
    return <Navigate to="/login" state={{ from: location.pathname }} replace />;
  }

  if (adminOnly && state.user.role !== 'admin') {
    return (
      <div className="flex min-h-screen items-center justify-center px-4">
        <div className="text-center space-y-2">
          <p className="text-lg font-semibold">Access denied</p>
          <p className="text-sm text-muted-foreground">
            Admin role required to view this page. Your current role is <code>{state.user.role}</code>.
          </p>
        </div>
      </div>
    );
  }

  return <>{children}</>;
}
