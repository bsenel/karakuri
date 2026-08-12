import { useState } from 'react';
import { useAuth } from './AuthProvider';

// Shown whenever there is no valid session.
//
// Two ways in, and both are always offered when the server supports them.
// Federated login is a full-page navigation rather than a fetch, because the
// identity provider needs the browser itself — and the session comes back as
// httpOnly cookies, which is why nothing here handles a token.
//
// The password form stays even when a provider is configured. That is the
// break-glass path when the identity provider is unreachable, and the one way
// an administrator gets in to fix it.
export function LoginModal() {
  const { login, error, loginOptions } = useAuth();
  const [id, setID] = useState('');
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    try {
      await login(id.trim(), password);
    } catch {
      // AuthProvider owns the message; clear the field so a retry starts clean.
      setPassword('');
    } finally {
      setBusy(false);
    }
  };

  // Only offered once the probe has landed and reported a provider; rendering
  // a dead button while that is in flight would be worse than a brief absence.
  const sso = loginOptions?.enabled && loginOptions.login_url ? loginOptions : null;

  return (
    <div className="modal-overlay">
      <div className="modal">
        <h2 style={{ marginTop: 0 }}>Karakuri</h2>
        <p className="muted small">
          Sign in to continue. On a fresh install the server creates an{' '}
          <code>admin</code> account with the password from{' '}
          <code>KARAKURI_AUTH_BOOTSTRAP_PASSWORD</code>.
        </p>
        {sso && (
          <div className="col" style={{ marginBottom: '1rem' }}>
            <a className="button primary" href={sso.login_url} style={{ textAlign: 'center' }}>
              Sign in with {providerLabel(sso.provider)}
            </a>
            <p className="muted small" style={{ textAlign: 'center', margin: 0 }}>
              or sign in with a password
            </p>
          </div>
        )}
        <form onSubmit={submit} className="col">
          <div>
            <label htmlFor="login-id">User</label>
            <input
              id="login-id"
              autoFocus
              autoComplete="username"
              value={id}
              onChange={(e) => setID(e.target.value)}
              placeholder="admin"
            />
          </div>
          <div>
            <label htmlFor="login-password">Password</label>
            <input
              id="login-password"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>
          {error && <p className="pill red">{error}</p>}
          <button type="submit" className="primary" disabled={busy || !id || !password}>
            {busy ? 'Signing in…' : 'Sign in'}
          </button>
        </form>
      </div>
    </div>
  );
}

// providerLabel names the protocol the way somebody signing in would recognise
// it. It deliberately does not name the organisation's identity provider, which
// the server does not tell unauthenticated callers either.
function providerLabel(provider: string): string {
  switch (provider) {
    case 'oidc':
      return 'SSO';
    case 'saml':
      return 'SAML';
    default:
      return provider;
  }
}
