import { useEffect, useState } from 'react';
import { api } from '@/api/client';
import { describe } from '@/api/useApi';
import type { HealthResponse } from '@/api/types';

export function HealthPage() {
  const [h, setH] = useState<HealthResponse | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    const load = async () => {
      try {
        setH(await api.get<HealthResponse>('/health'));
        setErr(null);
      } catch (e) { setErr(describe(e)); }
    };
    void load();
    const id = setInterval(load, 5000);
    return () => clearInterval(id);
  }, []);

  if (err) {
    return (
      <>
        <h1>Health</h1>
        <p><span className="pill red">{err}</span></p>
        <p className="muted">
          Could not read the server's health. Trying again every 5 seconds.
        </p>
      </>
    );
  }
  if (!h) {
    return (
      <>
        <h1>Health</h1>
        <p className="muted">Loading…</p>
      </>
    );
  }

  const bySlot = new Map<string, typeof h.adapters>();
  for (const a of h.adapters ?? []) {
    const cur = bySlot.get(a.slot) ?? [];
    cur.push(a);
    bySlot.set(a.slot, cur);
  }

  return (
    <>
      <h1>Health</h1>

      <h2>Providers</h2>
      <div className="row" style={{ flexWrap: 'wrap' }}>
        {Object.entries(h.providers ?? {}).map(([name, ok]) => (
          <span key={name} className={`pill ${ok ? 'green' : ''}`}>{name}: {ok ? 'up' : 'down'}</span>
        ))}
      </div>
      {/* A bare heading over nothing reads as a page still loading. */}
      {Object.keys(h.providers ?? {}).length === 0 && (
        <p className="muted">No providers are configured.</p>
      )}

      <h2>Adapters</h2>
      {bySlot.size === 0 && <p className="muted">No adapters are configured.</p>}
      {Array.from(bySlot.entries()).map(([slot, list]) => (
        <div key={slot} className="card" style={{ marginBottom: 12 }}>
          <h3>{slot}</h3>
          <table>
            <thead><tr><th>Instance</th><th>Type</th><th>Active</th><th>Default</th></tr></thead>
            <tbody>
              {list.map((a) => (
                <tr key={a.instance}>
                  <td className="mono small">{a.instance}</td>
                  <td>{a.type}</td>
                  <td>{a.active ? <span className="pill green">yes</span> : <span className="pill">no</span>}</td>
                  <td>{a.is_default ? '★' : ''}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ))}

      <h2>Git + Exporters</h2>
      <pre className="payload">{JSON.stringify({ git: h.git, exporters: h.exporters }, null, 2)}</pre>
    </>
  );
}
