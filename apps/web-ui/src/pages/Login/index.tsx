import type React from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { Package } from 'lucide-react';

import { useViewModel } from '@/lib/viewmodel';
import { LoginViewModel } from './view-model';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';

export default function LoginPage() {
  const vm = useViewModel(LoginViewModel);
  const state = vm.$useSnapshot();
  const navigate = useNavigate();
  const location = useLocation();

  const from = (location.state as { from?: string } | null)?.from ?? '/';

  async function onSubmit(e: React.SubmitEvent) {
    e.preventDefault();
    const ok = await vm.submit();
    if (ok) {
      navigate(from, { replace: true });
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-background px-4">
      <div className="w-full max-w-sm space-y-6">
        <div className="flex flex-col items-center gap-3">
          <Package className="h-10 w-10" />
          <div className="text-center">
            <h1 className="text-2xl font-bold">Dockery</h1>
            <p className="text-sm text-muted-foreground">Sign in to manage your registry</p>
          </div>
        </div>

        <form onSubmit={onSubmit} className="space-y-4 rounded-lg border bg-card p-6 shadow-sm">
          <div className="space-y-2">
            <Label htmlFor="username">Username</Label>
            <Input
              id="username"
              autoComplete="username"
              value={state.username}
              onChange={(e) => vm.setUsername(e.target.value)}
              disabled={state.submitting}
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="password">Password</Label>
            <Input
              id="password"
              type="password"
              autoComplete="current-password"
              value={state.password}
              onChange={(e) => vm.setPassword(e.target.value)}
              disabled={state.submitting}
              required
            />
          </div>
          {state.error && (
            <p className="text-sm text-destructive" role="alert">
              {state.error}
            </p>
          )}
          <Button type="submit" className="w-full" disabled={state.submitting}>
            {state.submitting ? 'Signing in…' : 'Sign in'}
          </Button>
        </form>
      </div>
    </div>
  );
}
