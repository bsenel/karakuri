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
vi.mock('@/auth/AuthProvider', () => ({ useAuth: () => ({ can: () => true }) }));

import { APIError } from '@/api/client';
import { AuditEventPage } from './AuditEventPage';

// The page links back to the list, so it needs a router.
const renderPage = () => render(<MemoryRouter><AuditEventPage /></MemoryRouter>);

const auditEvent = {
  id: 'a1',
  objective_id: 'obj-1',
  success: true,
  kind: 'tool_call',
  created_at: '2026-10-10T08:00:00Z',
};

describe('AuditEventPage', () => {
  // Braces matter: mockReset returns the mock, and vitest runs a function
  // returned from beforeEach as that test's cleanup — it would call get().
  beforeEach(() => {
    get.mockReset();
  });

  it('announces that it is loading', () => {
    // Never resolves: the page is still waiting on its first answer.
    get.mockReturnValue(new Promise(() => {}));

    renderPage();
    expect(screen.getByRole('status')).toHaveTextContent('Loading…');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('shows the event once it has loaded', async () => {
    get.mockResolvedValue(auditEvent);

    renderPage();
    await waitFor(() => expect(screen.getByRole('heading', { name: 'tool_call' })).toBeInTheDocument());
    expect(screen.getByText('succeeded')).toBeInTheDocument();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('says the event was not found, with a way back and a retry', async () => {
    get.mockRejectedValue(new APIError(500, '{"message":"database is locked"}'));

    renderPage();
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('Not found'));
    expect(screen.getByRole('link', { name: 'Back to the audit log' })).toBeInTheDocument();

    get.mockResolvedValue(auditEvent);
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(screen.getByRole('heading', { name: 'tool_call' })).toBeInTheDocument());
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});
