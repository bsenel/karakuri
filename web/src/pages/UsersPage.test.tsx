import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// vi.mock is hoisted above the imports, so the stub has to be created inside
// the factory and reached through vi.hoisted rather than closed over.
const { get } = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post: vi.fn(), put: vi.fn(), del: vi.fn() } };
});
vi.mock('@/auth/AuthProvider', () => ({ useAuth: () => ({ can: () => true, identity: null }) }));

import { APIError } from '@/api/client';
import { UsersPage } from './UsersPage';

describe('UsersPage', () => {
  beforeEach(() => get.mockReset());

  it('says the bindings failed rather than that nobody holds a role, and retries', async () => {
    get.mockImplementation((path: string) => {
      if (path === '/auth/users') return Promise.resolve([{ id: 'alice' }]);
      if (path === '/auth/bindings') {
        return Promise.reject(new APIError(500, '{"message":"binding store is unavailable"}'));
      }
      return Promise.resolve([]);
    });

    render(<UsersPage />);
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Could not load role bindings: binding store is unavailable',
    );
    expect(screen.queryByText(/this principal can sign in and see nothing/)).not.toBeInTheDocument();

    get.mockImplementation((path: string) => {
      if (path === '/auth/users') return Promise.resolve([{ id: 'alice' }]);
      return Promise.resolve([]);
    });
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    expect(await screen.findByText('alice')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('shows a loading line until the users arrive', async () => {
    let finish: (v: unknown) => void = () => {};
    get.mockImplementation((path: string) => {
      if (path === '/auth/users') return new Promise((resolve) => { finish = resolve; });
      return Promise.resolve([]);
    });

    render(<UsersPage />);
    expect(screen.getByRole('status')).toHaveTextContent('Loading…');

    finish([{ id: 'alice' }]);
    await waitFor(() => expect(screen.queryByRole('status')).not.toBeInTheDocument());
    expect(screen.getByText('alice')).toBeInTheDocument();
  });

  it('says what to do next when there are no principals', async () => {
    get.mockResolvedValue([]);

    render(<UsersPage />);
    expect(
      await screen.findByText('No principals yet. Add a user above to give someone access.'),
    ).toBeInTheDocument();
  });
});
