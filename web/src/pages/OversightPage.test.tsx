import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const { get } = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post: vi.fn(), put: vi.fn(), del: vi.fn() } };
});

import { APIError } from '@/api/client';
import type { Digest, DigestObjective } from '@/api/types';
import { OversightPage } from './OversightPage';

const twins = [
  { id: 'twin-1', name: 'Ann' },
  { id: 'twin-2', name: 'Team' },
];

const stream = (over: Partial<DigestObjective>): DigestObjective => ({
  id: 'obj-x',
  title: 'untitled',
  status: 'converged',
  senses: 0,
  reconciles: 0,
  drift_detected: 0,
  converged: 0,
  escalations: 0,
  failures: 0,
  actions: 0,
  criteria_met: 1,
  ...over,
});

// Two standing objectives: one with refusals and blocks (the refusals are
// given newest first on purpose, so the page has to order them), and one that
// did nothing worth a line.
const digest: Digest = {
  twin_id: 'twin-1',
  twin_name: 'Ann',
  since: '2026-11-21T00:00:00Z',
  until: '2026-11-22T00:00:00Z',
  objectives: [
    stream({
      id: 'obj-1',
      title: 'Deliver the roadmap',
      status: 'blocked',
      senses: 12,
      reconciles: 5,
      converged: 3,
      escalations: 2,
      failures: 1,
      actions: 4,
      refusals: [
        {
          at: '2026-11-21T18:00:00Z',
          capability: 'git.push',
          reason: 'pushed ref refs/heads/main was never named',
        },
        {
          at: '2026-11-21T09:00:00Z',
          capability: 'shell.exec',
          reason: 'outside the plan bounds',
        },
      ],
      blocks: [
        {
          kind: 'waiting_for_quota',
          reason: 'coding agent is out of quota',
          at: '2026-11-21T20:00:00Z',
          until: '2026-11-23T05:00:00Z',
        },
        { kind: 'bounds_violation', reason: 'pushed to an unnamed ref', at: '2026-11-21T18:00:00Z' },
        { kind: 'remote_unreadable', reason: 'forge returned 500', at: '2026-11-21T21:00:00Z' },
      ],
    }),
    stream({ id: 'obj-2', title: 'Keep the docs fresh', senses: 7, reconciles: 0 }),
    stream({
      id: 'obj-3',
      title: 'Triage inbound issues',
      status: 'blocked',
      paused: true,
      paused_why: 'the circuit breaker tripped after three failures',
    }),
  ],
};

const renderPage = () => render(<MemoryRouter><OversightPage /></MemoryRouter>);

// The report URLs the page asked for, in order.
const previewCalls = () =>
  get.mock.calls.map((c) => String(c[0])).filter((p) => p.startsWith('/reports/preview'));

async function pickTwin(id: string) {
  await waitFor(() => expect(screen.getByRole('option', { name: 'Ann' })).toBeInTheDocument());
  fireEvent.change(screen.getByRole('combobox', { name: 'Twin' }), { target: { value: id } });
}

const streamRegion = (title: string) => screen.findByRole('region', { name: title });

