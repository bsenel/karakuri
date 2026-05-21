// EventSource wrapper for the Karakuri SSE endpoints
// (/api/v1/objectives/:id/events and /api/v1/twins/:id/events).
//
// EventSource does not natively support custom headers, so when auth is on we
// pass the bearer token as a `?token=...` query string and the server reads
// it from the URL fallback path. (The middleware already handles this for
// `/events` endpoints.)

import { getToken } from './client';
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
  const token = getToken();
  const url = token ? `${path}?token=${encodeURIComponent(token)}` : path;
  const es = new EventSource(url);

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

  return { close: () => es.close() };
}
