import { useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { api } from '@/api/client';
import type { AuditEvent } from '@/api/types';

// AuditPage surfaces the authority-bounds audit log served by
// GET /api/v1/audit (Phase 13). Filter set matches `krk audit`:
//   --objective, --agent, --kind, --bounds-violation, --since, --limit
//
// Row click expands the payload JSON inline. Modification rows (Phase
// 13.5) render the structured diff (removed_actions, added_constraints,
// revised_confidence) in human form on top of the raw payload.
export function AuditPage() {
  const [params, setParams] = useSearchParams();
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [expanded, setExpanded] = useState<string | null>(params.get('event'));

  const objective = params.get('objective') ?? '';
  const agent = params.get('agent') ?? '';
  const kind = params.get('kind') ?? '';
  const violationsOnly = params.get('bounds_violation') === 'true';
  const since = params.get('since') ?? '';
  const limit = params.get('limit') ?? '100';

  const queryString = useMemo(() => {
    const qs = new URLSearchParams();
    if (objective) qs.set('objective_id', objective);
    if (agent) qs.set('agent_id', agent);
    if (kind) qs.set('kind', kind);
    if (violationsOnly) qs.set('bounds_violation', 'true');
    if (since) qs.set('since', since);
    if (limit) qs.set('limit', limit);
    return qs.toString();
  }, [objective, agent, kind, violationsOnly, since, limit]);

  const load = async () => {
    setLoading(true);
    try {
      const list = await api.get<AuditEvent[]>('/audit' + (queryString ? '?' + queryString : ''));
      setEvents(list ?? []);
      setErr(null);
    } catch (e) { setErr(String(e)); }
    finally { setLoading(false); }
  };
  useEffect(() => { void load(); }, [queryString]);

  const setParam = (k: string, v: string) => {
    const next = new URLSearchParams(params);
    if (v) next.set(k, v); else next.delete(k);
    setParams(next, { replace: true });
  };

  return (
    <>
      <h1>Audit Log</h1>
      <p className="muted small">
        Authority-bounds escalations, approvals, modifications, rejections, and tool executions
        — same filters as <code>krk audit</code>.
      </p>

      <div className="card" style={{ marginTop: 12 }}>
        <div className="row" style={{ gap: 8, flexWrap: 'wrap' }}>
          <label className="col" style={{ flex: '1 1 180px' }}>
            <span className="small muted">Objective</span>
            <input value={objective} onChange={(e) => setParam('objective', e.target.value)} placeholder="objective ID" />
          </label>
          <label className="col" style={{ flex: '1 1 180px' }}>
            <span className="small muted">Agent</span>
            <input value={agent} onChange={(e) => setParam('agent', e.target.value)} placeholder="agent ID" />
          </label>
          <label className="col" style={{ flex: '1 1 140px' }}>
            <span className="small muted">Kind</span>
            <select value={kind} onChange={(e) => setParam('kind', e.target.value)}>
              <option value="">all</option>
              <option value="execute">execute</option>
              <option value="escalation">escalation</option>
              <option value="approval">approval</option>
              <option value="modification">modification</option>
              <option value="rejection">rejection</option>
              <option value="authz_denied">authz_denied</option>
            </select>
          </label>
          <label className="col" style={{ flex: '1 1 200px' }}>
            <span className="small muted">Since (RFC3339)</span>
            <input value={since} onChange={(e) => setParam('since', e.target.value)} placeholder="2026-06-01T00:00:00Z" />
          </label>
          <label className="row" style={{ alignItems: 'center', marginTop: 16 }}>
            <input
              type="checkbox"
              checked={violationsOnly}
              onChange={(e) => setParam('bounds_violation', e.target.checked ? 'true' : '')}
            />
            <span style={{ marginLeft: 6 }}>violations only</span>
          </label>
        </div>
      </div>

      {err && <p className="pill red" style={{ marginTop: 12 }}>{err}</p>}
      {loading && <p className="muted small" style={{ marginTop: 12 }}>Loading…</p>}

      <div className="col" style={{ marginTop: 16 }}>
        {!loading && events.length === 0 && <p className="muted">No matching audit events.</p>}
        {events.map((ev) => (
          <AuditEventRow
            key={ev.id}
            ev={ev}
            expanded={expanded === ev.id}
            onToggle={() => setExpanded(expanded === ev.id ? null : ev.id)}
          />
        ))}
      </div>
    </>
  );
}

interface RowProps {
  ev: AuditEvent;
  expanded: boolean;
  onToggle: () => void;
}

function AuditEventRow({ ev, expanded, onToggle }: RowProps) {
  const payload = useMemo(() => {
    if (!ev.payload_json) return null;
    try { return JSON.parse(ev.payload_json) as Record<string, unknown>; } catch { return null; }
  }, [ev.payload_json]);

  const mods = payload && typeof payload.modifications === 'object' && payload.modifications !== null
    ? (payload.modifications as Record<string, unknown>)
    : null;

  return (
    <div className="card">
      <div className="row" style={{ cursor: 'pointer' }} onClick={onToggle}>
        <KindPill kind={ev.kind} bounds={ev.bounds_violation} />
        <span className="muted small">{new Date(ev.created_at).toLocaleString()}</span>
        {ev.escalation_reason && <span className="small">{ev.escalation_reason}</span>}
        <span className="grow" />
        {ev.approver && <span className="small">approver: <strong>{ev.approver}</strong></span>}
        <span className="mono small muted">{ev.id}</span>
      </div>
      {ev.objective_id && (
        <div className="row small muted" style={{ marginTop: 4 }}>
          objective: <code>{ev.objective_id}</code>
          {ev.agent_id && <> · agent: <code>{ev.agent_id}</code></>}
          {ev.capability && <> · capability: <code>{ev.capability}</code></>}
        </div>
      )}
      {expanded && (
        <div style={{ marginTop: 12 }}>
          {mods && <ModificationDiff mods={mods} />}
          {payload ? (
            <pre className="payload" style={{ maxHeight: 360, overflow: 'auto' }}>
              {JSON.stringify(payload, null, 2)}
            </pre>
          ) : (
            <p className="muted small">no payload</p>
          )}
        </div>
      )}
    </div>
  );
}

function KindPill({ kind, bounds }: { kind: string; bounds?: boolean }) {
  let cls = 'pill';
  switch (kind) {
    case 'escalation': cls += ' amber'; break;
    case 'approval': cls += ' green'; break;
    case 'modification': cls += ' blue'; break;
    case 'rejection': cls += ' red'; break;
    // A refused API request (Phase 14). Same colour as a rejection: both are
    // "this did not happen, and here is who wanted it to".
    case 'authz_denied': cls += ' red'; break;
    case 'execute':
    default: cls += ' grey'; break;
  }
  return (
    <span className={cls}>
      {kind}{bounds ? ' · bounds' : ''}
    </span>
  );
}

function ModificationDiff({ mods }: { mods: Record<string, unknown> }) {
  const removed = Array.isArray(mods.removed_actions) ? (mods.removed_actions as string[]) : [];
  const constraints = Array.isArray(mods.added_constraints) ? (mods.added_constraints as string[]) : [];
  const floor = typeof mods.revised_confidence === 'number' ? mods.revised_confidence : undefined;
  return (
    <div className="card" style={{ background: 'var(--panel-2, #1a1a1a)', marginBottom: 12 }}>
      <strong className="small">Modification diff</strong>
      {removed.length > 0 && (
        <div className="small" style={{ marginTop: 4 }}>
          <span className="muted">dropped:</span> {removed.map((r) => <code key={r} style={{ marginRight: 6 }}>{r}</code>)}
        </div>
      )}
      {constraints.length > 0 && (
        <div className="small" style={{ marginTop: 4 }}>
          <span className="muted">constraints:</span>
          <ul style={{ marginTop: 4 }}>
            {constraints.map((c, i) => <li key={i}>{c}</li>)}
          </ul>
        </div>
      )}
      {floor !== undefined && (
        <div className="small" style={{ marginTop: 4 }}>
          <span className="muted">confidence floor:</span> <strong>{floor.toFixed(2)}</strong>
        </div>
      )}
    </div>
  );
}
