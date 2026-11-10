import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// Phase 37 step 6: one control that stops every running loop and pauses every
// standing objective the caller may stop, through POST /stop-all (step 4).
//
// vi.mock is hoisted above the imports, so the stubs have to be created inside
// the factory and reached through vi.hoisted rather than closed over.
const { get, post, can } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), can: vi.fn() }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post, put: vi.fn(), del: vi.fn() } };
});
vi.mock('@/auth/AuthProvider', () => ({ useAuth: () => ({ can }) }));

import { StopAllControl } from './StopAllControl';

// Opens the control and confirms it with a reason.
const stopAll = (reason: string) => {
  fireEvent.click(screen.getByRole('button', { name: 'Stop all' }));
  const dialog = screen.getByRole('dialog', { name: 'Stop every run' });
  fireEvent.change(within(dialog).getByRole('textbox', { name: 'Reason' }), { target: { value: reason } });
  fireEvent.click(within(dialog).getByRole('button', { name: 'Stop everything' }));
};

describe('StopAllControl', () => {
  // Braces matter: mockReset returns the mock, and vitest runs a function
  // returned from beforeEach as that test's cleanup.
  beforeEach(() => {
    get.mockReset();
    post.mockReset();
    can.mockReset();
    can.mockReturnValue(true);
  });

  it('asks for a reason before stopping anything, and sends it to POST /stop-all', async () => {
    post.mockResolvedValue({ loops_stopped: [], objectives_paused: [], not_reached: [] });

    render(<StopAllControl />);
    fireEvent.click(screen.getByRole('button', { name: 'Stop all' }));

    // Opening the control stops nothing: the reason comes first.
    expect(post).not.toHaveBeenCalled();
    const dialog = screen.getByRole('dialog', { name: 'Stop every run' });
    const confirm = within(dialog).getByRole('button', { name: 'Stop everything' });
    expect(confirm).toBeDisabled();

    fireEvent.change(within(dialog).getByRole('textbox', { name: 'Reason' }), {
      target: { value: 'something is wrong everywhere' },
    });
    fireEvent.click(confirm);

    await waitFor(() =>
      expect(post).toHaveBeenCalledWith('/stop-all', { reason: 'something is wrong everywhere' }),
    );
  });

  it('shows what was stopped, what was paused, and what was not reached and why', async () => {
    post.mockResolvedValue({
      loops_stopped: [{ loop_id: 'loop-a', found: 'running' }],
      objectives_paused: [
        { objective_id: 'obj-b', pass_cancelled: true },
        { objective_id: 'obj-c', pass_cancelled: false },
      ],
      not_reached: [{ kind: 'objective', id: 'obj-d', reason: 'not permitted' }],
    });

    render(<StopAllControl />);
    stopAll('something is wrong everywhere');

    await waitFor(() => expect(screen.getByRole('list', { name: 'Loops stopped' })).toBeInTheDocument());
    expect(screen.getByRole('list', { name: 'Loops stopped' })).toHaveTextContent('loop-a');

    const paused = within(screen.getByRole('list', { name: 'Objectives paused' })).getAllByRole('listitem');
    expect(paused).toHaveLength(2);
    // The pass a pause cut is named, so nobody wonders whether it finished.
    expect(paused[0]).toHaveTextContent('obj-b');
    expect(paused[0]).toHaveTextContent('pass cancelled');
    expect(paused[1]).toHaveTextContent('obj-c');
    expect(paused[1]).not.toHaveTextContent('pass cancelled');

    // A caller whose scope is narrower than the deployment is never told
    // everything was stopped when it was not.
    const notReached = screen.getByRole('list', { name: 'Not reached' });
    expect(notReached).toHaveTextContent('objective');
    expect(notReached).toHaveTextContent('obj-d');
    expect(notReached).toHaveTextContent('not permitted');
  });

  it('says plainly when there was nothing left to stop', async () => {
    // What a second call answers: nothing listed, and nothing not reached.
    post.mockResolvedValue({ loops_stopped: null, objectives_paused: null, not_reached: null });

    render(<StopAllControl />);
    stopAll('again');

    await waitFor(() => expect(screen.getByText(/nothing was stopped or paused/i)).toBeInTheDocument());
    expect(screen.queryByRole('list', { name: 'Not reached' })).not.toBeInTheDocument();
  });

  it('is offered to a principal who may stop loops or pause objectives', () => {
    can.mockImplementation((action: string) => action === 'objective:pause');
    render(<StopAllControl />);
    expect(screen.getByRole('button', { name: 'Stop all' })).toBeInTheDocument();
  });

  it('is absent for a principal who may neither stop a loop nor pause an objective', () => {
    can.mockImplementation((action: string) => action !== 'loop:stop' && action !== 'objective:pause');
    render(<StopAllControl />);
    expect(screen.queryByRole('button', { name: 'Stop all' })).not.toBeInTheDocument();
  });
});
