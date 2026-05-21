import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '@/api/client';
import type { Checkpoint } from '@/api/types';

export function CheckpointsPage() {
  const [items, setItems] = useState<Checkpoint[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const load = async () => {
    try {
      const list = await api.get<Checkpoint[]>('/checkpoints');
      setItems(list ?? []);
      setErr(null);
    } catch (e) { setErr(String(e)); }
  };
  useEffect(() => { void load(); }, []);

  const resolve = async (id: string, decision: 'approve' | 'reject' | 'modify') => {
    setBusy(id);
    try {
      await api.post(`/checkpoints/${id}/resolve`, { decision });
      await load();
    } catch (e) { setErr(String(e)); }
    finally { setBusy(null); }
  };

  return (
    <>
      <h1>Checkpoints</h1>
      <p className="muted small">Pending escalations from running loops. Approve to let the loop continue.</p>

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
            </div>
            <h3 style={{ marginTop: 8 }}>{c.reason}</h3>
            {c.context && (
              <pre className="payload" style={{ maxHeight: 200 }}>{JSON.stringify(c.context, null, 2)}</pre>
            )}
            <div className="row" style={{ marginTop: 12 }}>
              <button className="primary" disabled={busy === c.id} onClick={() => void resolve(c.id, 'approve')}>Approve</button>
              <button disabled={busy === c.id} onClick={() => void resolve(c.id, 'modify')}>Modify</button>
              <button className="danger" disabled={busy === c.id} onClick={() => void resolve(c.id, 'reject')}>Reject</button>
              <span className="grow" />
              <span className="mono small muted">{c.id}</span>
            </div>
          </div>
        ))}
      </div>
    </>
  );
}
