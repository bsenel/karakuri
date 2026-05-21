import { createContext, useCallback, useContext, useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { getToken, setToken } from '@/api/client';
import { api } from '@/api/client';
import type { HealthResponse } from '@/api/types';

interface AuthContext {
  token: string;
  setToken: (token: string) => void;
  health: HealthResponse | null;
  ready: boolean;
  error: string | null;
  retry: () => void;
}

const Ctx = createContext<AuthContext | null>(null);

// AuthProvider probes /health on mount. If the call succeeds, auth is valid
// (or unrequired). If it returns 401, the login modal is shown to capture a
// bearer token, which is then persisted to localStorage.
export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setTokenState] = useState(getToken());
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [ready, setReady] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const probe = useCallback(async () => {
    setReady(false);
    setError(null);
    try {
      const h = await api.get<HealthResponse>('/health');
      setHealth(h);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(msg);
    } finally {
      setReady(true);
    }
  }, []);

  useEffect(() => {
    void probe();
  }, [probe, token]);

  const update = useCallback((next: string) => {
    setToken(next);
    setTokenState(next);
  }, []);

  return (
    <Ctx.Provider value={{ token, setToken: update, health, ready, error, retry: probe }}>
      {children}
    </Ctx.Provider>
  );
}

export function useAuth(): AuthContext {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error('useAuth must be inside <AuthProvider>');
  return ctx;
}
