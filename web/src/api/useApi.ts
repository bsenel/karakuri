import { useCallback, useEffect, useState } from 'react';
import { api, APIError } from './client';

export interface Fetched<T> {
  data: T | null;
  loading: boolean;
  error: string | null;
  /** Re-runs the request, for after a write. */
  reload: () => void;
}

/**
 * useFetch reads a path and tracks the three states a caller has to render.
 *
 * The error is a string rather than an Error because every consumer displays
 * it, and a 403 in particular has to read as an explanation rather than a
 * stack trace — a scoped principal meeting a page they cannot open is a normal
 * event on this system, not a fault.
 *
 * `deps` is the caller's dependency list. It is passed rather than derived from
 * `path` so a page can refetch on a filter change without rebuilding the URL
 * twice.
 */
export function useFetch<T>(path: string | null, deps: unknown[] = []): Fetched<T> {
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(path !== null);
  const [error, setError] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);

  const reload = useCallback(() => setNonce((n) => n + 1), []);

  useEffect(() => {
    if (path === null) {
      setData(null);
      setLoading(false);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError(null);
    api
      .get<T>(path)
      .then((result) => {
        if (!cancelled) setData(result);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(describe(err));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      // A filter changed while this was in flight. Without this the slower of
      // two requests wins and the table shows the previous filter's rows.
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [path, nonce, ...deps]);

  return { data, loading, error, reload };
}

/**
 * describe turns whatever was thrown into something worth showing a person.
 *
 * The API answers errors as JSON with a message, so the useful text is inside
 * the body rather than in the status — "you can only approve a raise for a
 * subject you already hold" is the whole point of that response, and
 * "API 403: {...}" throws it away.
 */
export function describe(err: unknown): string {
  if (err instanceof APIError) {
    try {
      const parsed = JSON.parse(err.body) as { message?: string; error?: string };
      if (parsed.message) return parsed.message;
      if (parsed.error) return parsed.error;
    } catch {
      /* not JSON — fall through to the raw body */
    }
    if (err.body.trim()) return err.body.trim();
    return `Request failed (${err.status})`;
  }
  if (err instanceof Error) return err.message;
  return String(err);
}
