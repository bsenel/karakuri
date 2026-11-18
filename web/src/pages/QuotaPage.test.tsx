import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';

// vi.mock is hoisted above the imports, so the stub has to be created inside
// the factory and reached through vi.hoisted rather than closed over.
const { get } = vi.hoisted(() => ({ get: vi.fn().mockResolvedValue([]) }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post: vi.fn(), put: vi.fn(), del: vi.fn() } };
});
vi.mock('@/auth/AuthProvider', () => ({
  useAuth: () => ({ can: () => true }),
}));

import { QuotaPage } from './QuotaPage';

describe('QuotaPage', () => {
  it('names its tabs so they are not a second unnamed navigation', () => {
    render(
      <MemoryRouter initialEntries={['/quota']}>
        <Routes>
          <Route path="/quota/*" element={<QuotaPage />} />
        </Routes>
      </MemoryRouter>,
    );

    const tabs = screen.getByRole('navigation', { name: 'Quota' });
    expect(tabs).toHaveTextContent('Usage');
    expect(tabs).toHaveTextContent('Requests');
    expect(tabs).toHaveTextContent('Limits');
  });
});
