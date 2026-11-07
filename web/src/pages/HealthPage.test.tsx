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
import { HealthPage } from './HealthPage';

describe('HealthPage', () => {
  // Braces matter: mockReset returns the mock, and vitest runs a function
  // returned from beforeEach as that test's cleanup — it would call get().
  beforeEach(() => {
    get.mockReset();
  });

  it('names the page while it is loading', () => {
    // Never resolves: the page is still waiting on its first answer.
    get.mockReturnValue(new Promise(() => {}));

    render(<HealthPage />);
    expect(screen.getByText('Health')).toBeInTheDocument();
    expect(screen.getByText('Loading…')).toBeInTheDocument();
  });

  it('shows the message of a failed request and says it keeps trying', async () => {
    get.mockRejectedValue(new APIError(503, '{"message":"database is unreachable"}'));

    render(<HealthPage />);
    await waitFor(() =>
      expect(screen.getByText('database is unreachable')).toBeInTheDocument(),
    );
    expect(screen.getByText('Health')).toBeInTheDocument();
    expect(screen.getByText(/Trying again every 5 seconds/)).toBeInTheDocument();
    expect(screen.queryByText('Loading…')).not.toBeInTheDocument();
  });

  it('tells a screen reader that it is loading, and then what went wrong', async () => {
    let fail: (e: unknown) => void = () => {};
    get.mockReturnValue(new Promise((_, reject) => { fail = reject; }));

    render(<HealthPage />);
    expect(screen.getByRole('status')).toHaveTextContent('Loading…');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();

    fail(new APIError(503, '{"message":"database is unreachable"}'));
    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent('database is unreachable'),
    );
    expect(screen.getByText(/Check that the server is running/)).toBeInTheDocument();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('says so when no providers or adapters are configured', async () => {
    get.mockResolvedValue({ status: 'ok', adapters: [], providers: {} });

    render(<HealthPage />);
    await waitFor(() =>
      expect(screen.getByText('No providers are configured.')).toBeInTheDocument(),
    );
    expect(screen.getByText('No adapters are configured.')).toBeInTheDocument();
  });
});
