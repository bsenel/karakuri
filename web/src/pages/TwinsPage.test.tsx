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

import { APIError } from '@/api/client';
import { TwinsPage } from './TwinsPage';

const renderPage = () =>
  render(
    <MemoryRouter>
      <TwinsPage />
    </MemoryRouter>,
  );

describe('TwinsPage', () => {
  // Braces matter: mockReset returns the mock, and vitest runs a function
  // returned from beforeEach as that test's cleanup — it would call get().
  beforeEach(() => {
    get.mockReset();
    post.mockReset();
  });

  it('says it is loading rather than that there are no twins', () => {
    // Never resolves: the page is still waiting on its first answer.
    get.mockReturnValue(new Promise(() => {}));

    renderPage();
    expect(screen.getByText('Loading…')).toBeInTheDocument();
    expect(screen.queryByText(/No twins yet/)).not.toBeInTheDocument();
  });

  it('points at the create form once the list is known to be empty', async () => {
    get.mockResolvedValue([]);

    renderPage();
    await waitFor(() =>
      expect(screen.getByText(/No twins yet/)).toHaveTextContent(
        'No twins yet. Create one above, or run krk twin create --name "Platform team".',
      ),
    );
    expect(screen.queryByText('Loading…')).not.toBeInTheDocument();
    expect(screen.queryByText('Retry')).not.toBeInTheDocument();
  });

  it('shows the message of a failed request with a retry, and does not claim the list is empty', async () => {
    get.mockRejectedValue(new APIError(403, '{"message":"twin:read is not granted"}'));

    renderPage();
    await waitFor(() =>
      expect(screen.getByText('twin:read is not granted')).toBeInTheDocument(),
    );
    expect(screen.getByText('Retry')).toBeInTheDocument();
    expect(screen.queryByText(/No twins yet/)).not.toBeInTheDocument();
    expect(screen.queryByText('Loading…')).not.toBeInTheDocument();
  });

  it('tells a screen reader that it is loading, and then what went wrong', async () => {
    // Text that appears on its own is silent unless it sits in a live region.
    let fail: (e: unknown) => void = () => {};
    get.mockReturnValue(new Promise((_, reject) => { fail = reject; }));

    renderPage();
    expect(screen.getByRole('status')).toHaveTextContent('Loading…');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();

    fail(new APIError(403, '{"message":"twin:read is not granted"}'));
    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent('twin:read is not granted'),
    );
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('names the twins table for a screen reader', async () => {
    get.mockResolvedValue([]);

    renderPage();
    await waitFor(() => expect(screen.getByRole('table', { name: 'Twins' })).toBeInTheDocument());
  });

  it('announces the twin it just created', async () => {
    get.mockResolvedValue([]);
    post.mockResolvedValue({ id: 't_1' });

    renderPage();
    await waitFor(() => expect(screen.getByRole('table', { name: 'Twins' })).toBeInTheDocument());

    fireEvent.change(screen.getByRole('textbox', { name: 'Name' }), { target: { value: 'Platform team' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create' }));

    // The reload after the create shows "Loading…" in a status of its own;
    // waitFor retries until that one is gone and only the announcement stays.
    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent('Created twin “Platform team”.'),
    );
    expect(post).toHaveBeenCalledWith('/twins/', expect.objectContaining({ name: 'Platform team' }));
    expect(screen.getByRole('textbox', { name: 'Name' })).toHaveValue('');
  });
});
