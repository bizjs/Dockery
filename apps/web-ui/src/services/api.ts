/**
 * Dockery API client wrapper.
 *
 * All /api/* requests go through here. Two things to remember:
 *
 *   1. `credentials: 'include'` — Dockery relies on the HttpOnly
 *      session cookie set by /api/auth/login; without this flag the
 *      browser would not send it on follow-up requests.
 *
 *   2. Responses are unwrapped from the kratoscarf envelope
 *      ({code, message, data}) into a normal value or thrown error,
 *      so call sites get typed data directly.
 */
export interface ApiEnvelope<T = unknown> {
  code: number;
  message: string;
  data?: T;
}

export class ApiError extends Error {
  readonly status: number;
  readonly code: number;

  constructor(status: number, code: number, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {};
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
  }
  const res = await fetch(path, {
    method,
    headers,
    credentials: 'include',
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  // Some endpoints (e.g. 204 delete) may return empty body.
  const text = await res.text();
  let env: ApiEnvelope<T> | null = null;
  if (text) {
    try {
      env = JSON.parse(text) as ApiEnvelope<T>;
    } catch {
      throw new ApiError(res.status, -1, `non-JSON response: ${text.slice(0, 200)}`);
    }
  }
  if (!res.ok) {
    throw new ApiError(res.status, env?.code ?? -1, env?.message ?? res.statusText);
  }
  if (!env) {
    return undefined as T;
  }
  if (env.code !== 0) {
    throw new ApiError(res.status, env.code, env.message);
  }
  return env.data as T;
}

export const api = {
  get: <T>(p: string) => request<T>('GET', p),
  post: <T>(p: string, body?: unknown) => request<T>('POST', p, body),
  put: <T>(p: string, body?: unknown) => request<T>('PUT', p, body),
  patch: <T>(p: string, body?: unknown) => request<T>('PATCH', p, body),
  delete: <T>(p: string) => request<T>('DELETE', p),
};
