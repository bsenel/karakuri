import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '@/api/client';
import { describe } from '@/api/useApi';
import type { Objective, ObjectiveTemplate, Twin } from '@/api/types';

export function ObjectivesPage() {
  const [items, setItems] = useState<Objective[]>([]);
  const [templates, setTemplates] = useState<ObjectiveTemplate[]>([]);
  const [twins, setTwins] = useState<Twin[]>([]);
  const [err, setErr] = useState<string | null>(null);
  // Whether the list has been read at least once. `err` alone cannot say:
  // a failed create sets it too, and the list is still known then.
  const [loaded, setLoaded] = useState(false);

  // Create form
  const [title, setTitle] = useState('');
  const [twinID, setTwinID] = useState('');
  const [domain, setDomain] = useState('software');
  const [templateID, setTemplateID] = useState('');
  const [maxIter, setMaxIter] = useState(20);

  const load = async () => {
    try {
      const [obj, tpl, tw] = await Promise.all([
        api.get<Objective[]>('/objectives'),
        api.get<ObjectiveTemplate[]>('/objectives/templates'),
        api.get<Twin[]>('/twins'),
      ]);
      setItems(obj ?? []);
      setTemplates(tpl ?? []);
      setTwins(tw ?? []);
      setErr(null);
      setLoaded(true);
    } catch (e) {
      setErr(describe(e));
    }
  };

  useEffect(() => { void load(); }, []);

  const create = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.post<Objective>('/objectives/', {
        title,
        domain,
        twin_id: twinID || undefined,
        template_id: templateID || undefined,
        max_iterations: maxIter,
      });
      setTitle('');
      await load();
    } catch (e) {
      setErr(describe(e));
    }
  };

  return (
    <>
      <h1>Objectives</h1>

      <div className="card" style={{ marginBottom: 16 }}>
        <h3>Create</h3>
        <form onSubmit={create} className="col">
          <div className="row">
            <div className="grow">
              <label htmlFor="objective-title">Title</label>
              <input id="objective-title" value={title} onChange={(e) => setTitle(e.target.value)} required />
            </div>
            <div>
              <label htmlFor="objective-max-iterations">Max iterations</label>
              <input id="objective-max-iterations" type="number" min={1} value={maxIter} onChange={(e) => setMaxIter(Number(e.target.value))} />
            </div>
          </div>
          <div className="row">
            <div className="grow">
              <label htmlFor="objective-twin">Twin</label>
              <select id="objective-twin" value={twinID} onChange={(e) => setTwinID(e.target.value)}>
                <option value="">(none)</option>
                {twins.map((t) => <option key={t.id} value={t.id}>{t.name} ({t.kind})</option>)}
              </select>
            </div>
            <div className="grow">
              <label htmlFor="objective-domain">Domain</label>
              <input id="objective-domain" value={domain} onChange={(e) => setDomain(e.target.value)} />
            </div>
            <div className="grow">
              <label htmlFor="objective-template">Template</label>
              <select id="objective-template" value={templateID} onChange={(e) => setTemplateID(e.target.value)}>
                <option value="">(none)</option>
                {templates.map((t) => <option key={t.id} value={t.id}>{t.title}</option>)}
              </select>
            </div>
            <button className="primary" type="submit">Create</button>
          </div>
        </form>
      </div>

      {err && (
        <p>
          <span className="pill red">{err}</span>{' '}
          {!loaded && <button onClick={() => void load()}>Retry</button>}
        </p>
      )}
      <table>
        <thead><tr><th>Title</th><th>Domain</th><th>Status</th><th>Twin</th><th>Created</th></tr></thead>
        <tbody>
          {items.map((o) => (
            <tr key={o.id}>
              <td><Link to={`/objectives/${o.id}`}>{o.title}</Link></td>
              <td>{o.domain}</td>
              <td><StatusPill status={o.status} /></td>
              <td className="mono small">{o.twin_id ?? '—'}</td>
              <td className="muted small">{new Date(o.created_at).toLocaleString()}</td>
            </tr>
          ))}
          {/* Neither "not loaded yet" nor "could not load" is "no objectives". */}
          {!loaded && !err && (
            <tr><td colSpan={5} className="muted">Loading…</td></tr>
          )}
          {loaded && items.length === 0 && (
            <tr>
              <td colSpan={5} className="muted">
                No objectives yet. Create one above, or run <code>krk objective create</code>.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </>
  );
}

function StatusPill({ status }: { status: string }) {
  const color =
    status === 'completed' ? 'green' :
    status === 'failed' || status === 'cancelled' ? 'red' :
    status === 'active' ? 'amber' : '';
  return <span className={`pill ${color}`}>{status}</span>;
}
