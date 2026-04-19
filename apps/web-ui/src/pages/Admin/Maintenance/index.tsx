import { AlertTriangle, Loader2, Trash2 } from 'lucide-react';

import { useViewModel } from '@/lib/viewmodel';
import { MaintenanceViewModel } from './view-model';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
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

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms} ms`;
  const s = ms / 1000;
  if (s < 60) return `${s.toFixed(1)} s`;
  const m = Math.floor(s / 60);
  const rem = Math.round(s - m * 60);
  return `${m}m ${rem}s`;
}

export default function MaintenancePage() {
  const vm = useViewModel(MaintenanceViewModel);
  const s = vm.$useSnapshot();

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold">Maintenance</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Administrative operations on the underlying registry. These actions are audited.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Trash2 className="h-5 w-5" />
            Garbage collection
          </CardTitle>
          <CardDescription>
            Reclaim disk space by removing blobs no longer referenced by any manifest.
            The registry is taken offline for the duration of the run; the UI blocks
            image deletes and docker pushes/pulls will fail. Typically seconds on
            small registries, minutes on large ones.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-start gap-3 rounded-md border border-amber-500/40 bg-amber-500/10 px-4 py-3 text-sm">
            <AlertTriangle className="h-5 w-5 flex-shrink-0 text-amber-600 mt-0.5" />
            <div className="space-y-1">
              <p className="font-medium">Registry will be offline for the duration of the run.</p>
              <p className="text-muted-foreground">
                Runs <code className="font-mono text-xs">registry garbage-collect --delete-untagged</code>;
                manifests without any remaining tag reference are removed alongside
                their orphan blobs.
              </p>
            </div>
          </div>

          {s.error && (
            <div className="rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
              {s.error}
            </div>
          )}

          {s.lastResult && !s.running && (
            <div className="rounded-md border border-emerald-500/40 bg-emerald-500/10 px-4 py-3 text-sm">
              <p className="font-medium text-emerald-700 dark:text-emerald-400">
                Completed in {formatDuration(s.lastResult.duration_ms)}.
              </p>
              {s.lastResult.output_tail && (
                <pre className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-all rounded bg-muted px-3 py-2 text-xs font-mono text-foreground">
                  {s.lastResult.output_tail}
                </pre>
              )}
            </div>
          )}

          <div>
            <Button onClick={() => vm.openConfirm()} disabled={s.running} variant="destructive">
              {s.running ? (
                <>
                  <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                  Running garbage collection…
                </>
              ) : (
                <>
                  <Trash2 className="h-4 w-4 mr-2" />
                  Run garbage collection
                </>
              )}
            </Button>
            {s.running && (
              <p className="mt-2 text-xs text-muted-foreground">
                Don't close this tab. The server is waiting on the registry to stop,
                sweep, and restart. If you navigate away, the run continues but you
                won't see its result.
              </p>
            )}
          </div>
        </CardContent>
      </Card>

      <AlertDialog open={s.confirmOpen} onOpenChange={(open) => !open && vm.closeConfirm()}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Run garbage collection?</AlertDialogTitle>
            <AlertDialogDescription>
              The registry will be stopped, swept, and restarted. Docker pushes and
              pulls will fail until the run completes. This action is irreversible
              for the blobs it removes.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={() => vm.triggerGC()} className="bg-destructive hover:bg-destructive/90">
              Run GC
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
