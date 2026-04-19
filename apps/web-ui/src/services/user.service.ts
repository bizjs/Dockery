import { api } from './api';
import type { UserRole } from './auth.service';

export interface UserView {
  id: number;
  username: string;
  role: UserRole;
  disabled: boolean;
  created_at: number;
  updated_at: number;
}

export interface UserListView {
  items: UserView[];
  total: number;
}

export const userService = {
  list: () => api.get<UserListView>('/api/users'),

  create: (req: { username: string; password: string; role: UserRole }) =>
    api.post<UserView>('/api/users', req),

  update: (id: number, req: { role?: UserRole; disabled?: boolean }) =>
    api.patch<UserView>(`/api/users/${id}`, req),

  remove: (id: number) => api.delete<null>(`/api/users/${id}`),

  setPassword: (id: number, req: { old_password?: string; new_password: string }) =>
    api.put<null>(`/api/users/${id}/password`, req),
};
