import { Loader2, RefreshCw, Settings2, ShieldAlert } from 'lucide-react';
import { useViewModel } from 'bizify';

import { SettingsViewModel } from './view-model';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Switch } from '@/components/ui/switch';
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

function formatUpdatedAt(unixSeconds: number): string {
  return new Date(unixSeconds * 1000).toLocaleString();
}

export default function SettingsPage() {
  const vm = useViewModel(SettingsViewModel);
  const s = vm.useSnapshot();
  const enabled = s.policy?.prevent_tag_overwrite ?? false;
  const unavailable = s.loading || s.saving || !s.policy || s.unknown;
  const enabling = s.pendingValue === true;

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold">Settings</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Persistent system behavior. Changes take effect immediately and are audited.
        </p>
      </div>

      <section className="space-y-3">
        <div>
          <h3 className="text-lg font-semibold">Registry</h3>
          <p className="text-sm text-muted-foreground">Image push and tag behavior.</p>
        </div>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Settings2 className="h-5 w-5" />
              Tag overwrite protection
            </CardTitle>
            <CardDescription>
              Prevent an existing tag from being moved to a different manifest digest.
              Re-pushing the same digest remains allowed.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {s.error && (
              <div className="flex items-start justify-between gap-4 rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
                <span>{s.error}</span>
                <Button size="sm" variant="outline" onClick={() => vm.reload()} disabled={s.loading || s.saving}>
                  <RefreshCw className={`mr-2 h-4 w-4 ${s.loading ? 'animate-spin' : ''}`} />
                  Retry
                </Button>
              </div>
            )}

            <div className="flex items-center justify-between gap-6 rounded-md border px-4 py-4">
              <div className="space-y-1">
                <p className="font-medium">
                  {s.loading ? 'Loading current policy…' : enabled ? 'Enabled' : 'Disabled'}
                </p>
                <p className="text-sm text-muted-foreground">
                  {enabled
                    ? 'Existing tags cannot be moved to another digest.'
                    : 'Existing tags may be overwritten. This is the default behavior.'}
                </p>
                {s.policy && (
                  <p className="text-xs text-muted-foreground">
                    {s.policy.version === 0
                      ? 'Using the default value · not saved yet · version 0'
                      : `Last updated ${formatUpdatedAt(s.policy.updated_at)} by ${s.policy.updated_by} · version ${s.policy.version}`}
                  </p>
                )}
              </div>
              <div className="flex items-center gap-3">
                {(s.loading || s.saving) && <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />}
                <Switch
                  aria-label="Tag overwrite protection"
                  checked={enabled}
                  disabled={unavailable}
                  onCheckedChange={(checked) => vm.requestChange(checked)}
                />
              </div>
            </div>
          </CardContent>
        </Card>
      </section>

      <AlertDialog open={s.pendingValue !== null} onOpenChange={(open) => !open && vm.cancelChange()}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {enabling ? 'Enable tag overwrite protection?' : 'Disable tag overwrite protection?'}
            </AlertDialogTitle>
            <AlertDialogDescription className="space-y-2">
              <span className="flex items-start gap-2">
                <ShieldAlert className="mt-0.5 h-4 w-4 flex-shrink-0" />
                <span>
                  {enabling
                    ? 'CI pipelines that intentionally move rolling tags such as latest will fail when the tag already points to another digest.'
                    : 'Every user with push permission will be able to move existing tags to different image digests again.'}
                </span>
              </span>
              <span className="block">The change applies immediately without restarting Dockery.</span>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={s.saving}>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={() => vm.confirmChange()} disabled={s.saving}>
              {enabling ? 'Enable protection' : 'Disable protection'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
