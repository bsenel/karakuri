import { render, screen } from '@testing-library/react';
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

describe('RolesPage accessibility', () => {
  // Braces matter: mockReset returns the mock, and vitest runs a function
  // returned from beforeEach as that test's cleanup — it would call get().
  beforeEach(() => {
    get.mockReset();
  });

  it('names the role matrix table', async () => {
    get.mockImplementation((path: string) => {
      if (path === '/auth/roles') {
        return Promise.resolve([{ name: 'viewer', policies: [{ effect: 'allow', actions: ['twin:read'] }] }]);
      }
      return Promise.resolve([{ action: 'quota:approve', description: 'approve a raise' }]);
    });

    render(<RolesPage />);
    const table = await screen.findByRole('table', { name: 'Actions each role grants' });
    expect(table).toHaveTextContent('twin:read');
  });
});
