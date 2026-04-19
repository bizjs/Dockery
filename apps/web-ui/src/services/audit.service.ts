import { api } from './api';

/** One row of the audit log, as returned by GET /api/audit. */
export interface AuditEntry {
  id: number;
  /** Unix seconds. */
  ts: number;
  actor: string;
  action: string;
  target?: string;
  scope?: string;
  client_ip?: string;
  success: boolean;
  detail?: Record<string, unknown>;
}

export interface AuditList {
  items: AuditEntry[];
  total: number;
}

/** Query filters. Empty strings and zeros are omitted from the request. */
export interface AuditFilter {
  actor?: string;
  action?: string;
  /** Unix seconds (inclusive). */
  since?: number;
  until?: number;
  limit?: number;
  offset?: number;
}

/** Canonical action values the backend writes (keep in sync with
 *  internal/biz/audit.go). Used to populate the action dropdown. */
export const AUDIT_ACTIONS = [
  'token.issued',
  'token.denied',
  'auth.login.success',
  'auth.login.failure',
  'user.created',
  'user.role_changed',
  'user.disabled',
  'user.enabled',
  'user.deleted',
  'user.password_changed',
  'permission.granted',
  'permission.updated',
  'permission.revoked',
  'image.deleted',
  'gc.started',
  'gc.completed',
  'key.rotated',
] as const;

export type AuditAction = (typeof AUDIT_ACTIONS)[number];

export const auditService = {
  list(filter: AuditFilter = {}): Promise<AuditList> {
    const params = new URLSearchParams();
    if (filter.actor) params.set('actor', filter.actor);
    if (filter.action) params.set('action', filter.action);
    if (filter.since) params.set('since', String(filter.since));
    if (filter.until) params.set('until', String(filter.until));
    if (filter.limit) params.set('limit', String(filter.limit));
    if (filter.offset) params.set('offset', String(filter.offset));
    const qs = params.toString();
    return api.get<AuditList>(qs ? `/api/audit?${qs}` : '/api/audit');
  },
};
