import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '@/api/client';
import type { Twin } from '@/api/types';

export function TwinsPage() {
  const [twins, setTwins] = useState<Twin[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);

  // Create form state
  const [name, setName] = useState('');
  const [kind, setKind] = useState<'person' | 'team' | 'organization'>('team');
  const [domain, setDomain] = useState('software');

  const load = async () => {
    setLoading(true);
    try {
      const list = await api.get<Twin[]>('/twins');
      setTwins(list ?? []);
      setErr(null);
    } catch (e) {
      setErr(String(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(); }, []);

  const create = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.post<Twin>('/twins/', { name, kind, domain });
      setName('');
      await load();
    } catch (e) {
      setErr(String(e));
    }
  };

  return (
    <>
      <h1>Twins</h1>

      <div className="card" style={{ marginBottom: 16 }}>
        <h3>Create</h3>
        <form onSubmit={create} className="row" style={{ alignItems: 'flex-end' }}>
          {/* htmlFor/id rather than a bare <label>: without the association a
              screen reader reads the field as unlabelled, and so does anything
              else driving the page by accessible name. */}
          <div className="grow">
            <label htmlFor="twin-name">Name</label>
            <input
              id="twin-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
            />
          </div>
          <div>
            <label htmlFor="twin-kind">Kind</label>
            <select
              id="twin-kind"
              value={kind}
              onChange={(e) => setKind(e.target.value as typeof kind)}
            >
              <option value="person">person</option>
              <option value="team">team</option>
              <option value="organization">organization</option>
            </select>
          </div>
          <div>
            <label htmlFor="twin-domain">Domain</label>
            <input id="twin-domain" value={domain} onChange={(e) => setDomain(e.target.value)} />
          </div>
          <button className="primary" type="submit">Create</button>
        </form>
      </div>

      {err && <p className="pill red">{err}</p>}
      {loading ? <p className="muted">Loading…</p> : (
        <table>
          <thead>
            <tr><th>Name</th><th>Kind</th><th>Domain</th><th>Bindings</th><th>Created</th></tr>
          </thead>
          <tbody>
            {twins.map((t) => (
              <tr key={t.id}>
                <td><Link to={`/twins/${t.id}`}>{t.name}</Link></td>
                <td><span className="pill">{t.kind}</span></td>
                <td>{t.domain}</td>
                <td className="mono small">
                  {Object.entries(t.adapter_bindings ?? {}).length === 0
                    ? <span className="muted">—</span>
                    : Object.entries(t.adapter_bindings ?? {}).map(([k, v]) => (
                        <div key={k}>{k}={v}</div>
                      ))}
                </td>
                <td className="muted small">{new Date(t.created_at).toLocaleString()}</td>
              </tr>
            ))}
            {twins.length === 0 && (
              <tr><td colSpan={5} className="muted">No twins yet — create one above.</td></tr>
            )}
          </tbody>
        </table>
      )}
    </>
  );
}
