import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// vi.mock is hoisted above the imports, so the stub has to be created inside
// the factory and reached through vi.hoisted rather than closed over.
const { get } = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post: vi.fn(), put: vi.fn(), del: vi.fn() } };
});

import { TwinDetailPage } from './TwinDetailPage';

// The page links back to the list, so it needs a router.
const renderPage = () => render(<MemoryRouter><TwinDetailPage /></MemoryRouter>);

const twin = { id: 't1', name: 'Platform team', kind: 'team', domain: 'software', created_at: '', updated_at: '' };
const health = { status: 'ok', adapters: [], providers: {} };
// The page asks for the twin and for /health; answer each by path.
const answer = (path: string) => Promise.resolve(path === '/health' ? health : twin);

describe('TwinDetailPage accessibility', () => {
  // Braces matter: mockReset returns the mock, and vitest runs a function
  // returned from beforeEach as that test's cleanup — it would call get().
  beforeEach(() => {
    get.mockReset();
  });

  it('names the bindings table after its heading', async () => {
    get.mockImplementation(answer);

    renderPage();
    const table = await screen.findByRole('table', { name: 'Adapter Bindings' });
    expect(table).toHaveTextContent('No adapters are configured on this server');
  });
});
