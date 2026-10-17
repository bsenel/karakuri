import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

// vi.mock is hoisted above the imports, so the state the stub returns is
// created through vi.hoisted and each test sets it before it renders.
const auth = vi.hoisted(() => ({ error: null as string | null }));
vi.mock('@/auth/AuthProvider', () => ({
  useAuth: () => ({ login: vi.fn(), error: auth.error, loginOptions: null }),
}));

import { LoginModal } from './LoginModal';

describe('LoginModal', () => {
  it('is a named dialog with labelled fields', () => {
    auth.error = null;
    render(<LoginModal />);

    expect(screen.getByRole('dialog', { name: 'Karakuri' })).toBeInTheDocument();
    expect(screen.getByLabelText('User')).toBeInTheDocument();
    expect(screen.getByLabelText('Password')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('announces a failed sign-in', () => {
    auth.error = 'invalid credentials';
    render(<LoginModal />);

    expect(screen.getByRole('alert')).toHaveTextContent('invalid credentials');
  });
});
