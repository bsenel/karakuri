// EventSource wrapper for the Karakuri SSE endpoints
// (/api/v1/objectives/:id/events and /api/v1/twins/:id/events).
//
// EventSource cannot set headers, so the access token travels as an
// `?access_token=...` query parameter. The server accepts that fallback on SSE
// paths only — query strings end up in access logs, so it is scoped to the one
// case that has no alternative.
//
// The token is fetched (and refreshed if stale) before the stream opens, which
// is why openStream is async: a 15-minute access token has to be valid at
// connect time, and EventSource offers no way to re-authenticate mid-stream.

import { accessToken } from './client';
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
    let url = path;
    try {
      const token = await accessToken();
      url = `${path}?access_token=${encodeURIComponent(token)}`;
    } catch {
      // Unauthenticated: let the server reject the connection so the caller
      // surfaces the same error it would for any other 401.
    }
    if (closed) return;
    es = new EventSource(url);
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