describe('OversightPage', () => {
  // A block body: an arrow that returns the mock would hand vitest a function,
  // which it runs as teardown, calling get() after each test.
  beforeEach(() => {
    get.mockReset();
    get.mockImplementation((path: string) =>
      Promise.resolve(path === '/twins' ? twins : digest),
    );
  });

  it('asks the digest of the twin the user picked', async () => {
    renderPage();
    expect(previewCalls()).toEqual([]);

    await pickTwin('twin-1');
    await streamRegion('Deliver the roadmap');

    const calls = previewCalls();
    expect(calls).toHaveLength(1);
    const url = new URL(calls[0], 'http://localhost');
    expect(url.pathname).toBe('/reports/preview');
    expect(url.searchParams.get('twin_id')).toBe('twin-1');
  });

  it('passes the window the user chose, and asks again when it changes', async () => {
    renderPage();
    await pickTwin('twin-1');
    await streamRegion('Deliver the roadmap');

    fireEvent.change(screen.getByRole('combobox', { name: 'Window' }), { target: { value: '168h' } });

    await waitFor(() => expect(previewCalls()).toHaveLength(2));
    const url = new URL(previewCalls()[1], 'http://localhost');
    expect(url.searchParams.get('twin_id')).toBe('twin-1');
    expect(url.searchParams.get('window')).toBe('168h');
  });

  it('shows each stream by its title with its pass counts and their names', async () => {
    renderPage();
    await pickTwin('twin-1');

    const first = await streamRegion('Deliver the roadmap');
    expect(first).toHaveTextContent(/senses\W*12/i);
    expect(first).toHaveTextContent(/reconciles\W*5/i);
    expect(first).toHaveTextContent(/converged\W*3/i);
    expect(first).toHaveTextContent(/escalations\W*2/i);
    expect(first).toHaveTextContent(/failures\W*1/i);

    const second = await streamRegion('Keep the docs fresh');
    expect(second).toHaveTextContent(/senses\W*7/i);
    // One stream's numbers are not the other's.
    expect(second).not.toHaveTextContent(/senses\W*12/i);
  });

  it('lists every refusal with its capability and reason, oldest first', async () => {
    renderPage();
    await pickTwin('twin-1');

    const region = await streamRegion('Deliver the roadmap');
    const refusals = within(region).getByRole('list', { name: /refusals/i });
    const items = within(refusals).getAllByRole('listitem');

    expect(items).toHaveLength(2);
    expect(items[0]).toHaveTextContent('shell.exec');
    expect(items[0]).toHaveTextContent('outside the plan bounds');
    expect(items[1]).toHaveTextContent('git.push');
    expect(items[1]).toHaveTextContent('pushed ref refs/heads/main was never named');
  });

  it('names every kind of block in words', async () => {
    renderPage();
    await pickTwin('twin-1');

    const region = await streamRegion('Deliver the roadmap');
    const blocks = within(region).getByRole('list', { name: /blocks/i });

    expect(within(blocks).getAllByRole('listitem')).toHaveLength(3);
    expect(blocks).toHaveTextContent(/waiting for quota/i);
    expect(blocks).toHaveTextContent(/bounds violation/i);
    expect(blocks).toHaveTextContent(/remote unreadable/i);
    expect(blocks).toHaveTextContent('coding agent is out of quota');
    // The raw enum is for the API, not for a person.
    expect(blocks).not.toHaveTextContent('waiting_for_quota');
    expect(blocks).not.toHaveTextContent('bounds_violation');
    expect(blocks).not.toHaveTextContent('remote_unreadable');
  });

  it('shows when a quota block clears, and only for a block that says so', async () => {
    renderPage();
    await pickTwin('twin-1');

    const region = await streamRegion('Deliver the roadmap');
    const blocks = within(region).getByRole('list', { name: /blocks/i });
    const items = within(blocks).getAllByRole('listitem');
    const quota = items.find((i) => /waiting for quota/i.test(i.textContent ?? ''));
    const violation = items.find((i) => /bounds violation/i.test(i.textContent ?? ''));

    expect(quota).toHaveTextContent(/until[\s\S]*2026/i);
    expect(violation).not.toHaveTextContent(/until/i);
  });

  it('says in words that a quiet stream has no refusals and no blocks', async () => {
    renderPage();
    await pickTwin('twin-1');

    const region = await streamRegion('Keep the docs fresh');
    expect(region).toHaveTextContent(/no refusals and no blocks/i);
    expect(within(region).queryByRole('table')).not.toBeInTheDocument();
    expect(within(region).queryByRole('list', { name: /refusals/i })).not.toBeInTheDocument();
    expect(within(region).queryByRole('list', { name: /blocks/i })).not.toBeInTheDocument();
  });

  it('shows that a paused stream is paused and why', async () => {
    renderPage();
    await pickTwin('twin-1');

    const paused = await streamRegion('Triage inbound issues');
    expect(paused).toHaveTextContent(/paused/i);
    expect(paused).toHaveTextContent('the circuit breaker tripped after three failures');

    const running = await streamRegion('Keep the docs fresh');
    expect(running).not.toHaveTextContent(/paused/i);
  });

  it('shows a failed request as an error, never as nothing to report', async () => {
    get.mockImplementation((path: string) =>
      path === '/twins'
        ? Promise.resolve(twins)
        : Promise.reject(new APIError(403, '{"message":"report:read is not granted"}')),
    );

    renderPage();
    await pickTwin('twin-1');

    expect(await screen.findByRole('alert')).toHaveTextContent('report:read is not granted');
    expect(screen.queryByText(/no refusals and no blocks/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/nothing to report/i)).not.toBeInTheDocument();
    expect(screen.queryByRole('region')).not.toBeInTheDocument();
  });

  it('asks again when Retry is pressed after a failure', async () => {
    let fail = true;
    get.mockImplementation((path: string) => {
      if (path === '/twins') return Promise.resolve(twins);
      return fail ? Promise.reject(new Error('API 500: boom')) : Promise.resolve(digest);
    });

    renderPage();
    await pickTwin('twin-1');
    await screen.findByRole('alert');

    fail = false;
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));

    await streamRegion('Deliver the roadmap');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('says there are no standing objectives when the twin has none', async () => {
    get.mockImplementation((path: string) =>
      Promise.resolve(path === '/twins' ? twins : { ...digest, objectives: undefined }),
    );

    renderPage();
    await pickTwin('twin-1');

    expect(await screen.findByText(/no standing objectives/i)).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});
