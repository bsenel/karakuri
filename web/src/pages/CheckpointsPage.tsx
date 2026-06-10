import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '@/api/client';
import type { Checkpoint, CheckpointModifications } from '@/api/types';
import { ModifyCheckpointDialog } from '@/components/ModifyCheckpointDialog';

export function CheckpointsPage() {
  const [items, setItems] = useState<Checkpoint[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [modifying, setModifying] = useState<Checkpoint | null>(null);

  const load = async () => {
    try {
      const list = await api.get<Checkpoint[]>('/checkpoints');
      setItems(list ?? []);
      setErr(null);
    } catch (e) { setErr(String(e)); }
  };
  useEffect(() => { void load(); }, []);

  const resolve = async (id: string, decision: 'approve' | 'reject') => {
    setBusy(id);
    try {
      await api.post(`/checkpoints/${id}/resolve`, { decision });
      await load();
    } catch (e) { setErr(String(e)); }
    finally { setBusy(null); }
  };

  const submitModify = async (id: string, note: string, modifications: CheckpointModifications) => {
    await api.post(`/checkpoints/${id}/resolve`, { decision: 'modify', note, modifications });
    await load();
  };

  return (
    <>
      <h1>Checkpoints</h1>
      <p className="muted small">
        Pending escalations from running loops. Approve to let the loop continue,
        modify to revise the plan with feedback, or reject to terminate the loop.
      </p>

      {err && <p className="pill red">{err}</p>}

      <div className="col" style={{ marginTop: 16 }}>
        {items.length === 0 && <p className="muted">No pending checkpoints.</p>}
        {items.map((c) => (
          <div key={c.id} className="card">
            <div className="row">
              <span className="pill amber">pending</span>
              <span className="muted small">{new Date(c.created_at).toLocaleString()}</span>
              <span className="grow" />
              <Link to={`/objectives/${c.objective_id}`} className="small">objective ↗</Link>
              {c.audit_event_id && (
                <Link to={`/audit?event=${c.audit_event_id}`} className="small" style={{ marginLeft: 8 }}>
                  audit ↗
                </Link>
              )}
            </div>
            <h3 style={{ marginTop: 8 }}>{c.reason}</h3>

            {(c.capability || c.confidence !== undefined) && (
              <div className="row small" style={{ marginTop: 4, gap: 12 }}>
                {c.capability && <span><strong>capability:</strong> <code>{c.capability}</code></span>}
                {c.confidence !== undefined && <span><strong>confidence:</strong> {c.confidence.toFixed(2)}</span>}
              </div>
            )}

            {c.actions && c.actions.length > 0 && (
              <details style={{ marginTop: 8 }}>
                <summary className="small">Proposed actions ({c.actions.length})</summary>
                <ul style={{ marginTop: 4 }}>
                  {c.actions.map((a, i) => (
                    <li key={i}>
                      <code>{a.capability}</code>
                      {a.reason && <span className="muted small"> — {a.reason}</span>}
                    </li>
                  ))}
                </ul>
              </details>
            )}

            <div className="row" style={{ marginTop: 12 }}>
              <button className="primary" disabled={busy === c.id} onClick={() => void resolve(c.id, 'approve')}>Approve</button>
              <button disabled={busy === c.id} onClick={() => setModifying(c)}>Modify…</button>
              <button className="danger" disabled={busy === c.id} onClick={() => void resolve(c.id, 'reject')}>Reject</button>
              <span className="grow" />
              <span className="mono small muted">{c.id}</span>
            </div>
          </div>
        ))}
      </div>

      {modifying && (
        <ModifyCheckpointDialog
          checkpoint={modifying}
          onClose={() => setModifying(null)}
          onSubmit={(note, mods) => submitModify(modifying.id, note, mods)}
        />
      )}
    </>
  );
}
