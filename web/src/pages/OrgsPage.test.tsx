import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// vi.mock is hoisted above the imports, so the stub has to be created inside
// the factory and reached through vi.hoisted rather than closed over.
const { get } = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post: vi.fn(), put: vi.fn(), del: vi.fn() } };
});
vi.mock('@/auth/AuthProvider', () => ({ useAuth: () => ({ can: () => true }) }));

import { APIError } from '@/api/client';
import { OrgsPage } from './OrgsPage';

describe('OrgsPage', () => {
  // Braces matter: mockReset returns the mock, and vitest runs a function
  // returned from beforeEach as that test's cleanup — it would call get().
  beforeEach(() => {
    get.mockReset();
  });

  it('announces that it is loading rather than that there are no organisations', () => {
    // Never resolves: the page is still waiting on its first answer.
    get.mockReturnValue(new Promise(() => {}));

    render(<OrgsPage />);
    expect(screen.getByRole('status')).toHaveTextContent('Loading…');
    expect(screen.queryByText('No organisations yet.')).not.toBeInTheDocument();
  });

  it('says so once the tree is known to be empty', async () => {
    get.mockResolvedValue([]);

    render(<OrgsPage />);
    await waitFor(() => expect(screen.getByText('No organisations yet.')).toBeInTheDocument());
    expect(screen.getByText('No projects.')).toBeInTheDocument();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('says the tree could not be loaded, with a retry', async () => {
    get.mockRejectedValue(new APIError(500, '{"message":"database is locked"}'));

    render(<OrgsPage />);
    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent('Could not load organisations: database is locked'),
    );
    expect(screen.queryByText('No organisations yet.')).not.toBeInTheDocument();

    get.mockResolvedValue([{ id: 'o1', kind: 'org', name: 'Acme' }]);
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(screen.getByText('Acme')).toBeInTheDocument());
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});
