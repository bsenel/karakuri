import { NavLink, Outlet } from 'react-router-dom';
import { useAuth } from '@/auth/AuthProvider';

export function Layout() {
  const { health } = useAuth();
  return (
    <div className="layout">
      <nav className="topnav">
        <span className="brand">⌬ Karakuri</span>
        <NavLink to="/twins"       className={({ isActive }) => (isActive ? 'active' : '')}>Twins</NavLink>
        <NavLink to="/objectives"  className={({ isActive }) => (isActive ? 'active' : '')}>Objectives</NavLink>
        <NavLink to="/checkpoints" className={({ isActive }) => (isActive ? 'active' : '')}>Checkpoints</NavLink>
        <NavLink to="/memory"      className={({ isActive }) => (isActive ? 'active' : '')}>Memory</NavLink>
        <NavLink to="/artifacts"   className={({ isActive }) => (isActive ? 'active' : '')}>Artifacts</NavLink>
        <NavLink to="/health"      className={({ isActive }) => (isActive ? 'active' : '')}>Health</NavLink>
        <div className="grow" />
        {health && (
          <span className="small muted">
            {Object.values(health.providers).filter(Boolean).length} providers ·{' '}
            {health.adapters.filter((a) => a.active).length} adapters
          </span>
        )}
      </nav>
      <main className="main">
        <Outlet />
      </main>
    </div>
  );
}
