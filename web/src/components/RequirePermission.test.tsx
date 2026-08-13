import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { RequirePermission } from './RequirePermission';

// The component reads useAuth, and what it does with `can` is the whole
// behaviour — so the hook is the seam to replace.
const can = vi.fn();
vi.mock('@/auth/AuthProvider', () => ({
  useAuth: () => ({ can }),
}));

describe('RequirePermission', () => {
  it('renders the page for somebody holding the action', () => {
    can.mockReturnValue(true);
    render(
      <RequirePermission action="audit:read">
        <p>the audit log</p>
      </RequirePermission>,
    );
    expect(screen.getByText('the audit log')).toBeInTheDocument();
  });

  it('explains the refusal rather than rendering an empty page', () => {
    can.mockReturnValue(false);
    render(
      <RequirePermission action="audit:read" what="the audit log">
        <p>the audit log</p>
      </RequirePermission>,
    );
    expect(screen.queryByText('the audit log')).not.toBeInTheDocument();
    // The permission is named, because "not available" without saying what is
    // missing gives an administrator nothing to act on.
    expect(screen.getByText('audit:read')).toBeInTheDocument();
  });
});
