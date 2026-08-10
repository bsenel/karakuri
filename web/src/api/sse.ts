// EventSource wrapper for the Karakuri SSE endpoints
// (/api/v1/objectives/:id/events and /api/v1/twins/:id/events).
//
// EventSource cannot set headers, which is the usual reason SSE ends up with a
// token in the query string. It does send cookies, though, and the session is
// held in httpOnly cookies — so the stream authenticates with no credential in
// the URL at all. URLs leak into access logs, proxy logs and Referer headers;
// cookies do not.
//
// The session is refreshed if needed before the stream opens, which is why this
// is async: EventSource offers no way to re-authenticate mid-stream, so the
// cookies have to be valid at connect time.

import { ensureSession } from './client';
import type { SSEEvent } from './types';

export type SSEHandler = (event: SSEEvent) => void;

export interface SSEStream {
  close(): void;
}

export function streamObjective(objectiveID: string, onEvent: SSEHandler): SSEStream {
  return openStream(`/api/v1/objectives/${encodeURIComponent(objectiveID)}/events`, onEvent);
}

export function streamTwin(twinID: string, onEvent: SSEHandler): SSEStream {
  return openStream(`/api/v1/twins/${encodeURIComponent(twinID)}/events`, onEvent);
}

function openStream(path: string, onEvent: SSEHandler): SSEStream {
  let es: EventSource | null = null;
  let closed = false;

  void (async () => {
    try {
      await ensureSession();
    } catch {
      // Unauthenticated: open anyway and let the server reject the connection,
      // so the caller surfaces the same error it would for any other 401.
    }
    if (closed) return;
    es = new EventSource(path, { withCredentials: true });
    attach(es, onEvent);
  })();

  return {
    close() {
      closed = true;
      es?.close();
    },
  };
}

function attach(es: EventSource, onEvent: SSEHandler): void {
  es.onmessage = (msg) => {
    if (!msg.data) return;
    try {
      onEvent(JSON.parse(msg.data) as SSEEvent);
    } catch (err) {
      // Surface unparseable lines as synthetic error events so the UI can
      // show them rather than silently drop them.
      onEvent({ type: 'parse_error', timestamp: new Date().toISOString(), payload: { raw: msg.data, error: String(err) } });
    }
  };
  es.onerror = () => {
    // EventSource auto-reconnects; surface a one-shot "disconnected" event for UI.
    onEvent({ type: 'stream_error', timestamp: new Date().toISOString() });
  };
}
