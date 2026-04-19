import { api } from './api';

export interface PermissionView {
  id: number;
  user_id: number;
  repo_pattern: string;
  created_at: number;
}

export interface PermissionListView {
  items: PermissionView[];
  total: number;
}

export const permissionService = {
  /** List a user's repo patterns. Admin-only. */
  listForUser: (userId: number) =>
    api.get<PermissionListView>(`/api/users/${userId}/permissions`),

  /** Bulk-grant patterns. Duplicates on (user, pattern) are silently skipped;
   *  the response contains only freshly-inserted rows. */
  grantBatch: (userId: number, repoPatterns: string[]) =>
    api.post<PermissionListView>(`/api/users/${userId}/permissions`, {
      repo_patterns: repoPatterns,
    }),

  /** Change a single row's pattern. */
  update: (permissionId: number, repoPattern: string) =>
    api.patch<null>(`/api/permissions/${permissionId}`, {
      repo_pattern: repoPattern,
    }),

  /** Delete a single row by its id. */
  revoke: (permissionId: number) =>
    api.delete<null>(`/api/permissions/${permissionId}`),
};
