import { render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const { get } = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post: vi.fn(), put: vi.fn(), del: vi.fn() } };
});
vi.mock('@/auth/AuthProvider', () => ({ useAuth: () => ({ can: () => true }) }));

import { APIError } from '@/api/client';
import { StandingPanel, describe as describeOutcome, isNotStanding, when } from './StandingPanel';
import type { ReconcileOutcome } from '@/api/types';

function outcome(over: Partial<ReconcileOutcome> = {}): ReconcileOutcome {
  return {
    id: 'o1',
    objective_id: 'obj-1',
    trigger: 'drift',
    drift: { changed: false, blind: false },
    criteria_met: 1,
    converged: false,
    escalated: false,
    started_at: '2026-03-04T12:00:00Z',
    ended_at: '2026-03-04T12:00:01Z',
    ...over,
  };
}

describe('describe(outcome)', () => {
  it('names the cheap case plainly rather than leaving it blank', () => {
    // "Looked, nothing moved" is information — and the majority of rows. A
    // blank cell there reads as a bug in the panel.
    expect(describeOutcome(outcome({ trigger: 'schedule' })))
      .toBe('sensed; nothing moved, nothing spent');
  });

  it('says when a fingerprint proved nothing', () => {
    expect(describeOutcome(outcome({ drift: { changed: false, blind: true } })))
      .toContain('nothing could be hashed');
  });

  it('distinguishes drift reported from drift acted on', () => {
    const reported = outcome({ drift: { changed: true, blind: false, environments: ['git'] } });
    expect(describeOutcome(reported)).toContain('reported, not acted on');

    const acted = outcome({
      loop_id: 'l1',
      drift: { changed: true, blind: false, environments: ['git'] },
    });
    expect(describeOutcome(acted)).toContain('reconciled after drift');
  });

  it('does not report a deferred pass as having spent nothing', () => {
    // A budget deferral carries no loop, so before Phase 23's close-out it
    // fell through to the sense case and rendered as "nothing moved, nothing
    // spent" — the opposite of what happened, on the one row where the
    // operator most needs the truth.
    const d = describeOutcome(outcome({
      deferred: 'budget_exhausted',
      deferred_until: '2026-03-05T00:00:00Z',
    }));
    expect(d).toContain('spent its ceiling');
    expect(d).not.toContain('nothing spent');
    // And it says nobody has to act, because a budget clears itself.
    expect(d).toContain('clears itself');
  });

  it('names a deferral it does not have special wording for', () => {
    expect(describeOutcome(outcome({ deferred: 'quiet_hours' }))).toContain('quiet_hours');
  });

  it('reports an escalation as waiting, not as a failure', () => {
    const e = describeOutcome(outcome({ loop_id: 'l1', escalated: true }));
    expect(e).toContain('waiting on a decision');
    expect(e).not.toContain('failed');
  });

  it('shows the error when there is one', () => {
    expect(describeOutcome(outcome({ loop_id: 'l1', error: 'adapter unreachable' })))
      .toBe('failed: adapter unreachable');
  });
});

describe('isNotStanding()', () => {
  it('reads a 404 as "this objective is one-shot", not as a failure', () => {
    // The panel renders nothing at all for a one-shot objective. Getting this
    // wrong would put a red error box on every objective in the deployment
    // that is not standing, which is most of them.
    expect(isNotStanding(new APIError(404, 'objective is not standing'))).toBe(true);
  });

  it('does not swallow other failures', () => {
    expect(isNotStanding(new APIError(500, 'database is down'))).toBe(false);
    expect(isNotStanding(new APIError(403, 'forbidden'))).toBe(false);
    // Matched on the status, so an unrelated error whose text happens to
    // mention 404 is still an error.
    expect(isNotStanding(new Error('upstream returned 404 for something else'))).toBe(false);
    expect(isNotStanding(undefined)).toBe(false);
  });
});

describe('when()', () => {
  it('reads a null as "never" rather than as a missing value', () => {
    // Null next_due_at is a real state — an objective that reconciles only
    // when asked — and rendering it as an empty cell would look like a bug.
    expect(when(null)).toBe('never');
    expect(when(undefined)).toBe('never');
    expect(when('2026-03-04T12:00:00Z')).not.toBe('never');
  });
});

describe('StandingPanel', () => {
  beforeEach(() => get.mockReset());

  it('shows the phase, the autonomy it has earned, and why it is stopped', async () => {
    get.mockResolvedValue({
      state: {
        objective_id: 'obj-1',
        phase: 'paused',
        paused: true,
        paused_reason: '3 consecutive failed reconciles; last error: adapter unreachable',
        criteria_met: 0.5,
        score_streak: 0,
        consecutive_failures: 3,
        autonomy: 'propose',
        clean_runs: 0,
        next_due_at: null,
      },
      history: [outcome({ trigger: 'schedule' })],
    });

    render(<StandingPanel objectiveID="obj-1" />);

    await waitFor(() => expect(screen.getByText('paused')).toBeInTheDocument());
    expect(screen.getByText('propose')).toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveTextContent('3 consecutive failed reconciles');
    // A paused objective offers Resume, not Pause.
    expect(screen.getByRole('button', { name: 'Resume' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Pause' })).not.toBeInTheDocument();
    // Never due, and it says so rather than showing an empty cell.
    expect(screen.getAllByText('never').length).toBeGreaterThan(0);
  });

  it('lists the cheap passes alongside the expensive ones', async () => {
    get.mockResolvedValue({
      state: {
        objective_id: 'obj-1', phase: 'idle', paused: false,
        criteria_met: 1, score_streak: 0, consecutive_failures: 0, clean_runs: 2,
      },
      history: [
        outcome({ id: 'a', trigger: 'schedule' }),
        outcome({ id: 'b', loop_id: 'l1', converged: true }),
      ],
    });

    render(<StandingPanel objectiveID="obj-1" />);
    await waitFor(() => expect(screen.getByText('nothing moved, nothing spent', { exact: false })).toBeInTheDocument());
    expect(screen.getByText('reconciled — converged')).toBeInTheDocument();
  });
});
