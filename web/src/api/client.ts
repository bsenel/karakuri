// REST client for the Karakuri API.
//
// The browser never holds a token. Login asks the server for cookie mode, and
// the access and refresh tokens come back as httpOnly cookies this code cannot
// read — which is the point: anything reachable from JavaScript is readable by
// injected script, and a stolen refresh token is a persistent session.
//
// Consequences worth knowing:
//
//   - Every request sets `credentials: 'same-origin'` so the cookies ride along.
//     Without it fetch omits them and every call 401s.
//   - There is no expiry to check, because we cannot see the token. Refresh is
//     reactive: a 401 triggers one refresh and one retry.
//   - Refresh tokens rotate, so concurrent 401s must share a single in-flight
//     refresh. Letting each one refresh independently would spend the token
//     more than once, and the server treats a replayed refresh token as a leak
//     and revokes the whole session family.

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

async function post(path: string, body?: unknown): Promise<Response> {
  return fetch(`/api/v1${path}`, {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? '{}' : JSON.stringify(body),
  });
}

export async function login(id: string, password: string): Promise<void> {
  // `cookie: true` is what makes the server reply with Set-Cookie instead of
  // tokens in the body.
  const res = await post('/auth/token', { id, password, cookie: true });
  if (!res.ok) throw new APIError(res.status, await res.text());
}

export async function logout(): Promise<void> {
  // The server clears the cookies whether or not the revocation succeeds, so a
  // browser that cannot reach it still ends up logged out.
  try {
    await post('/auth/revoke');
  } catch {
    /* offline logout is still a logout */
  }
}

// Shared so parallel callers refresh once rather than racing each other into
// the server's reuse detector.
let inFlightRefresh: Promise<void> | null = null;

function refreshSession(): Promise<void> {
  if (!inFlightRefresh) {
    inFlightRefresh = (async () => {
      try {
        const res = await post('/auth/refresh');
        if (!res.ok) throw new SessionExpiredError();
      } finally {
        inFlightRefresh = null;
      }
    })();
  }
  return inFlightRefresh;
}

/**
 * Ensures the session is currently valid, refreshing if it is not.
 *
 * SSE needs this: EventSource sends the cookies but cannot retry a 401, so the
 * caller has to know the session is good *before* opening the stream.
 */
export async function ensureSession(): Promise<void> {
  const res = await fetch('/api/v1/auth/me', { credentials: 'same-origin' });
  if (res.ok) return;
  if (res.status !== 401) throw new APIError(res.status, await res.text());
  await refreshSession();
}

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const send = () => {
    const headers: Record<string, string> = { Accept: 'application/json' };
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    return fetch(`/api/v1${path}`, {
      method,
      credentials: 'same-origin',
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  };

  let res = await send();
  if (res.status === 401) {
    // The access cookie expired, or the signing key rotated. One refresh, one
    // retry, then give up and let the caller show the login form.
    await refreshSession();
    res = await send();
  }
  if (res.status === 429) {
    // One page load here is several requests at once — the quota page alone
    // asks for its config, its stored tiers, the requests and the overrides —
    // and a burst of them can legitimately reach the per-principal rate limit.
    // The server says exactly how long to wait, so waiting is a better answer
    // than rendering an empty table with no explanation.
    //
    // Once, and briefly. A page that retried repeatedly would turn a limit into
    // a hang, and the limiter's whole purpose is to be felt.
    const wait = retryAfterMS(res.headers.get('Retry-After'));
    if (wait !== null) {
      await new Promise((resolve) => setTimeout(resolve, wait));
      res = await send();
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

/**
 * How long the server asked us to wait, in milliseconds, or null if waiting is
 * not worth it.
 *
 * Capped at two seconds: beyond that the honest thing is to show the refusal
 * rather than leave somebody looking at a spinner. A missing or unparseable
 * header also means null — inventing a delay the server did not ask for is how
 * a client becomes the reason a limit is being hit.
 */
function retryAfterMS(header: string | null): number | null {
  if (!header) return null;
  const seconds = Number(header);
  if (!Number.isFinite(seconds) || seconds < 0) return null;
  const ms = Math.ceil(seconds * 1000);
  return ms <= 2000 ? Math.max(ms, 250) : null;
}

export const api = {
  get: <T>(path: string) => call<T>('GET', path),
  post: <T>(path: string, body?: unknown) => call<T>('POST', path, body),
  put: <T>(path: string, body?: unknown) => call<T>('PUT', path, body),
  del: <T>(path: string) => call<T>('DELETE', path),
};
