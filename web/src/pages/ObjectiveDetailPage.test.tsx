import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// vi.mock is hoisted above the imports, so the stub has to be created inside
// the factory and reached through vi.hoisted rather than closed over.
const { get, post } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post, put: vi.fn(), del: vi.fn() } };
});
// jsdom has no EventSource, and the timeline is not what these tests are about.
vi.mock('@/api/sse', () => ({ streamObjective: () => ({ close: () => {} }) }));
// The standing panel makes its own requests; it has its own test.
vi.mock('@/components/StandingPanel', () => ({ StandingPanel: () => null }));

import { ObjectiveDetailPage } from './ObjectiveDetailPage';

// The page links back to the list, so it needs a router.
const renderPage = () => render(<MemoryRouter><ObjectiveDetailPage /></MemoryRouter>);

const objective = { id: 'o1', title: 'Ship the docs', domain: 'software', status: 'active', created_at: '', updated_at: '' };

describe('ObjectiveDetailPage', () => {
  // Braces matter: mockReset returns the mock, and vitest runs a function
  // returned from beforeEach as that test's cleanup — it would call get().
  beforeEach(() => {
    get.mockReset();
    post.mockReset();
  });

  it('announces that it is loading', () => {
    // Never resolves: the page is still waiting on its first answer.
    get.mockReturnValue(new Promise(() => {}));

    renderPage();
    expect(screen.getByRole('status')).toHaveTextContent('Loading…');
  });

  it('says the objective could not be loaded, with a way back and a retry', async () => {
    get.mockRejectedValue(new Error('API 404: objective not found'));

    renderPage();
    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent(
        'Could not load this objective: Error: API 404: objective not found',
      ),
    );
    expect(screen.getByRole('link', { name: '← Objectives' })).toBeInTheDocument();

    get.mockResolvedValue(objective);
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Ship the docs' })).toBeInTheDocument());
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('keeps the objective on screen when the loop fails to start', async () => {
    get.mockResolvedValue(objective);
    post.mockRejectedValue(new Error('API 500: boom'));

    renderPage();
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Ship the docs' })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: 'Start loop' }));

    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('API 500: boom'));
    expect(screen.getByRole('heading', { name: 'Ship the docs' })).toBeInTheDocument();
  });
});
