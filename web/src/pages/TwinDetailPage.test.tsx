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

  it('says the twin could not be loaded, with a way back and a retry', async () => {
    get.mockRejectedValue(new Error('API 404: twin not found'));

    renderPage();
    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent('Could not load this twin: Error: API 404: twin not found'),
    );
    expect(screen.getByRole('link', { name: '← Twins' })).toBeInTheDocument();

    get.mockImplementation(answer);
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Platform team' })).toBeInTheDocument());
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
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
