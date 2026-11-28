import { act, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const { get, post, stream } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  stream: { emit: (() => {}) as (e: { type: string; timestamp: string }) => void },
}));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post, put: vi.fn(), del: vi.fn() } };
});
vi.mock('@/auth/AuthProvider', () => ({ useAuth: () => ({ can: () => true }) }));
// Keeps the handler so the test can push events down the stream.
vi.mock('@/api/sse', () => ({
  streamObjective: (_id: string, onEvent: typeof stream.emit) => {
    stream.emit = onEvent;
    return { close: () => {} };
  },
}));
vi.mock('@/components/StandingPanel', () => ({ StandingPanel: () => null }));

import { ObjectiveDetailPage } from './ObjectiveDetailPage';

const objective = { id: 'o1', title: 'Ship the docs', domain: 'software', status: 'active', created_at: '', updated_at: '' };

describe('ObjectiveDetailPage stream', () => {
  beforeEach(() => {
    get.mockReset();
    post.mockReset();
    // jsdom has no scrollTo; the page scrolls the event list after each event.
    if (!Element.prototype.scrollTo) Element.prototype.scrollTo = vi.fn() as never;
  });

  it('says live updates are disconnected, and drops the notice once events resume', async () => {
    get.mockResolvedValue(objective);
    // The page reads its id from the route; without one it never subscribes to the stream.
    render(
      <MemoryRouter initialEntries={['/objectives/o1']}>
        <Routes>
          <Route path="/objectives/:id" element={<ObjectiveDetailPage />} />
        </Routes>
      </MemoryRouter>,
    );
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Ship the docs' })).toBeInTheDocument());

    act(() => stream.emit({ type: 'stream_error', timestamp: '2026-11-22T10:00:00Z' }));
    expect(screen.getByText(/Live updates are disconnected/)).toBeInTheDocument();

    act(() => stream.emit({ type: 'step', timestamp: '2026-11-22T10:00:05Z' }));
    expect(screen.queryByText(/Live updates are disconnected/)).not.toBeInTheDocument();
  });

  it('drops the notice once the stream reconnects, even when no event follows', async () => {
    get.mockResolvedValue(objective);
    render(
      <MemoryRouter initialEntries={['/objectives/o1']}>
        <Routes>
          <Route path="/objectives/:id" element={<ObjectiveDetailPage />} />
        </Routes>
      </MemoryRouter>,
    );
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Ship the docs' })).toBeInTheDocument());

    act(() => stream.emit({ type: 'stream_error', timestamp: '2026-11-22T10:00:00Z' }));
    expect(screen.getByText(/Live updates are disconnected/)).toBeInTheDocument();

    // An idle standing objective sends nothing between passes; the reconnect alone must clear it.
    act(() => stream.emit({ type: 'stream_open', timestamp: '2026-11-22T10:00:03Z' }));
    expect(screen.queryByText(/Live updates are disconnected/)).not.toBeInTheDocument();
  });
});
