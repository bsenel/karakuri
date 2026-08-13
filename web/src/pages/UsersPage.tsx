import { useState } from 'react';
import { api } from '@/api/client';
import { describe, useFetch } from '@/api/useApi';
import { useAuth } from '@/auth/AuthProvider';
import { Actions } from '@/auth/permissions';
import { DataTable } from '@/components/DataTable';
import { RequirePermission } from '@/components/RequirePermission';

interface Principal {
  id: string;
  name?: string;
  kind?: string;
}

interface Binding {
  id: string;
  principal_id: string;
  role: string;
  scope: string;
}

interface Role {
  name: string;
  description?: string;
}

interface Container {
  id: string;
  kind: string;
  name: string;
  parent_id?: string;
}

export function UsersPage() {
  return (
    <RequirePermission action={Actions.authRead} what="users and their roles">
      <Users />
    </RequirePermission>
  );
}

function Users() {
  const { can, identity } = useAuth();
  const users = useFetch<Principal[]>('/auth/users');
  const bindings = useFetch<Binding[]>('/auth/bindings');
  const roles = useFetch<Role[]>('/auth/roles');
  const containers = useFetch<Container[]>(can(Actions.containerRead) ? '/containers' : null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const mayWrite = can(Actions.authWrite);
  const byPrincipal = new Map<string, Binding[]>();
  for (const b of bindings.data ?? []) {
    byPrincipal.set(b.principal_id, [...(byPrincipal.get(b.principal_id) ?? []), b]);
  }

  const reload = () => {
    users.reload();
    bindings.reload();
  };

  const removeBinding = async (id: string) => {
    setBusy(id);
    setError(null);
    try {
      await api.del(`/auth/bindings/${encodeURIComponent(id)}`);
      reload();
    } catch (e) {
      setError(describe(e));
    } finally {
      setBusy(null);
    }
  };

  const removeUser = async (id: string) => {
    // Deleting an identity takes its access with it, and there is no undo.
    if (!window.confirm(`Delete ${id}? Their bindings go too, and this cannot be undone.`)) return;
    setBusy(id);
    setError(null);
    try {
      await api.del(`/auth/users/${encodeURIComponent(id)}`);
      reload();
    } catch (e) {
      setError(describe(e));
    } finally {
      setBusy(null);
    }
  };

  return (
    <>
      <h1>Users</h1>
      <p className="muted small">
        Who exists, and what each of them may do. A binding is a role over a scope — the
        scope is what confines an operator to one organisation rather than all of them.
      </p>

      {error && <p className="error">{error}</p>}

      {mayWrite && (
        <NewUserForm
          roles={roles.data ?? []}
          containers={containers.data ?? []}
          onCreated={reload}
          onError={setError}
        />
      )}

      <DataTable
        loading={users.loading}
        error={users.error}
        rows={users.data ?? []}
        keyOf={(p) => p.id}
        empty="No principals yet."
        columns={[
          {
            header: 'Principal',
            render: (p) => (
              <>
                <span className="mono">{p.id}</span>
                {p.name && p.name !== p.id && <span className="muted small"> · {p.name}</span>}
                {identity?.principal.id === p.id && <span className="pill accent">you</span>}
              </>
            ),
          },
          {
            header: 'Roles and scopes',
            render: (p) => {
              const held = byPrincipal.get(p.id) ?? [];
              if (held.length === 0) {
                return (
                  <span className="muted small">
                    none — this principal can sign in and see nothing
                  </span>
                );
              }
              return (
                <div className="row" style={{ flexWrap: 'wrap' }}>
                  {held.map((b) => (
                    <span key={b.id} className="pill" title={b.id}>
                      {b.role}
                      {b.scope !== '*' && <span className="muted"> @ {b.scope}</span>}
                      {mayWrite && (
                        <button
                          className="linklike small"
                          disabled={busy === b.id}
                          onClick={() => void removeBinding(b.id)}
                          title="Revoke this binding"
                        >
                          ×
                        </button>
                      )}
                    </span>
                  ))}
                </div>
              );
            },
          },
          ...(mayWrite
            ? [
                {
                  header: '',
                  render: (p: Principal) => (
                    <button
                      className="linklike small"
                      disabled={busy === p.id || identity?.principal.id === p.id}
                      title={
                        identity?.principal.id === p.id
                          ? 'You cannot delete the account you are signed in as'
                          : 'Delete this principal'
                      }
                      onClick={() => void removeUser(p.id)}
                    >
                      delete
                    </button>
                  ),
                },
              ]
            : []),
        ]}
      />
    </>
  );
}

function NewUserForm({
  roles,
  containers,
  onCreated,
  onError,
}: {
  roles: Role[];
  containers: Container[];
  onCreated: () => void;
  onError: (message: string | null) => void;
}) {
  const [open, setOpen] = useState(false);
  const [id, setId] = useState('');
  const [role, setRole] = useState('viewer');
  const [scope, setScope] = useState('*');
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);

  if (!open) {
    return (
      <div className="row" style={{ margin: '16px 0' }}>
        <button className="primary" onClick={() => setOpen(true)}>
          Add a user
        </button>
      </div>
    );
  }

  const submit = async () => {
    setBusy(true);
    onError(null);
    try {
      await api.post('/auth/users', {
        id,
        name: id,
        roles: [role],
        scope,
        password,
      });
      setOpen(false);
      setId('');
      setPassword('');
      onCreated();
    } catch (e) {
      onError(describe(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="card" style={{ margin: '16px 0' }}>
      <h3>New user</h3>
      <div className="row" style={{ flexWrap: 'wrap' }}>
        <input
          placeholder="id, e.g. alice"
          value={id}
          onChange={(e) => setId(e.target.value)}
          aria-label="Principal ID"
        />
        <select value={role} onChange={(e) => setRole(e.target.value)} aria-label="Role">
          {roles.map((r) => (
            <option key={r.name} value={r.name}>
              {r.name}
            </option>
          ))}
        </select>
        <select value={scope} onChange={(e) => setScope(e.target.value)} aria-label="Scope">
          {/* The scope is the whole point of the form. "*" is everything; a
              container confines the role to that org, team or project and
              everything inside it. */}
          <option value="*">everywhere</option>
          {containers.map((c) => (
            <option key={c.id} value={`${c.kind}:${c.id}`}>
              {c.kind}: {c.name}
            </option>
          ))}
        </select>
        <input
          type="password"
          placeholder="initial password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          aria-label="Initial password"
        />
        <button className="primary" disabled={busy || !id || !password} onClick={() => void submit()}>
          Create
        </button>
        <button onClick={() => setOpen(false)}>Cancel</button>
      </div>
      <p className="muted small" style={{ marginTop: 8 }}>
        You can only grant a scope you hold yourself — the server refuses anything wider,
        which is what stops an administrator of one organisation writing themselves into
        another.
      </p>
    </div>
  );
}
