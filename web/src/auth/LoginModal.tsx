import { useState } from 'react';
import { useAuth } from './AuthProvider';

// Shown whenever there is no valid session. Exchanges an ID and password for a
// short-lived access token plus a rotating refresh token, both held by the API
// client — nothing here touches storage directly.
export function LoginModal() {
  const { login, error } = useAuth();
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

  return (
    <div className="modal-overlay">
      <div className="modal">
        <h2 style={{ marginTop: 0 }}>Karakuri</h2>
        <p className="muted small">
          Sign in to continue. On a fresh install the server creates an{' '}
          <code>admin</code> account with the password from{' '}
          <code>KARAKURI_AUTH_BOOTSTRAP_PASSWORD</code>.
        </p>
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
