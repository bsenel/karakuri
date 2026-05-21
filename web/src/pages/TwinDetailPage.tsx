import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { api } from '@/api/client';
import type { HealthAdapter, HealthResponse, Twin } from '@/api/types';

// Drill-down for one twin. Shows core fields, child twins, and an inline
// editor for adapter bindings (slot → instance) that PUTs to /twins/:id/bindings.
export function TwinDetailPage() {
  const { id = '' } = useParams();
  const [twin, setTwin] = useState<Twin | null>(null);
  const [adapters, setAdapters] = useState<HealthAdapter[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [savingBindings, setSavingBindings] = useState(false);
  const [draftBindings, setDraftBindings] = useState<Record<string, string>>({});

  const load = async () => {
    try {
      const [t, h] = await Promise.all([
        api.get<Twin>(`/twins/${id}`),
        api.get<HealthResponse>('/health'),
      ]);
      setTwin(t);
      setAdapters(h.adapters);
      setDraftBindings(t.adapter_bindings ?? {});
      setErr(null);
    } catch (e) {
      setErr(String(e));
    }
  };

  useEffect(() => { void load(); /* eslint-disable-next-line react-hooks/exhaustive-deps */ }, [id]);

  const slots = Array.from(new Set(adapters.map((a) => a.slot))).sort();

  const saveBindings = async () => {
    setSavingBindings(true);
    try {
      await api.put<Twin>(`/twins/${id}/bindings`, { adapter_bindings: draftBindings });
      await load();
    } catch (e) {
      setErr(String(e));
    } finally {
      setSavingBindings(false);
    }
  };

  if (err) return <p className="pill red">{err}</p>;
  if (!twin) return <p className="muted">Loading…</p>;

  return (
    <>
      <p className="muted small"><Link to="/twins">← Twins</Link></p>
      <h1>{twin.name}</h1>
      <div className="row" style={{ marginBottom: 16 }}>
        <span className="pill">{twin.kind}</span>
        <span className="pill accent">{twin.domain}</span>
        <span className="muted small mono">id: {twin.id}</span>
      </div>

      <div className="card">
        <h3>Adapter Bindings</h3>
        <p className="muted small">Map slot → instance. Empty means fall back to the slot's default.</p>
        <table style={{ marginTop: 12 }}>
          <thead>
            <tr><th>Slot</th><th>Instance</th><th>Active types</th></tr>
          </thead>
          <tbody>
            {slots.map((slot) => {
              const slotAdapters = adapters.filter((a) => a.slot === slot);
              return (
                <tr key={slot}>
                  <td className="mono small">{slot}</td>
                  <td>
                    <select
                      value={draftBindings[slot] ?? ''}
                      onChange={(e) => setDraftBindings({ ...draftBindings, [slot]: e.target.value })}
                    >
                      <option value="">(default)</option>
                      {slotAdapters.map((a) => (
                        <option key={a.instance} value={a.instance}>{a.instance} ({a.type})</option>
                      ))}
                    </select>
                  </td>
                  <td className="small muted">{slotAdapters.map((a) => a.type).join(', ')}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
        <div className="row" style={{ marginTop: 12 }}>
          <button className="primary" onClick={() => void saveBindings()} disabled={savingBindings}>
            {savingBindings ? 'Saving…' : 'Save bindings'}
          </button>
        </div>
      </div>

      {twin.children && twin.children.length > 0 && (
        <div className="card" style={{ marginTop: 16 }}>
          <h3>Child twins</h3>
          <ul>
            {twin.children.map((c) => <li key={c}><Link to={`/twins/${c}`}>{c}</Link></li>)}
          </ul>
        </div>
      )}
    </>
  );
}
