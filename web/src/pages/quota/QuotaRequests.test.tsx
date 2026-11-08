import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const { get, post } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post, put: vi.fn(), del: vi.fn() } };
});
vi.mock('@/auth/AuthProvider', () => ({ useAuth: () => ({ can: () => true }) }));

import { APIError } from '@/api/client';
import { QuotaRequests } from './QuotaRequests';

const pending = {
  id: 'r1',
  subject: 'twin-1',
  name: 'llm-tokens',
  cap: 2_000_000,
  reason: 'a long migration',
  status: 'pending',
  requested_by: 'ann',
  created_at: '2026-11-08T09:00:00Z',
};

describe('QuotaRequests', () => {
  beforeEach(() => {
    get.mockReset();
    post.mockReset();
  });

  // The refusal is the tenancy rule explaining itself, so it is announced
  // rather than left for somebody to notice above the table.
  it('announces a refused decision', async () => {
    get.mockImplementation((path: string) =>
      Promise.resolve(path === '/quota/requests' ? [pending] : []),
    );
    post.mockRejectedValue(
      new APIError(403, '{"message":"you can only approve a raise for a subject you already hold"}'),
    );

    render(<QuotaRequests />);
    await waitFor(() => expect(screen.getByText('a long migration')).toBeInTheDocument());

    fireEvent.click(screen.getByRole('button', { name: 'Approve' }));
    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent(
        'you can only approve a raise for a subject you already hold',
      ),
    );
  });

  it('labels the requests table', async () => {
    get.mockImplementation((path: string) =>
      Promise.resolve(path === '/quota/requests' ? [pending] : []),
    );

    render(<QuotaRequests />);
    await waitFor(() =>
      expect(screen.getByRole('table', { name: 'Requests' })).toBeInTheDocument(),
    );
    expect(
      screen.getByText('No raises in force — every subject is on the configured tier.'),
    ).toBeInTheDocument();
  });
});
