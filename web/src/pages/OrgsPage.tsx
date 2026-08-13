import { useMemo, useState } from 'react';
import { api } from '@/api/client';
import { describe, useFetch } from '@/api/useApi';
import { useAuth } from '@/auth/AuthProvider';
import { Actions } from '@/auth/permissions';
import { RequirePermission } from '@/components/RequirePermission';

interface Container {
  id: string;
  kind: 'org' | 'team' | 'project' | string;
  name: string;
  parent_id?: string;
}

export function OrgsPage() {
  return (
    <RequirePermission action={Actions.containerRead} what="the tenancy tree">
      <Orgs />
    </RequirePermission>
  );
}

/**
 * The tenancy tree: organisations, the teams inside them, and projects.
 *
 * Projects are rendered apart from the tree rather than inside it, because that
 * is what they are — a project can hold twins from more than one organisation,
 * which is the case a path-shaped hierarchy cannot express and the reason
 * Phase 17 chose scope sets (ADR 010). Drawing one inside a single org would be
 * drawing a lie.
 */
function Orgs() {
  const { can } = useAuth();
  const containers = useFetch<Container[]>('/containers');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const mayWrite = can(Actions.containerWrite);
  const { orgs, teamsByOrg, projects } = useMemo(() => {
    const all = containers.data ?? [];
    const orgs = all.filter((c) => c.kind === 'org').sort(byName);
    const projects = all.filter((c) => c.kind === 'project').sort(byName);
    const teamsByOrg = new Map<string, Container[]>();
    for (const team of all.filter((c) => c.kind === 'team')) {
      const key = team.parent_id ?? '';
      teamsByOrg.set(key, [...(teamsByOrg.get(key) ?? []), team].sort(byName));
    }
    return { orgs, teamsByOrg, projects };
  }, [containers.data]);

  const create = async (kind: string, name: string, parentID: string) => {
    setError(null);
    try {
      await api.post('/containers', { kind, name, parent_id: parentID });
      containers.reload();
    } catch (e) {
      setError(describe(e));
    }
  };

  const remove = async (c: Container) => {
    if (!window.confirm(`Delete ${c.kind} "${c.name}"?`)) return;
    setBusy(c.id);
    setError(null);
    try {
      await api.del(`/containers/${encodeURIComponent(c.id)}`);
      containers.reload();
    } catch (e) {
      // The server refuses a container with anything in it, which is the
      // message worth showing rather than a generic failure.
      setError(describe(e));
    } finally {
      setBusy(null);
    }
  };

  if (containers.error) return <p className="error">{containers.error}</p>;
  if (containers.loading) return <p className="muted">Loading…</p>;

  return (
    <>
      <h1>Organisations</h1>
      <p className="muted small">
        The tenancy tree. A binding scoped to an organisation covers everything inside it,
        and the label a binding stores is the container's ID — so renaming one here
        rewrites no policy.
      </p>

      {error && <p className="error">{error}</p>}

      {mayWrite && <CreateRow kinds={['org']} onCreate={(name) => create('org', name, '')} label="New organisation" />}

      {orgs.length === 0 && <p className="muted">No organisations yet.</p>}

      {orgs.map((org) => (
        <div className="card" key={org.id} style={{ marginTop: 12 }}>
          <div className="row">
            <strong>{org.name}</strong>
            <span className="mono muted small">{org.id}</span>
            <div className="grow" />
            {mayWrite && (
              <button className="linklike small" disabled={busy === org.id} onClick={() => void remove(org)}>
                delete
              </button>
            )}
          </div>

          <ul style={{ margin: '8px 0 0 0' }}>
            {(teamsByOrg.get(org.id) ?? []).map((team) => (
              <li key={team.id} className="row">
                <span>{team.name}</span>
                <span className="mono muted small">{team.id}</span>
                <div className="grow" />
                {mayWrite && (
                  <button
                    className="linklike small"
                    disabled={busy === team.id}
                    onClick={() => void remove(team)}
                  >
                    delete
                  </button>
                )}
              </li>
            ))}
            {(teamsByOrg.get(org.id) ?? []).length === 0 && (
              <li className="muted small">no teams</li>
            )}
          </ul>

          {mayWrite && (
            <CreateRow
              kinds={['team']}
              label="New team"
              onCreate={(name) => create('team', name, org.id)}
            />
          )}
        </div>
      ))}

      <h2>Projects</h2>
      <p className="muted small">
        A project can hold twins from more than one organisation, which is why it sits
        beside the tree rather than inside it.
      </p>
      {mayWrite && <CreateRow kinds={['project']} label="New project" onCreate={(name) => create('project', name, '')} />}
      {projects.length === 0 && <p className="muted">No projects.</p>}
      <ul>
        {projects.map((p) => (
          <li key={p.id} className="row">
            <span>{p.name}</span>
            <span className="mono muted small">{p.id}</span>
            <div className="grow" />
            {mayWrite && (
              <button className="linklike small" disabled={busy === p.id} onClick={() => void remove(p)}>
                delete
              </button>
            )}
          </li>
        ))}
      </ul>
    </>
  );
}

function CreateRow({
  label,
  onCreate,
}: {
  kinds: string[];
  label: string;
  onCreate: (name: string) => void | Promise<void>;
}) {
  const [name, setName] = useState('');
  return (
    <div className="row" style={{ marginTop: 8 }}>
      <input
        placeholder={label}
        value={name}
        onChange={(e) => setName(e.target.value)}
        aria-label={label}
      />
      <button
        disabled={!name.trim()}
        onClick={() => {
          void onCreate(name.trim());
          setName('');
        }}
      >
        Add
      </button>
    </div>
  );
}

function byName(a: Container, b: Container) {
  return a.name.localeCompare(b.name);
}
