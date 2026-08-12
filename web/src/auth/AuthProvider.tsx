import { createContext, useCallback, useContext, useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { api, login as apiLogin, logout as apiLogout } from '@/api/client';
import type { HealthResponse } from '@/api/types';

/** The calling principal's identity and effective permissions, from /auth/me. */
export interface Identity {
  principal: { id: string; name?: string; kind?: string };
  roles: string[];
  permissions: string[];
}

/**
 * What kind of login this server offers, from /auth/sso/config.
 *
 * The endpoint is public because the login screen has to render before anyone
 * has a credential — and it is deliberately thin. Which identity provider an
 * organisation uses is not something to describe to unauthenticated callers.
 */
export interface LoginOptions {
  provider: string;
  enabled: boolean;
  password_login: boolean;
  login_url?: string;
}

interface AuthContext {
  identity: Identity | null;
  health: HealthResponse | null;
  ready: boolean;
  error: string | null;
  /** How this server expects people to sign in. Null until the probe lands. */
  loginOptions: LoginOptions | null;
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
  const [loginOptions, setLoginOptions] = useState<LoginOptions | null>(null);
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
    // Which login to offer is asked for regardless of whether we are signed
    // in: it is what the form renders from, and a server that cannot answer
    // still has a password form worth showing.
    try {
      setLoginOptions(await api.get<LoginOptions>('/auth/sso/config'));
    } catch {
      setLoginOptions({ provider: 'bearer', enabled: false, password_login: true });
    }
    // There is no local session to inspect — the cookies are httpOnly — so
    // identity is established by asking the server who we are.
    try {
      setIdentity(await api.get<Identity>('/auth/me'));
    } catch {
      // No session, expired, or revoked — fall through to the login form.
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
    <Ctx.Provider value={{ identity, health, ready, error, loginOptions, login, logout, can }}>
      {children}
    </Ctx.Provider>
  );
}

export function useAuth(): AuthContext {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error('useAuth must be inside <AuthProvider>');
  return ctx;
}
