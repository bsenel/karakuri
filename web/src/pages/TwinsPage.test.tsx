import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// vi.mock is hoisted above the imports, so the stub has to be created inside
// the factory and reached through vi.hoisted rather than closed over.
const { get } = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post: vi.fn(), put: vi.fn(), del: vi.fn() } };
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
    await waitFor(() => expect(screen.getByText(/No twins yet/)).toBeInTheDocument());
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
});
