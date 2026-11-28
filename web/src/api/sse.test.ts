import { describe, expect, it, vi } from 'vitest';
import { attach } from './sse';

// EventSource reconnects on its own. A reconnect that brings no new event must
// still end the "disconnected" state, so an open after an error says so; the
// first open, with nothing to recover from, says nothing.
describe('attach', () => {
  it('emits stream_open only on an open that follows an error', () => {
    const es = {} as EventSource;
    const onEvent = vi.fn();
    attach(es, onEvent);

    es.onopen?.call(es, new Event('open'));
    expect(onEvent).not.toHaveBeenCalled();

    es.onerror?.call(es, new Event('error'));
    es.onopen?.call(es, new Event('open'));
    expect(onEvent.mock.calls.map(([e]) => e.type)).toEqual(['stream_error', 'stream_open']);

    // A second open without a new error adds nothing.
    es.onopen?.call(es, new Event('open'));
    expect(onEvent).toHaveBeenCalledTimes(2);
  });
});
