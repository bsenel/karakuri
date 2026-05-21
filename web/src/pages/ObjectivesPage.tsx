import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '@/api/client';
import type { Objective, ObjectiveTemplate, Twin } from '@/api/types';

export function ObjectivesPage() {
  const [items, setItems] = useState<Objective[]>([]);
  const [templates, setTemplates] = useState<ObjectiveTemplate[]>([]);
  const [twins, setTwins] = useState<Twin[]>([]);
  const [err, setErr] = useState<string | null>(null);

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
    } catch (e) {
      setErr(String(e));
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
      setErr(String(e));
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
              <label>Title</label>
              <input value={title} onChange={(e) => setTitle(e.target.value)} required />
            </div>
            <div>
              <label>Max iterations</label>
              <input type="number" min={1} value={maxIter} onChange={(e) => setMaxIter(Number(e.target.value))} />
            </div>
          </div>
          <div className="row">
            <div className="grow">
              <label>Twin</label>
              <select value={twinID} onChange={(e) => setTwinID(e.target.value)}>
                <option value="">(none)</option>
                {twins.map((t) => <option key={t.id} value={t.id}>{t.name} ({t.kind})</option>)}
              </select>
            </div>
            <div className="grow">
              <label>Domain</label>
              <input value={domain} onChange={(e) => setDomain(e.target.value)} />
            </div>
            <div className="grow">
              <label>Template</label>
              <select value={templateID} onChange={(e) => setTemplateID(e.target.value)}>
                <option value="">(none)</option>
                {templates.map((t) => <option key={t.id} value={t.id}>{t.title}</option>)}
              </select>
            </div>
            <button className="primary" type="submit">Create</button>
          </div>
        </form>
      </div>

      {err && <p className="pill red">{err}</p>}
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
          {items.length === 0 && (
            <tr><td colSpan={5} className="muted">No objectives yet.</td></tr>
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
