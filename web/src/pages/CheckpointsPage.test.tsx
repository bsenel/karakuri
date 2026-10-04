import { render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// vi.mock is hoisted above the imports, so the stub has to be created inside
// the factory and reached through vi.hoisted rather than closed over.
const { get } = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post: vi.fn(), put: vi.fn(), del: vi.fn() } };
});

import { APIError } from '@/api/client';
import { CheckpointsPage } from './CheckpointsPage';

describe('CheckpointsPage', () => {
  beforeEach(() => get.mockReset());

  it('says it is loading rather than that nothing is pending', () => {
    // Never resolves: the page is still waiting on its first answer.
    get.mockReturnValue(new Promise(() => {}));

    render(<CheckpointsPage />);
    expect(screen.getByText('Loading…')).toBeInTheDocument();
    expect(screen.queryByText(/No pending checkpoints/)).not.toBeInTheDocument();
  });

  it('says what the queue is once it is known to be empty', async () => {
    get.mockResolvedValue([]);

    render(<CheckpointsPage />);
    await waitFor(() => expect(screen.getByText(/No pending checkpoints/)).toBeInTheDocument());
    expect(screen.getByText(/escalates for approval/)).toBeInTheDocument();
    expect(screen.queryByText('Loading…')).not.toBeInTheDocument();
  });

  it('shows the message of a failed request, and does not claim the queue is empty', async () => {
    get.mockRejectedValue(new APIError(403, '{"message":"checkpoint:read is not granted"}'));

    render(<CheckpointsPage />);
    await waitFor(() =>
      expect(screen.getByText('checkpoint:read is not granted')).toBeInTheDocument(),
    );
    expect(screen.queryByText(/No pending checkpoints/)).not.toBeInTheDocument();
    expect(screen.queryByText('Loading…')).not.toBeInTheDocument();
  });
});
