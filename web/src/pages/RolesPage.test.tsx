import { render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// vi.mock is hoisted above the imports, so the stub has to be created inside
// the factory and reached through vi.hoisted rather than closed over.
const { get } = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post: vi.fn(), put: vi.fn(), del: vi.fn() } };
});
vi.mock('@/auth/AuthProvider', () => ({ useAuth: () => ({ can: () => true }) }));

import { RolesPage } from './RolesPage';

describe('RolesPage', () => {
  beforeEach(() => get.mockReset());

  it('turns nested policies into a grid, and reads a wildcard as everything', async () => {
    get.mockImplementation((path: string) => {
      if (path === '/auth/roles') {
        return Promise.resolve([
          { name: 'viewer', policies: [{ effect: 'allow', actions: ['twin:read'] }] },
          { name: 'admin', policies: [{ effect: 'allow', actions: ['*'] }] },
        ]);
      }
      return Promise.resolve([{ action: 'quota:approve', description: 'approve a raise' }]);
    });

    render(<RolesPage />);
    await waitFor(() => expect(screen.getByText('twin:read')).toBeInTheDocument());

    // The catalogue widens the rows beyond what any role happens to grant: an
    // action nobody holds is worth seeing as an empty row rather than not at all.
    expect(screen.getByText('quota:approve')).toBeInTheDocument();

    // Two roles, and admin's wildcard covers both rows while viewer's covers one.
    expect(screen.getAllByText('✓')).toHaveLength(3);
  });

  it('does not render a deny as a grant', async () => {
    get.mockImplementation((path: string) => {
      if (path === '/auth/roles') {
        return Promise.resolve([
          {
            name: 'restricted',
            policies: [
              { effect: 'allow', actions: ['twin:read'] },
              { effect: 'deny', actions: ['twin:delete'] },
            ],
          },
        ]);
      }
      return Promise.resolve([]);
    });

    render(<RolesPage />);
    await waitFor(() => expect(screen.getByText('twin:read')).toBeInTheDocument());
    // The denied action is not a row at all — it was never granted, and a tick
    // beside it would be worse than its absence.
    expect(screen.queryByText('twin:delete')).not.toBeInTheDocument();
    expect(screen.getAllByText('✓')).toHaveLength(1);
  });
});
