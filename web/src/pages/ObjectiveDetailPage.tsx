import { useEffect, useRef, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { api } from '@/api/client';
import { streamObjective } from '@/api/sse';
import type { LoopStatus, Objective, SSEEvent } from '@/api/types';

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

  if (err) return <p className="pill red">{err}</p>;
  if (!obj) return <p className="muted">Loading…</p>;

  // Weighted-score progress: derived from the latest verify event payload OR
  // the status object when polling kicks in.
  const score = status?.weighted_score ??
    Number((events.findLast?.((e) => (e.payload as Record<string, unknown> | undefined)?.weighted_score !== undefined)?.payload as Record<string, unknown> | undefined)?.weighted_score ?? 0);

  return (
    <>
      <p className="muted small"><Link to="/objectives">← Objectives</Link></p>
      <h1>{obj.title}</h1>
      <div className="row" style={{ marginBottom: 16 }}>
        <span className="pill accent">{obj.domain}</span>
        <span className="pill">{obj.status}</span>
        {obj.twin_id && <Link to={`/twins/${obj.twin_id}`} className="small">twin: {obj.twin_id}</Link>}
      </div>

      <div className="row">
        <button className="primary" onClick={() => void startLoop()} disabled={!!loopID && !status?.completed}>
          {!loopID ? 'Start loop' : status?.completed ? 'Run again' : 'Running…'}
        </button>
        {status && (
          <span className="small muted">
            iteration {status.iteration} · {status.paused ? 'paused' : status.completed ? 'completed' : 'running'}
            {status.last_step && <> · last step: {status.last_step}</>}
          </span>
        )}
      </div>

      {obj.success_criteria && obj.success_criteria.length > 0 && (
        <div className="card" style={{ marginTop: 16 }}>
          <h3>Success criteria</h3>
          <div className="col">
            {obj.success_criteria.map((c) => (
              <div key={c.id}>
                <div className="row small">
                  <span className="grow">{c.description}</span>
                  <span className="mono muted">w={c.weight ?? 1}</span>
                </div>
                <div className="progress"><div style={{ width: `${Math.round(score * 100)}%` }} /></div>
              </div>
            ))}
          </div>
        </div>
      )}

      <h2>Loop timeline</h2>
      <div ref={eventsBoxRef} className="timeline" style={{ maxHeight: 480, overflowY: 'auto' }}>
        {events.length === 0 && <p className="muted small">Waiting for events…</p>}
        {events.map((e, i) => <EventRow key={i} event={e} />)}
      </div>
    </>
  );
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
