import type { ReactNode } from 'react';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// Whether the session is known is per test; the routed test is signed in.
const { auth } = vi.hoisted(() => ({
  auth: {
    ready: false,
    identity: null as null | { principal: { id: string; name: string }; roles: string[] },
  },
}));
vi.mock('@/auth/AuthProvider', () => ({
  AuthProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
  useAuth: () => ({
    ready: auth.ready,
    identity: auth.identity,
    health: null,
    can: () => true,
    logout: vi.fn(),
  }),
}));
// The page is tested on its own; here only that the route reaches it.
vi.mock('@/pages/OversightPage', () => ({
  OversightPage: () => <h1>Oversight page</h1>,
}));

import App from './App';

describe('App', () => {
  beforeEach(() => {
    auth.ready = false;
    auth.identity = null;
  });

  it('announces that it is loading while the session is checked', () => {
    render(<MemoryRouter><App /></MemoryRouter>);

    expect(screen.getByRole('status')).toHaveTextContent('Loading…');
  });

  // Phase 40 step 14: one web page for the oversight digest.
  it('renders the oversight page at /oversight', () => {
    auth.ready = true;
    auth.identity = { principal: { id: 'u1', name: 'Ann' }, roles: [] };

    render(<MemoryRouter initialEntries={['/oversight']}><App /></MemoryRouter>);

    expect(screen.getByRole('heading', { name: 'Oversight page' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Oversight' })).toHaveAttribute('aria-current', 'page');
  });
});
