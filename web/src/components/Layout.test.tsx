import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';

vi.mock('@/auth/AuthProvider', () => ({
  useAuth: () => ({ health: null, identity: null, can: () => true, logout: vi.fn() }),
}));

import { visibleNavigation } from '@/auth/permissions';
import { Layout } from './Layout';

describe('Layout', () => {
  it('names the navigation and announces the current page', () => {
    const current = visibleNavigation(() => true)[0];

    render(<MemoryRouter initialEntries={[current.to]}><Layout /></MemoryRouter>);

    // A named landmark, so a screen reader lists it as "Main navigation"
    // rather than as one unnamed region among several.
    expect(screen.getByRole('navigation', { name: 'Main' })).toBeInTheDocument();
    // The router marks the active link; nothing else tells somebody who cannot
    // see the highlight where they are.
    expect(screen.getByRole('link', { name: current.label })).toHaveAttribute('aria-current', 'page');
    expect(screen.getByRole('button', { name: 'Sign out' })).toBeInTheDocument();
  });
});
