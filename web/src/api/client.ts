// REST client for the Karakuri API.
//
// Access tokens are short-lived (15 minutes by default), so every call goes
// through one wrapper that refreshes them transparently. The refresh token
// rotates on each use, which has two consequences the UI has to respect:
//
//   1. Concurrent 401s must not each trigger their own refresh — the first
//      would spend the token and the rest would look like replays, which the
//      server treats as a leak and answers by revoking the whole family. A
//      single in-flight promise is shared instead.
//   2. Whatever the server returns has to be stored immediately, because the
//      token just used is now dead.

const STORAGE_KEY = 'karakuri_session';

export interface Session {
  access_token: string;
  refresh_token: string;
  /** Epoch milliseconds at which the access token expires. */
  expires_at: number;
}

interface TokenResponse {
  access_token: string;
  refresh_token: string;
  expires_in: number;
}

/** Refresh this many milliseconds before the access token actually expires. */
const REFRESH_SKEW_MS = 60_000;

export function getSession(): Session | null {
  const raw = localStorage.getItem(STORAGE_KEY);
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw) as Session;
    return parsed.access_token ? parsed : null;
  } catch {
    return null;
  }
}

export function setSession(session: Session | null): void {
  if (session) localStorage.setItem(STORAGE_KEY, JSON.stringify(session));
  else localStorage.removeItem(STORAGE_KEY);
}

function toSession(res: TokenResponse): Session {
  return {
    access_token: res.access_token,
    refresh_token: res.refresh_token,
    expires_at: Date.now() + res.expires_in * 1000,
  };
}

export class APIError extends Error {
  constructor(public status: number, public body: string) {
    super(`API ${status}: ${body}`);
  }
}

/** Raised when no valid session remains and the user has to log in again. */
export class SessionExpiredError extends Error {
  constructor(message = 'session expired') {
    super(message);
  }
}

export async function login(id: string, password: string): Promise<Session> {
  const res = await fetch('/api/v1/auth/token', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id, password }),
  });
  const text = await res.text();
  if (!res.ok) throw new APIError(res.status, text);
  const session = toSession(JSON.parse(text) as TokenResponse);
  setSession(session);
  return session;
}

export async function logout(): Promise<void> {
  const session = getSession();
  setSession(null);
  if (!session) return;
  // Best-effort: the local session is already gone either way.
  try {
    await fetch('/api/v1/auth/revoke', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: session.refresh_token }),
    });
  } catch {
    /* offline logout is still a logout */
  }
}

// Shared so parallel callers refresh once rather than racing each other into
// the server's reuse detector.
let inFlightRefresh: Promise<Session> | null = null;

async function refreshSession(session: Session): Promise<Session> {
  if (!inFlightRefresh) {
    inFlightRefresh = (async () => {
      try {
        const res = await fetch('/api/v1/auth/refresh', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ refresh_token: session.refresh_token }),
        });
        if (!res.ok) {
          setSession(null);
          throw new SessionExpiredError();
        }
        const next = toSession((await res.json()) as TokenResponse);
        setSession(next);
        return next;
      } finally {
        inFlightRefresh = null;
      }
    })();
  }
  return inFlightRefresh;
}

/**
 * Returns a usable access token, refreshing first if the current one is within
 * REFRESH_SKEW_MS of expiry. Throws SessionExpiredError when there is no way
 * back to a valid token.
 */
export async function accessToken(): Promise<string> {
  const session = getSession();
  if (!session) throw new SessionExpiredError('not logged in');
  if (Date.now() < session.expires_at - REFRESH_SKEW_MS) return session.access_token;
  const refreshed = await refreshSession(session);
  return refreshed.access_token;
}

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const send = async (token: string | null) => {
    const headers: Record<string, string> = { Accept: 'application/json' };
    if (token) headers.Authorization = `Bearer ${token}`;
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    return fetch(`/api/v1${path}`, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  };

  // /health is public, so an unauthenticated probe is legitimate.
  let token: string | null = null;
  try {
    token = await accessToken();
  } catch (err) {
    if (!(err instanceof SessionExpiredError)) throw err;
  }

  let res = await send(token);
  if (res.status === 401 && token) {
    // The token was rejected despite looking fresh — the signing key rotated,
    // or the principal was disabled. One retry after a refresh, then give up.
    const session = getSession();
    if (session) {
      const refreshed = await refreshSession(session);
      res = await send(refreshed.access_token);
    }
  }

  const text = await res.text();
  if (!res.ok) throw new APIError(res.status, text);
  if (!text) return undefined as T;
  try {
    return JSON.parse(text) as T;
  } catch {
    return text as unknown as T;
  }
}

export const api = {
  get: <T>(path: string) => call<T>('GET', path),
  post: <T>(path: string, body?: unknown) => call<T>('POST', path, body),
  put: <T>(path: string, body?: unknown) => call<T>('PUT', path, body),
  del: <T>(path: string) => call<T>('DELETE', path),
};
