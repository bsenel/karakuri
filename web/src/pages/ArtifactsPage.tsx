import { useEffect, useState } from 'react';
import { api } from '@/api/client';
import type { Artifact } from '@/api/types';

export function ArtifactsPage() {
  const [items, setItems] = useState<Artifact[]>([]);
  const [filter, setFilter] = useState('');
  const [err, setErr] = useState<string | null>(null);
  // null until the first answer, so "not loaded" never reads as "none exist".
  const [loaded, setLoaded] = useState<'ok' | 'failed' | null>(null);

  // Diff state
  const [sha1, setSha1] = useState('');
  const [sha2, setSha2] = useState('');
  const [diff, setDiff] = useState<string | null>(null);

  const load = async (objectiveID?: string) => {
    try {
      const q = objectiveID ? `?objective=${encodeURIComponent(objectiveID)}` : '';
      const list = await api.get<Artifact[]>(`/artifacts${q}`);
      setItems(list ?? []);
      setErr(null);
      setLoaded('ok');
    } catch (e) {
      setItems([]);
      setErr(String(e));
      setLoaded('failed');
    }
  };

  useEffect(() => { void load(); }, []);

  const runDiff = async () => {
    if (!sha1 || !sha2) return;
    try {
      const text = await api.get<string>(`/artifacts/${sha1}/diff/${sha2}`);
      setDiff(typeof text === 'string' ? text : JSON.stringify(text, null, 2));
    } catch (e) { setErr(String(e)); }
  };

  return (
    <>
      <h1>Artifacts</h1>

      <div className="card" style={{ marginBottom: 16 }}>
        <h3>Filter</h3>
        <div className="row">
          <div className="grow">
            <label htmlFor="artifact-objective">Objective ID</label>
            <input id="artifact-objective" value={filter} onChange={(e) => setFilter(e.target.value)} placeholder="(all)" />
          </div>
          <button onClick={() => void load(filter || undefined)}>Apply</button>
        </div>
      </div>

      {err && <p className="pill red" role="alert">{err}</p>}

      <table>
        <thead><tr><th>SHA</th><th>Objective</th><th>Agent</th><th>Kind</th><th>Size</th><th>Created</th></tr></thead>
        <tbody>
          {items.map((a) => (
            <tr key={a.sha}>
              <td className="mono small">
                <code>{a.sha.slice(0, 16)}…</code>{' '}
                {/* "A" alone names nothing out of context; the name says which
                    artifact goes into which side of the diff below. */}
                <button onClick={() => setSha1(a.sha)} className="small" aria-label={`Use ${a.sha.slice(0, 16)} as SHA A`}>A</button>{' '}
                <button onClick={() => setSha2(a.sha)} className="small" aria-label={`Use ${a.sha.slice(0, 16)} as SHA B`}>B</button>
              </td>
              <td className="mono small">{a.objective_id}</td>
              <td className="mono small">{a.agent_id}</td>
              <td>{a.kind ?? '—'}</td>
              <td className="mono small">{a.size ?? '—'}</td>
              <td className="muted small">{new Date(a.created_at).toLocaleString()}</td>
            </tr>
          ))}
          {loaded === null && (
            <tr><td colSpan={6} className="muted" role="status">Loading…</td></tr>
          )}
          {loaded === 'failed' && (
            <tr><td colSpan={6} className="muted">Could not load artifacts.</td></tr>
          )}
          {loaded === 'ok' && items.length === 0 && (
            <tr><td colSpan={6} className="muted">No artifacts yet. Agents store them as objectives run.</td></tr>
          )}
        </tbody>
      </table>

      <h2>Diff</h2>
      <div className="card">
        <div className="row">
          <div className="grow">
            <label htmlFor="artifact-sha-a">SHA A</label>
            <input id="artifact-sha-a" value={sha1} onChange={(e) => setSha1(e.target.value)} className="mono" />
          </div>
          <div className="grow">
            <label htmlFor="artifact-sha-b">SHA B</label>
            <input id="artifact-sha-b" value={sha2} onChange={(e) => setSha2(e.target.value)} className="mono" />
          </div>
          <button className="primary" onClick={() => void runDiff()} disabled={!sha1 || !sha2}>Diff</button>
        </div>
        {diff && <pre className="payload" style={{ marginTop: 12, maxHeight: 480 }}>{diff}</pre>}
      </div>
    </>
  );
}
