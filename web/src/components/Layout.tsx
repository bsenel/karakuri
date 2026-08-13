import { NavLink, Outlet } from 'react-router-dom';
import { useAuth } from '@/auth/AuthProvider';
import { visibleNavigation } from '@/auth/permissions';

export function Layout() {
  const { health, identity, can, logout } = useAuth();
  // The navigation shows what this principal can actually open. Hiding a link
  // is a courtesy rather than a control — the server refuses either way — but a
  // menu full of items that answer 403 teaches people to ignore errors.
  const links = visibleNavigation(can);

  return (
    <div className="layout">
      <nav className="topnav">
        <span className="brand">⌬ Karakuri</span>
        {links.map((entry) => (
          <NavLink
            key={entry.to}
            to={entry.to}
            className={({ isActive }) => (isActive ? 'active' : '')}
          >
            {entry.label}
          </NavLink>
        ))}
        <div className="grow" />
        {health && (
          <span className="small muted">
            {Object.values(health.providers).filter(Boolean).length} providers ·{' '}
            {health.adapters.filter((a) => a.active).length} adapters
          </span>
        )}
        {identity && (
          <span className="small muted" title={identity.roles.join(', ')}>
            {identity.principal.name || identity.principal.id}
          </span>
        )}
        <button className="linklike small" onClick={() => void logout()}>
          Sign out
        </button>
      </nav>
      <main className="main">
        <Outlet />
      </main>
    </div>
  );
}
