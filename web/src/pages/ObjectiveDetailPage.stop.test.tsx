import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// Phase 37 step 6: the running loop this page shows can be stopped with a
// reason, and a stopped loop says who stopped it, when and why.
//
// vi.mock is hoisted above the imports, so the stubs have to be created inside
// the factory and reached through vi.hoisted rather than closed over.
const { get, post, can } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), can: vi.fn() }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post, put: vi.fn(), del: vi.fn() } };
});
vi.mock('@/auth/AuthProvider', () => ({ useAuth: () => ({ can }) }));
// jsdom has no EventSource, and the timeline is not what these tests are about.
vi.mock('@/api/sse', () => ({ streamObjective: () => ({ close: () => {} }) }));
// The standing panel has its own tests, StandingPanel.stop.test.tsx among them.
vi.mock('@/components/StandingPanel', () => ({ StandingPanel: () => null }));

import { ObjectiveDetailPage } from './ObjectiveDetailPage';

// The page links back to the list, so it needs a router.
const renderPage = () => render(<MemoryRouter><ObjectiveDetailPage /></MemoryRouter>);

const objective = { id: 'o1', title: 'Ship the docs', domain: 'software', status: 'active', created_at: '', updated_at: '' };

const runningLoop = { loop_id: 'l1', objective_id: 'o1', iteration: 1, paused: false, completed: false };

// stopped, stopped_by, stop_reason and stopped_at are fields GET
// /loops/{id}/status does not return yet: step 6 adds them to loop.Status from
// the stored row's StoppedBy and StopReason (Phase 37 step 3).
const stoppedLoop = {
  ...runningLoop,
  completed: true,
  stopped: true,
  stopped_by: 'alice',
  stop_reason: 'started against a merged branch',
  stopped_at: '2026-11-10T09:30:00Z',
};

describe('ObjectiveDetailPage loop stop', () => {
  // Braces matter: mockReset returns the mock, and vitest runs a function
  // returned from beforeEach as that test's cleanup — it would call get().
  beforeEach(() => {
    get.mockReset();
    post.mockReset();
    can.mockReset();
    can.mockReturnValue(true);
  });

  // Starts a loop and answers its status with whatever `loop()` returns at the
  // time, so a test can change the loop's state after the stop.
  const startLoop = async (loop: () => unknown) => {
    get.mockImplementation((path: string) => Promise.resolve(path.startsWith('/loops/') ? loop() : objective));
    renderPage();
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Ship the docs' })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: 'Start loop' }));
  };

  it('asks for a reason before stopping the running loop, and sends it to the stop route', async () => {
    let stopped = false;
    post.mockImplementation((path: string) => {
      if (path === '/loops/l1/stop') {
        stopped = true;
        return Promise.resolve({ loop_id: 'l1', found: 'running', objective_id: 'o1', objective_paused: false });
      }
      return Promise.resolve({ loop_id: 'l1' });
    });
    await startLoop(() => (stopped ? stoppedLoop : runningLoop));

    await waitFor(() => expect(screen.getByRole('button', { name: 'Stop loop' })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: 'Stop loop' }));

    // Opening the control stops nothing: the reason comes first.
    expect(post).not.toHaveBeenCalledWith('/loops/l1/stop', expect.anything());
    const dialog = screen.getByRole('dialog', { name: 'Stop this loop' });
    const confirm = within(dialog).getByRole('button', { name: 'Stop the loop' });
    expect(confirm).toBeDisabled();

    fireEvent.change(within(dialog).getByRole('textbox', { name: 'Reason' }), {
      target: { value: 'started against a merged branch' },
    });
    fireEvent.click(confirm);

    await waitFor(() =>
      expect(post).toHaveBeenCalledWith('/loops/l1/stop', { reason: 'started against a merged branch' }),
    );
    // The page asks for the loop's status again once the stop has answered,
    // rather than waiting for the next poll, and shows it stopped.
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('stopped by alice'));
  });

  it('shows who stopped the loop, when and why, and not that it completed', async () => {
    post.mockResolvedValue({ loop_id: 'l1' });
    await startLoop(() => stoppedLoop);

    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('stopped by alice'));
    const status = screen.getByRole('status');
    expect(status).toHaveTextContent('started against a merged branch');
    expect(status).not.toHaveTextContent('completed');
    expect(status.querySelector('time')).toHaveAttribute('datetime', '2026-11-10T09:30:00Z');
    // A stopped loop has nothing left to stop.
    expect(screen.queryByRole('button', { name: 'Stop loop' })).not.toBeInTheDocument();
  });

  it('offers no stop to a principal without loop:stop', async () => {
    can.mockImplementation((action: string) => action !== 'loop:stop');
    post.mockResolvedValue({ loop_id: 'l1' });
    await startLoop(() => runningLoop);

    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('running'));
    expect(screen.queryByRole('button', { name: 'Stop loop' })).not.toBeInTheDocument();
  });
});
