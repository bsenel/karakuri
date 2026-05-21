import { useState } from 'react';
import { api } from '@/api/client';
import type { MemoryEntry, MemoryQuery } from '@/api/types';

const TIERS = ['working', 'episodic', 'semantic', 'procedural'] as const;

export function MemoryPage() {
  const [agent, setAgent] = useState('');
  const [twin, setTwin] = useState('');
  const [text, setText] = useState('');
  const [topK, setTopK] = useState(10);
  const [selected, setSelected] = useState<string[]>(['episodic', 'semantic']);
  const [results, setResults] = useState<MemoryEntry[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    try {
      const q: MemoryQuery = {
        agent_id: agent || undefined,
        twin_id: twin || undefined,
        query: text || undefined,
        tiers: selected,
        top_k: topK,
      };
      const r = await api.post<MemoryEntry[]>('/memory/recall', q);
      setResults(r ?? []);
      setErr(null);
    } catch (e) { setErr(String(e)); }
    finally { setLoading(false); }
  };

  return (
    <>
      <h1>Memory recall</h1>
      <form onSubmit={submit} className="card col">
        <div className="row">
          <div className="grow">
            <label>Agent ID</label>
            <input value={agent} onChange={(e) => setAgent(e.target.value)} placeholder="(any)" />
          </div>
          <div className="grow">
            <label>Twin ID</label>
            <input value={twin} onChange={(e) => setTwin(e.target.value)} placeholder="(any)" />
          </div>
          <div>
            <label>Top K</label>
            <input type="number" min={1} value={topK} onChange={(e) => setTopK(Number(e.target.value))} />
          </div>
        </div>
        <div>
          <label>Query</label>
          <input value={text} onChange={(e) => setText(e.target.value)} placeholder="search text…" />
        </div>
        <div>
          <label>Tiers</label>
          <div className="row">
            {TIERS.map((t) => (
              <label key={t} className="row small" style={{ display: 'inline-flex', margin: 0 }}>
                <input
                  type="checkbox"
                  style={{ width: 'auto' }}
                  checked={selected.includes(t)}
                  onChange={(e) =>
                    setSelected(e.target.checked ? [...selected, t] : selected.filter((x) => x !== t))
                  }
                />
                <span style={{ marginLeft: 4 }}>{t}</span>
              </label>
            ))}
          </div>
        </div>
        <button className="primary" type="submit" disabled={loading}>Recall</button>
      </form>

      {err && <p className="pill red">{err}</p>}

      <h2>Results ({results.length})</h2>
      <table>
        <thead><tr><th>Tier</th><th>Agent</th><th>Twin</th><th>Content</th><th>Confidence</th><th>When</th></tr></thead>
        <tbody>
          {results.map((r) => (
            <tr key={r.id}>
              <td><span className="pill accent">{r.tier}</span></td>
              <td className="mono small">{r.agent_id}</td>
              <td className="mono small">{r.twin_id ?? '—'}</td>
              <td style={{ maxWidth: 480 }}>{r.content}</td>
              <td className="mono small">{r.confidence?.toFixed?.(2) ?? '—'}</td>
              <td className="muted small">{new Date(r.created_at).toLocaleString()}</td>
            </tr>
          ))}
          {results.length === 0 && (
            <tr><td colSpan={6} className="muted">No results yet — run a recall query.</td></tr>
          )}
        </tbody>
      </table>
    </>
  );
}
