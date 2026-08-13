import { useMemo } from 'react';
import { useFetch } from '@/api/useApi';
import { Actions } from '@/auth/permissions';
import { RequirePermission } from '@/components/RequirePermission';

interface Policy {
  id?: string;
  effect?: string;
  actions?: string[];
  resources?: string[];
}

interface Role {
  name: string;
  description?: string;
  inherits?: string[];
  policies?: Policy[];
}

interface CatalogEntry {
  action: string;
  description: string;
}

export function RolesPage() {
  return (
    <RequirePermission action={Actions.authRead} what="the role catalogue">
      <Roles />
    </RequirePermission>
  );
}

/**
 * The role → permission matrix.
 *
 * `krk auth roles` prints the same data as JSON, and reading a 15×8 grid out of
 * nested JSON is exactly the thing a table is better at: the question people
 * actually ask is "which roles can approve a quota request", and that is a
 * column, not a document.
 */
function Roles() {
  const roles = useFetch<Role[]>('/auth/roles');
  const catalog = useFetch<CatalogEntry[]>('/auth/catalog');

  const { actions, grants } = useMemo(() => {
    const roleList = roles.data ?? [];
    const grants = new Map<string, Set<string>>();
    const seen = new Set<string>();

    for (const role of roleList) {
      const held = new Set<string>();
      for (const p of role.policies ?? []) {
        // Deny policies exist and are not rendered as grants. A role that both
        // allows and denies an action does not hold it, and showing a tick
        // would be worse than showing nothing.
        if (p.effect && p.effect !== 'allow') continue;
        for (const action of p.actions ?? []) {
          held.add(action);
          // "*" is a wildcard, not an action, so it is not a row. Listing it
          // would put a line labelled "*" in a table of permissions and tell a
          // reader nothing they could act on.
          if (action !== '*') seen.add(action);
        }
      }
      grants.set(role.name, held);
    }

    // The catalogue is the full list of actions the server knows, which is
    // wider than what any role happens to grant — an action nobody holds is
    // worth seeing as an empty column rather than not at all.
    for (const entry of catalog.data ?? []) seen.add(entry.action);

    return { actions: [...seen].sort(), grants };
  }, [roles.data, catalog.data]);

  const descriptions = new Map((catalog.data ?? []).map((c) => [c.action, c.description]));
  const roleList = roles.data ?? [];

  if (roles.error) return <p className="error">{roles.error}</p>;
  if (roles.loading) return <p className="muted">Loading…</p>;

  return (
    <>
      <h1>Roles</h1>
      <p className="muted small">
        What each role grants. A wildcard in the matrix means the role holds the action
        over every resource; the <em>scope</em> on somebody's binding is what narrows that
        to one organisation, and it is per person rather than per role.
      </p>

      <div className="tablewrap">
        <table>
          <thead>
            <tr>
              <th>Action</th>
              {roleList.map((r) => (
                <th key={r.name} title={r.description}>
                  {r.name}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {actions.map((action) => (
              <tr key={action}>
                <td>
                  <span className="mono">{action}</span>
                  {descriptions.get(action) && (
                    <div className="muted small">{descriptions.get(action)}</div>
                  )}
                </td>
                {roleList.map((r) => {
                  const held = grants.get(r.name);
                  // "*" is the wildcard the admin role holds, and it covers
                  // every action including ones added later.
                  const yes = held?.has(action) || held?.has('*');
                  return (
                    <td key={r.name} style={{ textAlign: 'center' }}>
                      {yes ? <span className="pill green">✓</span> : <span className="muted">·</span>}
                    </td>
                  );
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {roleList.some((r) => (r.inherits ?? []).length > 0) && (
        <>
          <h2>Inheritance</h2>
          <p className="muted small">
            An inherited role's permissions are already counted in the matrix above; this
            is where they came from.
          </p>
          <ul className="muted small">
            {roleList
              .filter((r) => (r.inherits ?? []).length > 0)
              .map((r) => (
                <li key={r.name}>
                  <span className="mono">{r.name}</span> inherits{' '}
                  {(r.inherits ?? []).join(', ')}
                </li>
              ))}
          </ul>
        </>
      )}
    </>
  );
}
