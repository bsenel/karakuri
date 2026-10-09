import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// vi.mock is hoisted above the imports, so the stub has to be created inside
// the factory and reached through vi.hoisted rather than closed over.
const { get } = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post: vi.fn(), put: vi.fn(), del: vi.fn() } };
});

import { AuditPage } from './AuditPage';

// The page reads its filters from the query string, so it needs a router.
const renderPage = () => render(<MemoryRouter><AuditPage /></MemoryRouter>);

describe('AuditPage', () => {
  // Braces matter: mockReset returns the mock, and vitest runs a function
  // returned from beforeEach as that test's cleanup — it would call get().
  beforeEach(() => {
    get.mockReset();
  });

  it('does not report a failed search as no matching events', async () => {
    get.mockRejectedValue(new Error('API 500: boom'));

    renderPage();
    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent('Could not load audit events: Error: API 500: boom'),
    );
    expect(screen.queryByText('No matching audit events.')).not.toBeInTheDocument();
  });

  it('asks again when Retry is pressed', async () => {
    get.mockRejectedValueOnce(new Error('API 500: boom'));
    // The server answers a nil slice as null.
    get.mockResolvedValue(null);

    renderPage();
    await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));

    await waitFor(() => expect(screen.getByText('No matching audit events.')).toBeInTheDocument());
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(get).toHaveBeenCalledTimes(2);
  });
});
