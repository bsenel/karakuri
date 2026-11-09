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
    expect(screen.queryByText(/No organisations yet/)).not.toBeInTheDocument();
  });

  it('says so once the tree is known to be empty', async () => {
    get.mockResolvedValue([]);

    render(<OrgsPage />);
    // The empty state names both ways to make the first one: the add button
    // above it and the krk command that does the same thing.
    await waitFor(() =>
      expect(screen.getByText(/No organisations yet/)).toHaveTextContent(
        'No organisations yet. Add one above, or run krk org create --name acme.',
      ),
    );
    expect(screen.getByText('No projects.')).toBeInTheDocument();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('names what each add button adds', async () => {
    get.mockResolvedValue([{ id: 'o1', kind: 'org', name: 'Acme' }]);

    render(<OrgsPage />);
    await waitFor(() => expect(screen.getByText('Acme')).toBeInTheDocument());
    expect(screen.getByRole('button', { name: 'Add organisation' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add team' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add project' })).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'New team' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Add' })).not.toBeInTheDocument();
  });

  it('says the tree could not be loaded, with a retry', async () => {
    get.mockRejectedValue(new APIError(500, '{"message":"database is locked"}'));

    render(<OrgsPage />);
    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent('Could not load organisations: database is locked'),
    );
    expect(screen.queryByText(/No organisations yet/)).not.toBeInTheDocument();

    get.mockResolvedValue([{ id: 'o1', kind: 'org', name: 'Acme' }]);
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(screen.getByText('Acme')).toBeInTheDocument());
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('names what each delete button deletes', async () => {
    get.mockResolvedValue([
      { id: 'o1', kind: 'org', name: 'Acme' },
      { id: 't1', kind: 'team', name: 'Core', parent_id: 'o1' },
      { id: 'p1', kind: 'project', name: 'Apollo' },
    ]);

    render(<OrgsPage />);
    await waitFor(() => expect(screen.getByText('Acme')).toBeInTheDocument());
    // Every one of these buttons reads "delete"; a screen reader lists them by
    // name, so each has to say which row it removes.
    expect(screen.getByRole('button', { name: 'Delete organisation Acme' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Delete team Core' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Delete project Apollo' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'delete' })).not.toBeInTheDocument();
  });
});
