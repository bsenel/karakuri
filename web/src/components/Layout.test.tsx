import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// What the principal holds is per test; most tests hold everything.
const { can } = vi.hoisted(() => ({ can: vi.fn((_action: string) => true) }));
vi.mock('@/auth/AuthProvider', () => ({
  useAuth: () => ({ health: null, identity: null, can, logout: vi.fn() }),
}));

import { visibleNavigation } from '@/auth/permissions';
import { Layout } from './Layout';

describe('Layout', () => {
  // A block body: an arrow that returns the mock would hand vitest a function,
  // which it runs as teardown.
  beforeEach(() => {
    can.mockReset();
    can.mockImplementation(() => true);
  });

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

  // Phase 37 step 6: one stop-all control, on every page, for the moment
  // something is going wrong everywhere. Its behaviour is specified in
  // StopAllControl.test.tsx.
  it('offers the stop-all control from every page', () => {
    render(<MemoryRouter><Layout /></MemoryRouter>);

    expect(screen.getByRole('button', { name: 'Stop all' })).toBeInTheDocument();
  });

  it('hides the decorative brand glyph from screen readers', () => {
    render(<MemoryRouter><Layout /></MemoryRouter>);

    expect(screen.getByText('⌬')).toHaveAttribute('aria-hidden', 'true');
  });

  // Phase 40 step 14: the oversight page reads GET /reports/preview, which
  // needs report:read, so the entry is offered on that permission alone.
  it('offers Oversight to a principal who holds report:read', () => {
    can.mockImplementation((action: string) => action === 'report:read');

    render(<MemoryRouter><Layout /></MemoryRouter>);

    expect(screen.getByRole('link', { name: 'Oversight' })).toHaveAttribute('href', '/oversight');
  });

  it('does not offer Oversight without report:read', () => {
    can.mockImplementation((action: string) => action !== 'report:read');

    render(<MemoryRouter><Layout /></MemoryRouter>);

    expect(screen.queryByRole('link', { name: 'Oversight' })).not.toBeInTheDocument();
    // Gating one entry must not hide the rest.
    expect(screen.getByRole('link', { name: 'Objectives' })).toBeInTheDocument();
  });
});
