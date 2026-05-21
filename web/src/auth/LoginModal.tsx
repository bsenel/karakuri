import { useState } from 'react';
import { useAuth } from './AuthProvider';

// Shown when /health returns 401. Captures a bearer token, writes it to
// localStorage via AuthProvider, and triggers a re-probe.
export function LoginModal() {
  const { setToken, retry, error } = useAuth();
  const [value, setValue] = useState('');

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    setToken(value.trim());
    retry();
  };

  return (
    <div className="modal-overlay">
      <div className="modal">
        <h2 style={{ marginTop: 0 }}>Karakuri</h2>
        <p className="muted small">
          The server requires authentication. Paste the bearer token from
          <code> auth.token</code> in <code>config.yaml</code> (or the
          <code> KARAKURI_AUTH_TOKEN</code> env var).
        </p>
        <form onSubmit={submit} className="col">
          <div>
            <label>Bearer token</label>
            <input
              type="password"
              autoFocus
              autoComplete="off"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              placeholder="krk_…"
            />
          </div>
          {error && <p className="pill red">{error}</p>}
          <button type="submit" className="primary">Continue</button>
        </form>
      </div>
    </div>
  );
}
