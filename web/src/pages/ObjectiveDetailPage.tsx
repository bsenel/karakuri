import { useEffect, useRef, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { api } from '@/api/client';
import { streamObjective } from '@/api/sse';
import { StandingPanel } from '@/components/StandingPanel';
import type { Criterion, LoopStatus, Objective, SSEEvent } from '@/api/types';

// Drill-down for one objective + the live SSE loop runner. Subscribes to the
// objective's /events stream and renders a coloured per-step timeline. The
// "Start loop" button POSTs /loops/ and stores the returned loop_id so the
// status pill can poll for the latest weighted score and iteration count.
export function ObjectiveDetailPage() {
  const { id = '' } = useParams();
  const [obj, setObj] = useState<Objective | null>(null);
  const [status, setStatus] = useState<LoopStatus | null>(null);
  const [events, setEvents] = useState<SSEEvent[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loopID, setLoopID] = useState<string | null>(null);
  const eventsBoxRef = useRef<HTMLDivElement>(null);

  const load = async () => {
    try {
      const o = await api.get<Objective>(`/objectives/${id}`);
      setObj(o);
      setErr(null);
    } catch (e) {
      setErr(String(e));
    }
  };

  useEffect(() => { void load(); /* eslint-disable-next-line react-hooks/exhaustive-deps */ }, [id]);

  // Open SSE stream once we have an objective. The browser stays subscribed
  // for the page lifetime; events are appended in order with the newest at
  // the bottom (jumps to anchor when received).
  useEffect(() => {
    if (!id) return;
    const stream = streamObjective(id, (e) => {
      setEvents((prev) => [...prev, e]);
      // Auto-scroll the timeline as new events arrive.
      requestAnimationFrame(() => {
        eventsBoxRef.current?.scrollTo({ top: eventsBoxRef.current.scrollHeight, behavior: 'smooth' });
      });
    });
    return () => stream.close();
  }, [id]);

  // Poll loop status when a loop is running.
  useEffect(() => {
    if (!loopID) return;
    let cancelled = false;
    const tick = async () => {
      try {
        const s = await api.get<LoopStatus>(`/loops/${loopID}/status`);
        if (!cancelled) setStatus(s);
      } catch {
        // Loop may not exist yet; ignore.
      }
    };
    void tick();
    const h = setInterval(tick, 2000);
    return () => { cancelled = true; clearInterval(h); };
  }, [loopID]);

  const startLoop = async () => {
    try {
      const resp = await api.post<{ loop_id: string }>('/loops/', { objective_id: id });
      setLoopID(resp.loop_id);
    } catch (e) {
      setErr(String(e));
    }
  };

  if (!obj) {
    if (!err) return <p className="muted" role="status">Loading…</p>;
    return (
      <>
        <p className="muted small"><Link to="/objectives">← Objectives</Link></p>
        <p className="pill red" role="alert">Could not load this objective: {err}</p>
        <div className="row" style={{ marginTop: 12 }}>
          <button onClick={() => void load()}>Retry</button>
        </div>
      </>
    );
  }

  return (
    <>
      <p className="muted small"><Link to="/objectives">← Objectives</Link></p>
      {/* A loop that fails to start keeps the page, so the timeline is not lost. */}
      {err && <p className="pill red" role="alert">{err}</p>}
      <h1>{obj.title}</h1>
      <div className="row" style={{ marginBottom: 16 }}>
        <span className="pill accent">{obj.domain}</span>
        <span className="pill">{obj.status}</span>
        {obj.mode === 'standing' && <span className="pill accent">standing</span>}
        {obj.twin_id && <Link to={`/twins/${obj.twin_id}`} className="small">twin: {obj.twin_id}</Link>}
      </div>

      <div className="row">
        <button className="primary" onClick={() => void startLoop()} disabled={!!loopID && !status?.completed}>
          {!loopID ? 'Start loop' : status?.completed ? 'Run again' : 'Running…'}
        </button>
        {/* Always mounted, so a screen reader hears each poll's change of state. */}
        <span className="small muted" role="status">
          {status && (
            <>
              iteration {status.iteration} · {status.paused ? 'paused' : status.completed ? 'completed' : 'running'}
              {status.last_step && <> · last step: {status.last_step}</>}
            </>
          )}
        </span>
      </div>

      <StandingPanel objectiveID={id} budget={obj?.budget} />

      {obj.success_criteria && obj.success_criteria.length > 0 && (
        <SuccessCriteria criteria={obj.success_criteria} />
      )}

      <h2>Loop timeline</h2>
      <div ref={eventsBoxRef} className="timeline" style={{ maxHeight: 480, overflowY: 'auto' }}>
        {events.length === 0 && <p className="muted small">Waiting for events…</p>}
        {events.map((e, i) => <EventRow key={i} event={e} />)}
      </div>
    </>
  );
}

// What the last verify step settled about each criterion, the same states the
// digest and `krk objective criteria` show. Required criteria are the gate:
// completion waits on each of them. Optional ones are a score, so they show as
// progress apart from the gate, and a partial score is expected rather than a
// failure.
function SuccessCriteria({ criteria }: { criteria: Criterion[] }) {
  // Absent required means required, as the server reads it.
  const gate = criteria.filter((c) => c.required !== false);
  const optional = criteria.filter((c) => c.required === false);
  const of = optional.reduce((sum, c) => sum + (c.weight ?? 0), 0);
  const earned = optional.reduce((sum, c) => sum + (c.state === 'met' ? c.weight ?? 0 : 0), 0);

  return (
    <div className="card" style={{ marginTop: 16 }}>
      <h3>Success criteria</h3>
      {gate.length > 0 && (
        <ul aria-label="Required criteria" className="col" style={{ listStyle: 'none', margin: 0, padding: 0 }}>
          {gate.map((c) => (
            <li key={c.id} className="row small">
              {/* Spaces between the parts keep the text readable, so a state
                  never runs into the description before it. */}
              <span className="grow">{c.description}</span>{' '}
              {comparison(c) && <span className="mono muted">{comparison(c)}</span>}{' '}
              {c.verifier && <span className="mono muted">{c.verifier}</span>}{' '}
              <span className={`pill ${stateColour(c.state)}`}>{stateLabel(c)}</span>
            </li>
          ))}
        </ul>
      )}
      {optional.length > 0 && (
        <>
          <p className="muted small" style={{ margin: '16px 0 8px 0' }}>
            progress {weight(earned)} of {weight(of)} on optional criteria
          </p>
          {of > 0 && <div className="progress"><div style={{ width: `${Math.round((earned / of) * 100)}%` }} /></div>}
          <ul aria-label="Optional criteria" className="col" style={{ listStyle: 'none', margin: '12px 0 0 0', padding: 0 }}>
            {optional.map((c) => (
              <li key={c.id} className="row small">
                <span className="grow">{c.description}</span>{' '}
                {comparison(c) && <span className="mono muted">{comparison(c)}</span>}{' '}
                {c.verifier && <span className="mono muted">{c.verifier}</span>}{' '}
                <span className="mono muted">w={c.weight ?? 0}</span>{' '}
                {/* An optional criterion nobody needed is not a failure. */}
                <span className={`pill ${c.state === 'not_met' ? '' : stateColour(c.state)}`}>
                  {c.state === 'not_met' ? 'not earned' : stateLabel(c)}
                </span>
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  );
}

// A criterion with no state was never run by its verifier: inconclusive:
// not_run, never met.
function stateLabel(c: Criterion): string {
  if (!c.state) return 'inconclusive: not_run';
  if (c.state === 'inconclusive') return c.reason ? `inconclusive: ${c.reason}` : 'inconclusive';
  return c.state;
}

// A measured criterion's `metric op target unit`, then its baseline when it
// has one, as `krk objective criteria` prints it: a target set against a start
// that was already bad is read beside the number it is measured from. An
// at_start baseline not measured yet says so rather than showing a number.
function comparison(c: Criterion): string {
  if (!c.metric || c.target == null) return '';
  const shown = [c.metric, c.op, c.target, c.unit].filter((p) => p !== undefined && p !== '').join(' ');
  if (c.baseline == null) return shown;
  const source = c.baseline === 'at_start' ? 'at_start' : 'fixed';
  if (c.baseline_value == null) return `${shown}, baseline ${source}, not measured`;
  const value = [c.baseline_value, c.unit].filter((p) => p !== undefined && p !== '').join(' ');
  return `${shown}, baseline ${value} (${source})`;
}

function stateColour(state: Criterion['state']): string {
  if (state === 'met') return 'green';
  if (state === 'not_met') return 'red';
  return 'amber';
}

// A criterion weight to two places without trailing zeros, keeping one so a
// whole weight reads as 1.0, as the digest renders it.
function weight(w: number): string {
  const r = Math.round(w * 100) / 100;
  return Number.isInteger(r) ? r.toFixed(1) : String(r);
}

function EventRow({ event }: { event: SSEEvent }) {
  const step = (event.payload?.step as string | undefined) ?? '';
  const t = new Date(event.timestamp);
  return (
    <details className={`event ${step}`}>
      <summary>
        <time>{t.toLocaleTimeString()}</time>
        <span className="mono small">{event.type}{step && `:${step}`}</span>
        <EventSummary event={event} />
      </summary>
      <pre className="payload">{JSON.stringify(event.payload ?? {}, null, 2)}</pre>
    </details>
  );
}

function EventSummary({ event }: { event: SSEEvent }) {
  const p = event.payload as Record<string, unknown> | undefined;
  if (!p) return <span />;
  const bits: string[] = [];
  if (p.weighted_score !== undefined) bits.push(`score=${Number(p.weighted_score).toFixed(2)}`);
  if (p.action_count !== undefined)   bits.push(`actions=${p.action_count}`);
  if (p.plan_count !== undefined)     bits.push(`plans=${p.plan_count}`);
  if (p.top_confidence !== undefined) bits.push(`conf=${Number(p.top_confidence).toFixed(2)}`);
  if (p.escalated)                    bits.push('escalated');
  return <span className="muted small">{bits.join(' · ')}</span>;
}
