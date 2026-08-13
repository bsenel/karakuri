import { render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const { get } = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post: vi.fn(), put: vi.fn(), del: vi.fn() } };
});
vi.mock('@/auth/AuthProvider', () => ({ useAuth: () => ({ can: () => true }) }));

import { QuotaSettings } from './QuotaSettings';

const config = (overrides: Record<string, unknown> = {}) => ({
  request: { algorithm: 'token_bucket', limit: 20, window: '1m0s', per_second: 1 },
  capability: { cap: 1000, period: 'daily' },
  llm_tokens: { cap: 5_000_000, period: 'daily' },
  adapter: { cap: 5000, period: 'daily' },
  pressure_threshold: 0.8,
  editable: true,
  configured: {
    request: { algorithm: 'token_bucket', limit: 20, window: '1m0s', per_second: 1 },
    capability: { cap: 1000, period: 'daily' },
    llm_tokens: { cap: 1_000_000, period: 'daily' },
    adapter: { cap: 5000, period: 'daily' },
  },
  ...overrides,
});

describe('QuotaSettings', () => {
  beforeEach(() => get.mockReset());

  // The pairing is the whole reason the database-backed tiers needed care: an
  // operator reading the YAML file is reading the seed, not the limit.
  it('shows what is in force beside what was configured, when they differ', async () => {
    get.mockImplementation((path: string) =>
      Promise.resolve(
        path === '/quota'
          ? config()
          : {
              stored: [
                {
                  name: 'llm-tokens',
                  cap: 5_000_000,
                  reason: 'the team grew',
                  updated_by: 'ann',
                  updated_at: '2026-08-13T09:00:00Z',
                },
              ],
              editable: true,
            },
      ),
    );

    render(<QuotaSettings />);
    await waitFor(() => expect(screen.getByText('5,000,000')).toBeInTheDocument());

    // Both numbers, and who moved it and why — the audit trail is the point of
    // requiring a reason at write time.
    expect(screen.getByText('1,000,000')).toBeInTheDocument();
    expect(screen.getByText(/Set by ann — the team grew/)).toBeInTheDocument();
  });

  it('shows one number when nothing has been changed', async () => {
    get.mockImplementation((path: string) =>
      Promise.resolve(
        path === '/quota'
          ? config({ llm_tokens: { cap: 1_000_000, period: 'daily' } })
          : { stored: [], editable: true },
      ),
    );

    render(<QuotaSettings />);
    await waitFor(() => expect(screen.getAllByText('1,000,000')).toHaveLength(1));
    expect(screen.queryByText(/Set by/)).not.toBeInTheDocument();
  });

  // A deployment with no database is told why rather than offered a form whose
  // result would vanish on the next restart.
  it('explains itself when limits cannot be edited', async () => {
    get.mockImplementation((path: string) =>
      Promise.resolve(
        path === '/quota' ? config({ editable: false }) : { stored: [], editable: false },
      ),
    );

    render(<QuotaSettings />);
    await waitFor(() =>
      expect(screen.getByText(/keeps no database/)).toBeInTheDocument(),
    );
    expect(screen.queryByText('change')).not.toBeInTheDocument();
  });
});
