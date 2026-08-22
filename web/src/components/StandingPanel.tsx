import { useCallback, useEffect, useState } from 'react';
import { api, APIError } from '@/api/client';
import { useAuth } from '@/auth/AuthProvider';
import type { AutonomyLevel, Budget, ReconcileOutcome, ReconcileView } from '@/api/types';

/**
 * The control loop over one standing objective: what it is doing, when it is
 * next due, and what it has been doing.
 *
 * The history deliberately shows the cheap sense-only passes alongside the
 * expensive ones. They are the majority, and they are the evidence the
 * two-tier split is working — a list holding only the reconciles would make
 * the system look like it was barely watching.
 */
/**
 * The budget comes from the objective rather than the reconcile state, because
 * a ceiling is a declaration an operator made and the state is what the
 * supervisor did with it. The panel shows both so a deferred pass in the
 * history has its reason on the same screen.
 */
export function StandingPanel({ objectiveID, budget }: { objectiveID: string; budget?: Budget }) {
  const { can } = useAuth();
  const [view, setView] = useState<ReconcileView | null>(null);
  const [standing, setStanding] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setView(await api.get<ReconcileView>(`/objectives/${objectiveID}/reconcile`));
      setStanding(true);
      setErr(null);
    } catch (e) {
      if (isNotStanding(e)) {
        setStanding(false);
        return;
      }
      setErr(String(e));
    }
  }, [objectiveID]);

  useEffect(() => { void load(); }, [load]);

  // Poll while something is happening. Idle objectives are left alone: a
  // standing objective can sit converged for weeks, and polling it every few
  // seconds would be the one expensive thing about a design built to be cheap.
  useEffect(() => {
    const phase = view?.state.phase;
    if (phase !== 'sensing' && phase !== 'reconciling') return;
    const h = setInterval(() => { void load(); }, 3000);
    return () => clearInterval(h);
  }, [view?.state.phase, load]);

  const act = async (path: string, body?: unknown) => {
    setBusy(true);
    try {
      await api.post(`/objectives/${objectiveID}/${path}`, body ?? {});
      await load();
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  };

  if (!standing) return null;
  if (err) return <p className="pill red">{err}</p>;
  if (!view) return <p className="muted small">Loading control loop…</p>;

  const { state, history } = view;

  return (
    <div className="card" style={{ marginTop: 16 }}>
      <div className="row">
        <h3 className="grow">Standing</h3>
        <span className={`pill ${state.paused ? 'red' : 'accent'}`}>
          {state.paused ? 'paused' : state.phase}
        </span>
        {state.autonomy && <AutonomyPill level={state.autonomy} />}
      </div>

      {state.paused && state.paused_reason && (
        <p className="small" role="status">{state.paused_reason}</p>
      )}

      <dl className="row small muted" style={{ gap: 24, flexWrap: 'wrap' }}>
        <Fact label="Next due" value={when(state.next_due_at)} />
        <Fact label="Next sense" value={when(state.next_sense_at)} />
        <Fact label="Next reconcile" value={when(state.next_reconcile_at)} />
        <Fact label="Last converged" value={when(state.last_converged_at)} />
        <Fact label="Criteria met" value={`${Math.round(state.criteria_met * 100)}%`} />
        {state.consecutive_failures > 0 && (
          <Fact label="Consecutive failures" value={String(state.consecutive_failures)} />
        )}
        {state.clean_runs > 0 && <Fact label="Clean runs" value={String(state.clean_runs)} />}
        {budget?.daily ? <Fact label="Daily ceiling" value={budget.daily.toFixed(2)} /> : null}
        {budget?.per_reconcile ? <Fact label="Per-pass ceiling" value={budget.per_reconcile.toFixed(2)} /> : null}
      </dl>

      <div className="row">
        {can('objective:reconcile') && (
          <button onClick={() => void act('reconcile')} disabled={busy || state.paused}>
            Reconcile now
          </button>
        )}
        {can('objective:pause') && (
          state.paused
            ? <button className="primary" onClick={() => void act('resume')} disabled={busy}>Resume</button>
            : <button onClick={() => void act('pause', { reason: 'paused from the console' })} disabled={busy}>Pause</button>
        )}
      </div>

      <h4>Recent passes</h4>
      {history.length === 0 && <p className="muted small">Nothing yet.</p>}
      <table className="small">
        <thead>
          <tr>
            <th scope="col">When</th>
            <th scope="col">Trigger</th>
            <th scope="col">What happened</th>
            <th scope="col">Score</th>
          </tr>
        </thead>
        <tbody>
          {history.map((o) => <OutcomeRow key={o.id} outcome={o} />)}
        </tbody>
      </table>
    </div>
  );
}

function OutcomeRow({ outcome }: { outcome: ReconcileOutcome }) {
  return (
    <tr>
      <td><time dateTime={outcome.started_at}>{new Date(outcome.started_at).toLocaleString()}</time></td>
      <td><span className="pill">{outcome.trigger || 'sense'}</span></td>
      <td>{describe(outcome)}</td>
      <td className="mono">{Math.round(outcome.criteria_met * 100)}%</td>
    </tr>
  );
}

/**
 * One sentence for what a pass did. The cheap case is named plainly rather
 * than left blank — "looked, nothing moved" is information, and a blank cell
 * reads as a bug.
 */
export function describe(o: ReconcileOutcome): string {
  if (o.error) return `failed: ${o.error}`;
  // Before the sense case, because a deferred pass carries no loop and would
  // otherwise read as "nothing moved, nothing spent" — which for a budget is
  // the opposite of what happened.
  if (o.deferred) {
    const until = o.deferred_until ? `, resumes ${new Date(o.deferred_until).toLocaleString()}` : '';
    if (o.deferred === 'budget_exhausted') {
      return `deferred — spent its ceiling${until}. Nothing to do; a budget clears itself`;
    }
    return `deferred — ${o.deferred}${until}`;
  }
  if (o.escalated) return 'escalated — waiting on a decision';
  if (!o.loop_id) {
    if (o.drift.blind) return 'sensed; nothing could be hashed, so the schedule decides';
    if (o.drift.changed) return `drift in ${(o.drift.environments ?? []).join(', ')} — reported, not acted on`;
    return 'sensed; nothing moved, nothing spent';
  }
  if (o.converged) return 'reconciled — converged';
  if (o.drift.changed) return `reconciled after drift in ${(o.drift.environments ?? []).join(', ')}`;
  return 'reconciled';
}

function AutonomyPill({ level }: { level: AutonomyLevel }) {
  const label: Record<AutonomyLevel, string> = {
    sense: 'observe only',
    propose: 'proposes, never acts',
    act_with_notice: 'acts, reports everything',
    act: 'acts',
  };
  return <span className="pill" title={label[level]}>{level}</span>;
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd className="mono">{value}</dd>
    </div>
  );
}

/**
 * A 404 from the control-loop endpoint is the ordinary answer for a one-shot
 * objective, not a failure worth showing as one.
 *
 * Matched on the status rather than on the message, so a 404 whose body
 * happens to say something else — or an unrelated error whose body happens to
 * contain "404" — lands where it should.
 */
export function isNotStanding(e: unknown): boolean {
  return e instanceof APIError && e.status === 404;
}

/** Null is "never on its own", which is a real state and not a missing value. */
export function when(iso?: string | null): string {
  if (!iso) return 'never';
  return new Date(iso).toLocaleString();
}
