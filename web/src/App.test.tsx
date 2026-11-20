import type { ReactNode } from 'react';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';

vi.mock('@/auth/AuthProvider', () => ({
  AuthProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
  useAuth: () => ({ ready: false, identity: null, health: null, can: () => true, logout: vi.fn() }),
}));

import App from './App';

describe('App', () => {
  it('announces that it is loading while the session is checked', () => {
    render(<MemoryRouter><App /></MemoryRouter>);

    expect(screen.getByRole('status')).toHaveTextContent('Loading…');
  });
});
