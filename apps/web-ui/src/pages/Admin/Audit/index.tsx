import type React from 'react';
import { ChevronDown, ChevronLeft, ChevronRight, RefreshCw } from 'lucide-react';

import { useViewModel } from '@/lib/viewmodel';
import { AuditViewModel } from './view-model';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
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
import { AUDIT_ACTIONS, type AuditAction } from '@/services/audit.service';

function formatTs(ts: number): string {
  return new Date(ts * 1000).toLocaleString();
}

const LIMIT_OPTIONS = [25, 50, 100, 200] as const;
const ANY_ACTION = '__any__';

export default function AuditPage() {
  const vm = useViewModel(AuditViewModel);
  const s = vm.$useSnapshot();

  async function onSubmit(e: React.SubmitEvent) {
    e.preventDefault();
    await vm.applyFilters();
  }

  const start = s.items.length === 0 ? 0 : s.offset + 1;
  const end = s.offset + s.items.length;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold">Audit log</h2>
          <p className="text-sm text-muted-foreground mt-1">
            Every authentication and administrative event. Most recent first.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => vm.reload()} disabled={s.loading}>
          <RefreshCw className={`h-4 w-4 mr-2 ${s.loading ? 'animate-spin' : ''}`} />
          Refresh
        </Button>
      </div>

      {/* Filters */}
      <form
        onSubmit={onSubmit}
        className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-3 rounded-md border p-4"
      >
        <div className="space-y-1">
          <Label htmlFor="audit-actor" className="text-xs">Actor</Label>
          <Input
            id="audit-actor"
            placeholder="username (substring)"
            value={s.form.actor}
            onChange={(e) => vm.setField('actor', e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <Label className="text-xs">Action</Label>
          <Select
            value={s.form.action || ANY_ACTION}
            onValueChange={(v) =>
              vm.setField('action', v === ANY_ACTION ? '' : (v as AuditAction))
            }
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ANY_ACTION}>(any)</SelectItem>
              {AUDIT_ACTIONS.map((a) => (
                <SelectItem key={a} value={a}>
                  {a}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1">
          <Label htmlFor="audit-since" className="text-xs">Since</Label>
          <Input
            id="audit-since"
            type="datetime-local"
            value={s.form.since}
            onChange={(e) => vm.setField('since', e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="audit-until" className="text-xs">Until</Label>
          <Input
            id="audit-until"
            type="datetime-local"
            value={s.form.until}
            onChange={(e) => vm.setField('until', e.target.value)}
          />
        </div>
        <div className="flex items-end gap-2">
          <Button type="submit" disabled={s.loading} className="flex-1">
            Apply
          </Button>
          <Button
            type="button"
            variant="ghost"
            onClick={() => vm.clearFilters()}
            disabled={s.loading}
          >
            Clear
          </Button>
        </div>
      </form>

      {s.error && (
        <div className="rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {s.error}
        </div>
      )}

      {/* Table */}
      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-44">Timestamp</TableHead>
              <TableHead className="w-32">Actor</TableHead>
              <TableHead className="w-48">Action</TableHead>
              <TableHead>Target</TableHead>
              <TableHead className="w-32">Client IP</TableHead>
              <TableHead className="w-10"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {s.loading && s.items.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} className="text-center py-8 text-muted-foreground">
                  Loading…
                </TableCell>
              </TableRow>
            )}
            {!s.loading && s.items.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} className="text-center py-8 text-muted-foreground">
                  No entries match.
                </TableCell>
              </TableRow>
            )}
            {s.items.map((e) => {
              const isExpanded = s.expanded.has(e.id);
              const hasDetail = (e.detail && Object.keys(e.detail).length > 0) || e.scope;
              return (
                <Row
                  key={e.id}
                  entry={e}
                  isExpanded={isExpanded}
                  hasDetail={!!hasDetail}
                  onToggle={() => vm.toggleExpand(e.id)}
                />
              );
            })}
          </TableBody>
        </Table>
      </div>

      {/* Pagination */}
      <div className="flex items-center justify-between gap-4 flex-wrap">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <span>
            {s.total > 0 ? `${start}–${end} of ${s.total}` : '0 entries'}
          </span>
          <span className="text-muted-foreground">•</span>
          <Label htmlFor="audit-limit" className="text-xs">Page size</Label>
          <Select value={String(s.limit)} onValueChange={(v) => vm.setLimit(Number(v))}>
            <SelectTrigger id="audit-limit" className="h-8 w-20">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {LIMIT_OPTIONS.map((n) => (
                <SelectItem key={n} value={String(n)}>
                  {n}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => vm.prevPage()}
            disabled={s.loading || s.offset === 0}
          >
            <ChevronLeft className="h-4 w-4 mr-1" />
            Prev
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => vm.nextPage()}
            disabled={s.loading || s.offset + s.limit >= s.total}
          >
            Next
            <ChevronRight className="h-4 w-4 ml-1" />
          </Button>
        </div>
      </div>
    </div>
  );
}

interface RowProps {
  entry: import('@/services/audit.service').AuditEntry;
  isExpanded: boolean;
  hasDetail: boolean;
  onToggle: () => void;
}

function Row({ entry, isExpanded, hasDetail, onToggle }: RowProps) {
  const badgeClass = entry.success
    ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border-emerald-500/30'
    : 'bg-destructive/10 text-destructive border-destructive/30';
  return (
    <>
      <TableRow className={hasDetail ? 'cursor-pointer' : undefined} onClick={hasDetail ? onToggle : undefined}>
        <TableCell className="text-xs text-muted-foreground">{formatTs(entry.ts)}</TableCell>
        <TableCell className="font-medium">{entry.actor}</TableCell>
        <TableCell>
          <span
            className={`inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-mono ${badgeClass}`}
          >
            {entry.action}
          </span>
        </TableCell>
        <TableCell className="text-xs font-mono text-muted-foreground">
          {entry.target || '—'}
        </TableCell>
        <TableCell className="text-xs text-muted-foreground">{entry.client_ip || '—'}</TableCell>
        <TableCell className="text-right">
          {hasDetail && (
            <ChevronDown
              className={`h-4 w-4 text-muted-foreground transition-transform ${
                isExpanded ? 'rotate-180' : ''
              }`}
            />
          )}
        </TableCell>
      </TableRow>
      {isExpanded && hasDetail && (
        <TableRow>
          <TableCell colSpan={6} className="bg-muted/30">
            <div className="space-y-2 py-2">
              {entry.scope && (
                <div className="text-xs">
                  <span className="font-semibold">Scope: </span>
                  <code className="font-mono">{entry.scope}</code>
                </div>
              )}
              {entry.detail && Object.keys(entry.detail).length > 0 && (
                <pre className="whitespace-pre-wrap break-all rounded bg-background px-3 py-2 text-xs font-mono border">
                  {JSON.stringify(entry.detail, null, 2)}
                </pre>
              )}
            </div>
          </TableCell>
        </TableRow>
      )}
    </>
  );
}
