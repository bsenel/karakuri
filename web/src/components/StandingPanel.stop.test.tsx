import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// Phase 37 step 6: the pause of a standing objective asks for a reason, and a
// paused objective says who paused it, when and why, from step 5's record.
//
// vi.mock is hoisted above the imports, so the stubs have to be created inside
// the factory and reached through vi.hoisted rather than closed over.
const { get, post, can } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), can: vi.fn() }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post, put: vi.fn(), del: vi.fn() } };
});
vi.mock('@/auth/AuthProvider', () => ({ useAuth: () => ({ can }) }));

import { StandingPanel } from './StandingPanel';

const running = {
  state: {
    objective_id: 'obj-1', phase: 'reconciling', paused: false,
    criteria_met: 0.5, score_streak: 0, consecutive_failures: 0, clean_runs: 0,
  },
  history: [],
};

// paused_by and paused_at are fields the API does not return yet: step 6 adds
// them to ReconcileState beside paused_reason, from the pause's stop row.
const paused = {
  state: {
    objective_id: 'obj-1', phase: 'paused', paused: true,
    paused_reason: 'started against a merged branch',
    paused_by: 'alice',
    paused_at: '2026-11-10T09:30:00Z',
    criteria_met: 0.5, score_streak: 0, consecutive_failures: 0, clean_runs: 0,
  },
  history: [],
};

describe('StandingPanel pause control', () => {
  // Braces matter: mockReset returns the mock, and vitest runs a function
  // returned from beforeEach as that test's cleanup.
  beforeEach(() => {
    get.mockReset();
    post.mockReset();
    can.mockReset();
    can.mockReturnValue(true);
  });

  it('asks for a reason before pausing, and sends it to the pause route', async () => {
    get.mockResolvedValue(running);
    post.mockResolvedValue({});

    render(<StandingPanel objectiveID="obj-1" />);
    await waitFor(() => expect(screen.getByRole('button', { name: 'Pause' })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: 'Pause' }));

    // Opening the control pauses nothing: the reason comes first.
    expect(post).not.toHaveBeenCalled();
    const dialog = screen.getByRole('dialog', { name: 'Pause this objective' });
    const confirm = within(dialog).getByRole('button', { name: 'Pause the objective' });
    // A pause without a reason leaves nobody able to say why it stopped.
    expect(confirm).toBeDisabled();

    fireEvent.change(within(dialog).getByRole('textbox', { name: 'Reason' }), {
      target: { value: 'started against a merged branch' },
    });
    expect(confirm).toBeEnabled();
    fireEvent.click(confirm);

    await waitFor(() =>
      expect(post).toHaveBeenCalledWith('/objectives/obj-1/pause', { reason: 'started against a merged branch' }),
    );
  });

  it('shows who paused the objective, when and why', async () => {
    get.mockResolvedValue(paused);

    render(<StandingPanel objectiveID="obj-1" />);
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('started against a merged branch'));
    const status = screen.getByRole('status');
    expect(status).toHaveTextContent('paused by alice');
    // When, as a machine-readable time beside the words.
    expect(status.querySelector('time')).toHaveAttribute('datetime', '2026-11-10T09:30:00Z');
  });

  it('offers no pause to a principal without objective:pause', async () => {
    can.mockImplementation((action: string) => action !== 'objective:pause');
    get.mockResolvedValue(running);

    render(<StandingPanel objectiveID="obj-1" />);
    await waitFor(() => expect(screen.getByText('reconciling')).toBeInTheDocument());
    expect(screen.queryByRole('button', { name: 'Pause' })).not.toBeInTheDocument();
  });

  it('still shows who paused it to a principal who may not resume it', async () => {
    can.mockImplementation((action: string) => action !== 'objective:pause');
    get.mockResolvedValue(paused);

    render(<StandingPanel objectiveID="obj-1" />);
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('paused by alice'));
    expect(screen.queryByRole('button', { name: 'Resume' })).not.toBeInTheDocument();
  });
});
