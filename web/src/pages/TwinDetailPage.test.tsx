import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// vi.mock is hoisted above the imports, so the stub has to be created inside
// the factory and reached through vi.hoisted rather than closed over.
const { get, put } = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn() }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post: vi.fn(), put, del: vi.fn() } };
});

import { APIError } from '@/api/client';
import { TwinDetailPage } from './TwinDetailPage';

// The page links back to the list, so it needs a router.
const renderPage = () => render(<MemoryRouter><TwinDetailPage /></MemoryRouter>);

const twin = { id: 't1', name: 'Platform team', kind: 'team', domain: 'software', created_at: '', updated_at: '' };
const health = { status: 'ok', adapters: [], providers: {} };
// The page asks for the twin and for /health; answer each by path.
const answer = (path: string) => Promise.resolve(path === '/health' ? health : twin);

describe('TwinDetailPage', () => {
  // Braces matter: mockReset returns the mock, and vitest runs a function
  // returned from beforeEach as that test's cleanup — it would call get().
  beforeEach(() => {
    get.mockReset();
    put.mockReset();
  });

  it('tells a screen reader that it is loading, and then what to do about a failure', async () => {
    let fail: (e: unknown) => void = () => {};
    get.mockReturnValue(new Promise((_, reject) => { fail = reject; }));

    renderPage();
    expect(screen.getByRole('status')).toHaveTextContent('Loading…');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();

    fail(new Error('API 500: boom'));
    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent('Check that the server is running, then retry.'),
    );
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('says the twin could not be loaded, with a way back and a retry', async () => {
    get.mockRejectedValue(new Error('API 404: twin not found'));

    renderPage();
    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent('Could not load this twin: API 404: twin not found'),
    );
    expect(screen.getByRole('link', { name: '← Twins' })).toBeInTheDocument();

    get.mockImplementation(answer);
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Platform team' })).toBeInTheDocument());
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it("shows the server's message rather than the raw response", async () => {
    get.mockRejectedValue(new APIError(404, '{"error":"twin t1 not found"}'));

    renderPage();
    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent('Could not load this twin: twin t1 not found.'),
    );
    expect(screen.getByRole('alert')).not.toHaveTextContent('API 404');
  });

  it('says there is nothing to bind when the server has no adapters', async () => {
    get.mockImplementation(answer);

    renderPage();
    await waitFor(() =>
      expect(screen.getByText(/No adapters are configured on this server/)).toBeInTheDocument(),
    );
  });

  it('names each binding picker by its slot', async () => {
    const withAdapters = {
      ...health,
      adapters: [{ slot: 'git', instance: 'github-main', type: 'github', active: true, is_default: true }],
    };
    get.mockImplementation((path: string) => Promise.resolve(path === '/health' ? withAdapters : twin));

    renderPage();
    await waitFor(() =>
      expect(screen.getByRole('combobox', { name: 'Instance for git' })).toBeInTheDocument(),
    );
  });

  it('keeps the twin on screen when saving bindings fails', async () => {
    get.mockImplementation(answer);
    put.mockRejectedValue(new Error('API 500: boom'));

    renderPage();
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Platform team' })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: 'Save bindings' }));

    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('API 500: boom'));
    expect(screen.getByRole('heading', { name: 'Platform team' })).toBeInTheDocument();
  });
});
