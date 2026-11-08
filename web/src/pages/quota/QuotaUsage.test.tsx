import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const { get } = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post: vi.fn(), put: vi.fn(), del: vi.fn() } };
});

import { APIError } from '@/api/client';
import { QuotaUsage } from './QuotaUsage';

const twins = [{ id: 'twin-1', name: 'Ann' }];

describe('QuotaUsage', () => {
  beforeEach(() => get.mockReset());

  it('asks for a twin before it shows anything', async () => {
    get.mockResolvedValue(twins);

    render(<QuotaUsage />);
    await waitFor(() => expect(screen.getByRole('option', { name: 'Ann' })).toBeInTheDocument());
    expect(screen.getByText(/Pick a twin/)).toBeInTheDocument();
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
  });

  it("labels the table of a twin's tiers", async () => {
    get.mockImplementation((path: string) =>
      Promise.resolve(
        path === '/twins'
          ? twins
          : {
              twin_id: 'twin-1',
              tiers: {
                'llm-tokens': {
                  limit: 1_000_000,
                  remaining: 250_000,
                  used: 0.75,
                  reset_at: '2026-11-09T00:00:00Z',
                  allowed: true,
                },
              },
            },
      ),
    );

    render(<QuotaUsage />);
    await waitFor(() => expect(screen.getByRole('option', { name: 'Ann' })).toBeInTheDocument());
    fireEvent.change(screen.getByRole('combobox', { name: 'Twin' }), {
      target: { value: 'twin-1' },
    });

    await waitFor(() =>
      expect(screen.getByRole('table', { name: 'Usage by tier' })).toBeInTheDocument(),
    );
    expect(screen.getByText('250,000')).toBeInTheDocument();
  });

  it('announces a failure to read usage', async () => {
    get.mockImplementation((path: string) =>
      path === '/twins'
        ? Promise.resolve(twins)
        : Promise.reject(new APIError(500, '{"message":"quota store unavailable"}')),
    );

    render(<QuotaUsage />);
    await waitFor(() => expect(screen.getByRole('option', { name: 'Ann' })).toBeInTheDocument());
    fireEvent.change(screen.getByRole('combobox', { name: 'Twin' }), {
      target: { value: 'twin-1' },
    });

    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent('quota store unavailable'),
    );
  });
});
