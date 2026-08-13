import { Suspense, lazy } from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';
import { AuthProvider, useAuth } from '@/auth/AuthProvider';
import { LoginModal } from '@/auth/LoginModal';
import { Layout } from '@/components/Layout';
import { landingFor } from '@/auth/permissions';
import { TwinsPage }         from '@/pages/TwinsPage';
import { TwinDetailPage }    from '@/pages/TwinDetailPage';
import { ObjectivesPage }    from '@/pages/ObjectivesPage';
import { ObjectiveDetailPage } from '@/pages/ObjectiveDetailPage';
import { CheckpointsPage }   from '@/pages/CheckpointsPage';
import { AuditPage }         from '@/pages/AuditPage';
import { AuditEventPage }    from '@/pages/AuditEventPage';
import { MemoryPage }        from '@/pages/MemoryPage';
import { ArtifactsPage }     from '@/pages/ArtifactsPage';
import { HealthPage }        from '@/pages/HealthPage';
import { UsersPage }         from '@/pages/UsersPage';
import { RolesPage }         from '@/pages/RolesPage';
import { OrgsPage }          from '@/pages/OrgsPage';
import { QuotaPage }         from '@/pages/QuotaPage';
// The cost page is the only thing that pulls in Recharts, which is larger than
// the rest of the application put together. Loading it lazily means everybody
// else pays nothing for a page they may never open.
const CostPage = lazy(() => import('@/pages/CostPage').then((m) => ({ default: m.CostPage })));

export default function App() {
  return (
    <AuthProvider>
      <Shell />
    </AuthProvider>
  );
}

// Shell decides whether to mount the routed app or block on the login form.
// Authentication is never optional now, so the test is simply whether we know
// who the user is.
function Shell() {
  const { ready, identity, can } = useAuth();
  if (!ready) return <div style={{ padding: 24 }} className="muted">Loading…</div>;
  if (!identity) return <LoginModal />;
  return (
    <Routes>
      <Route path="/" element={<Layout />}>
        {/* Where somebody lands depends on what they hold. A fixed default
            sends an auditor with no objective access straight to a 403, which
            is a poor first impression of a system behaving correctly. */}
        <Route index element={<Navigate to={landingFor(can)} replace />} />
        <Route path="twins" element={<TwinsPage />} />
        <Route path="twins/:id" element={<TwinDetailPage />} />
        <Route path="objectives" element={<ObjectivesPage />} />
        <Route path="objectives/:id" element={<ObjectiveDetailPage />} />
        <Route path="checkpoints" element={<CheckpointsPage />} />
        <Route path="audit" element={<AuditPage />} />
        <Route path="audit/:id" element={<AuditEventPage />} />
        <Route path="memory" element={<MemoryPage />} />
        <Route path="artifacts" element={<ArtifactsPage />} />
        <Route path="health" element={<HealthPage />} />
        <Route path="users" element={<UsersPage />} />
        <Route path="roles" element={<RolesPage />} />
        <Route path="orgs" element={<OrgsPage />} />
        {/* Quota has nested tabs, so it claims the subtree rather than one path. */}
        <Route path="quota/*" element={<QuotaPage />} />
        <Route
          path="cost"
          element={
            <Suspense fallback={<p className="muted">Loading…</p>}>
              <CostPage />
            </Suspense>
          }
        />
      </Route>
    </Routes>
  );
}
