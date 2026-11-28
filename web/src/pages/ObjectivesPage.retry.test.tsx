import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// vi.mock is hoisted above the imports, so the stub has to be created inside
// the factory and reached through vi.hoisted rather than closed over.
const { get } = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post: vi.fn(), put: vi.fn(), del: vi.fn() } };
});

import { APIError } from '@/api/client';
import { ObjectivesPage } from './ObjectivesPage';

describe('ObjectivesPage retry', () => {
  // Braces matter: mockReset returns the mock, and vitest runs a function
  // returned from beforeEach as that test's cleanup.
  beforeEach(() => {
    get.mockReset();
  });

  it('shows that a retry is loading instead of leaving the old error up', async () => {
    get.mockRejectedValue(new APIError(503, '{"message":"database unavailable"}'));

    render(<ObjectivesPage />);
    await waitFor(() =>
      expect(screen.getByText('Could not load objectives: database unavailable')).toBeInTheDocument(),
    );

    // The retry is still in flight: it never resolves.
    get.mockReturnValue(new Promise(() => {}));
    fireEvent.click(screen.getByText('Retry'));

    expect(screen.getByText('Loading…')).toBeInTheDocument();
    expect(screen.queryByText('Could not load objectives: database unavailable')).not.toBeInTheDocument();
    expect(screen.queryByText('Retry')).not.toBeInTheDocument();
  });

  it('shows the empty list once a retry succeeds', async () => {
    get.mockRejectedValue(new APIError(503, '{"message":"database unavailable"}'));

    render(<ObjectivesPage />);
    await waitFor(() => expect(screen.getByText('Retry')).toBeInTheDocument());

    get.mockResolvedValue([]);
    fireEvent.click(screen.getByText('Retry'));

    await waitFor(() => expect(screen.getByText(/No objectives yet/)).toBeInTheDocument());
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});
