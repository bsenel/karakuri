import { Navigate, Route, Routes } from 'react-router-dom';
import { AuthProvider, useAuth } from '@/auth/AuthProvider';
import { LoginModal } from '@/auth/LoginModal';
import { Layout } from '@/components/Layout';
import { TwinsPage }         from '@/pages/TwinsPage';
import { TwinDetailPage }    from '@/pages/TwinDetailPage';
import { ObjectivesPage }    from '@/pages/ObjectivesPage';
import { ObjectiveDetailPage } from '@/pages/ObjectiveDetailPage';
import { CheckpointsPage }   from '@/pages/CheckpointsPage';
import { AuditPage }         from '@/pages/AuditPage';
import { MemoryPage }        from '@/pages/MemoryPage';
import { ArtifactsPage }     from '@/pages/ArtifactsPage';
import { HealthPage }        from '@/pages/HealthPage';

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
  const { ready, identity } = useAuth();
  if (!ready) return <div style={{ padding: 24 }} className="muted">Loading…</div>;
  if (!identity) return <LoginModal />;
  return (
    <Routes>
      <Route path="/" element={<Layout />}>
        <Route index element={<Navigate to="/objectives" replace />} />
        <Route path="twins" element={<TwinsPage />} />
        <Route path="twins/:id" element={<TwinDetailPage />} />
        <Route path="objectives" element={<ObjectivesPage />} />
        <Route path="objectives/:id" element={<ObjectiveDetailPage />} />
        <Route path="checkpoints" element={<CheckpointsPage />} />
        <Route path="audit" element={<AuditPage />} />
        <Route path="memory" element={<MemoryPage />} />
        <Route path="artifacts" element={<ArtifactsPage />} />
        <Route path="health" element={<HealthPage />} />
      </Route>
    </Routes>
  );
}
