import { api } from './api';

export type UserRole = 'admin' | 'write' | 'view';

export interface CurrentUser {
  id: number;
  username: string;
  role: UserRole;
}

export const authService = {
  /** Exchange credentials for a session cookie. Throws ApiError on failure. */
  login: (username: string, password: string) =>
    api.post<CurrentUser>('/api/auth/login', { username, password }),

  /** Clears the session values on the server; cookie itself expires by TTL. */
  logout: () => api.post<null>('/api/auth/logout'),

  /** Fetches the current session's user. 401 → not logged in. */
  me: () => api.get<CurrentUser>('/api/auth/me'),
};
