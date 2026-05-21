import { Navigate, Route, Routes } from 'react-router-dom';
import { AuthProvider, useAuth } from '@/auth/AuthProvider';
import { LoginModal } from '@/auth/LoginModal';
import { Layout } from '@/components/Layout';
import { TwinsPage }         from '@/pages/TwinsPage';
import { TwinDetailPage }    from '@/pages/TwinDetailPage';
import { ObjectivesPage }    from '@/pages/ObjectivesPage';
import { ObjectiveDetailPage } from '@/pages/ObjectiveDetailPage';
import { CheckpointsPage }   from '@/pages/CheckpointsPage';
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

// Shell decides whether to mount the routed app or block on a login modal.
// Auth requirement is inferred from /health: 401 → login required.
function Shell() {
  const { ready, error } = useAuth();
  if (!ready) return <div style={{ padding: 24 }} className="muted">Loading…</div>;
  if (error && /401|unauthorized/i.test(error)) return <LoginModal />;
  return (
    <Routes>
      <Route path="/" element={<Layout />}>
        <Route index element={<Navigate to="/objectives" replace />} />
        <Route path="twins" element={<TwinsPage />} />
        <Route path="twins/:id" element={<TwinDetailPage />} />
        <Route path="objectives" element={<ObjectivesPage />} />
        <Route path="objectives/:id" element={<ObjectiveDetailPage />} />
        <Route path="checkpoints" element={<CheckpointsPage />} />
        <Route path="memory" element={<MemoryPage />} />
        <Route path="artifacts" element={<ArtifactsPage />} />
        <Route path="health" element={<HealthPage />} />
      </Route>
    </Routes>
  );
}
