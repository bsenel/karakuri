import { createContext, useCallback, useContext, useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { api, getSession, login as apiLogin, logout as apiLogout } from '@/api/client';
import type { HealthResponse } from '@/api/types';

/** The calling principal's identity and effective permissions, from /auth/me. */
export interface Identity {
  principal: { id: string; name?: string; kind?: string };
  roles: string[];
  permissions: string[];
}

interface AuthContext {
  identity: Identity | null;
  health: HealthResponse | null;
  ready: boolean;
  error: string | null;
  login: (id: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  /** True when the principal holds the given action, e.g. `can('audit:read')`. */
  can: (action: string) => boolean;
}

const Ctx = createContext<AuthContext | null>(null);

// AuthProvider establishes who the user is before rendering the app.
//
// /health is public, so it is probed regardless — an operator should be able to
// see that a server is up before logging in. Identity comes from /auth/me,
// which needs a valid session; failing that, the login form is shown.
export function AuthProvider({ children }: { children: ReactNode }) {
  const [identity, setIdentity] = useState<Identity | null>(null);
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [ready, setReady] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const probe = useCallback(async () => {
    setReady(false);
    setError(null);
    try {
      setHealth(await api.get<HealthResponse>('/health'));
    } catch {
      // A server that cannot answer /health is a separate problem from being
      // logged out; the health page reports it in detail.
      setHealth(null);
    }
    if (getSession()) {
      try {
        setIdentity(await api.get<Identity>('/auth/me'));
      } catch {
        // Expired or revoked — fall through to the login form.
        setIdentity(null);
      }
    } else {
      setIdentity(null);
    }
    setReady(true);
  }, []);

  useEffect(() => {
    void probe();
  }, [probe]);

  const login = useCallback(
    async (id: string, password: string) => {
      setError(null);
      try {
        await apiLogin(id, password);
      } catch {
        // The server deliberately does not distinguish an unknown user from a
        // wrong password, and neither should the UI.
        setError('Invalid credentials');
        throw new Error('invalid credentials');
      }
      await probe();
    },
    [probe],
  );

  const logout = useCallback(async () => {
    await apiLogout();
    setIdentity(null);
    await probe();
  }, [probe]);

  const can = useCallback(
    (action: string) => identity?.permissions?.includes(action) ?? false,
    [identity],
  );

  return (
    <Ctx.Provider value={{ identity, health, ready, error, login, logout, can }}>
      {children}
    </Ctx.Provider>
  );
}

export function useAuth(): AuthContext {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error('useAuth must be inside <AuthProvider>');
  return ctx;
}
