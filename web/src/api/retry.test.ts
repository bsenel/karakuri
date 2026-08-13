import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from './client';

// One page load in this application is several requests at once — the quota
// page alone asks for its config, its tiers, the requests and the overrides —
// so reaching the per-principal rate limit is a normal event rather than an
// exceptional one. The server says how long to wait; honouring that beats
// rendering an empty table.
describe('rate-limited requests', () => {
  const fetchMock = vi.fn();

  beforeEach(() => {
    fetchMock.mockReset();
    vi.stubGlobal('fetch', fetchMock);
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  const limited = (retryAfter: string | null) =>
    new Response('{"error":"rate_limited"}', {
      status: 429,
      headers: retryAfter ? { 'Retry-After': retryAfter } : {},
    });

  it('waits the interval the server named, then retries once', async () => {
    fetchMock
      .mockResolvedValueOnce(limited('1'))
      .mockResolvedValueOnce(new Response('[{"id":"t1"}]', { status: 200 }));

    const pending = api.get<{ id: string }[]>('/twins');
    await vi.advanceTimersByTimeAsync(1100);
    await expect(pending).resolves.toEqual([{ id: 't1' }]);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('surfaces the refusal rather than retrying forever', async () => {
    fetchMock.mockResolvedValue(limited('1'));

    const pending = api.get('/twins').catch((e: unknown) => e);
    await vi.advanceTimersByTimeAsync(1100);
    const err = await pending;
    expect(String(err)).toContain('429');
    // Exactly one retry: a page that kept retrying would turn a limit into a
    // hang, and the limiter's whole purpose is to be felt.
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('does not wait when the server named no interval, or an unreasonable one', async () => {
    for (const header of [null, 'soon', '600']) {
      fetchMock.mockReset();
      fetchMock.mockResolvedValue(limited(header));
      const err = await api.get('/twins').catch((e: unknown) => e);
      expect(String(err)).toContain('429');
      // Inventing a delay the server did not ask for is how a client becomes
      // the reason a limit is being hit.
      expect(fetchMock).toHaveBeenCalledTimes(1);
    }
  });
});
