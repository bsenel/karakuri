import { useState } from 'react';
import { api } from '@/api/client';
import { describe, useFetch } from '@/api/useApi';
import { useAuth } from '@/auth/AuthProvider';
import { Actions } from '@/auth/permissions';
import { DataTable } from '@/components/DataTable';

interface Request {
  id: string;
  subject: string;
  name: string;
  cap: number;
  reason: string;
  status: 'pending' | 'approved' | 'rejected';
  requested_by: string;
  created_at: string;
  decided_by?: string;
  decision_note?: string;
}

interface Override {
  subject: string;
  name: string;
  cap: number;
  reason?: string;
  expires_at?: string;
}

interface Twin {
  id: string;
  name: string;
}

const tiers = ['llm-tokens', 'capability', 'adapter', 'request'];

/**
 * The self-service workflow from Phase 18: ask, and somebody decides.
 *
 * The two halves are on one page because they are two views of the same thing —
 * a pending request and the override it becomes — and separating them would
 * make "did my approval actually do anything" a navigation problem.
 */
export function QuotaRequests() {
  const { can } = useAuth();
  const requests = useFetch<Request[]>('/quota/requests');
  const overrides = useFetch<Override[]>('/quota/overrides');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const mayDecide = can(Actions.quotaApprove);

  const decide = async (id: string, approve: boolean) => {
    setBusy(id);
    setError(null);
    try {
      await api.post(`/quota/requests/${encodeURIComponent(id)}/decide`, { approve, note: '' });
      requests.reload();
      overrides.reload();
    } catch (e) {
      // The refusal here is worth reading in full: "you can only approve a
      // raise for a subject you already hold" is the tenancy rule explaining
      // itself, not a generic 403.
      setError(describe(e));
    } finally {
      setBusy(null);
    }
  };

  const revoke = async (o: Override) => {
    setBusy(o.subject + o.name);
    setError(null);
    try {
      await api.del(
        `/quota/overrides/${encodeURIComponent(o.subject)}/${encodeURIComponent(o.name)}`,
      );
      overrides.reload();
    } catch (e) {
      setError(describe(e));
    } finally {
      setBusy(null);
    }
  };

  return (
    <>
      {error && <p className="error">{error}</p>}

      {can(Actions.quotaRequest) && <AskForm onDone={requests.reload} onError={setError} />}

      <h2>Requests</h2>
      <DataTable
        loading={requests.loading}
        error={requests.error}
        rows={requests.data ?? []}
        keyOf={(r) => r.id}
        empty="No requests you can see. You see your own, and any you could approve."
        columns={[
          {
            header: 'Subject',
            render: (r) => (
              <>
                <span className="mono">{r.subject}</span>
                <div className="muted small">{r.name}</div>
              </>
            ),
          },
          { header: 'Asking for', numeric: true, render: (r) => r.cap.toLocaleString() },
          {
            header: 'Reason',
            render: (r) => (
              <>
                {r.reason}
                <div className="muted small">
                  {r.requested_by} · {new Date(r.created_at).toLocaleDateString()}
                </div>
              </>
            ),
          },
          {
            header: 'Status',
            render: (r) => (
              <span
                className={
                  r.status === 'approved'
                    ? 'pill green'
                    : r.status === 'rejected'
                      ? 'pill red'
                      : 'pill amber'
                }
              >
                {r.status}
                {r.decided_by && <span className="muted"> · {r.decided_by}</span>}
              </span>
            ),
          },
          {
            header: '',
            render: (r) =>
              r.status === 'pending' && mayDecide ? (
                <div className="row">
                  <button disabled={busy === r.id} onClick={() => void decide(r.id, true)}>
                    Approve
                  </button>
                  <button
                    className="linklike small"
                    disabled={busy === r.id}
                    onClick={() => void decide(r.id, false)}
                  >
                    reject
                  </button>
                </div>
              ) : null,
          },
        ]}
      />

      <h2>Raises in force</h2>
      <p className="muted small">
        What an approval actually wrote. Nothing here is a request any more — these are
        the limits being enforced right now, and revoking one puts that subject back on
        the tier.
      </p>
      <DataTable
        loading={overrides.loading}
        error={overrides.error}
        rows={overrides.data ?? []}
        keyOf={(o) => o.subject + o.name}
        empty="No raises in force — every subject is on the configured tier."
        columns={[
          { header: 'Subject', render: (o) => <span className="mono">{o.subject}</span> },
          { header: 'Tier', render: (o) => o.name },
          { header: 'Cap', numeric: true, render: (o) => o.cap.toLocaleString() },
          {
            header: 'Until',
            render: (o) =>
              o.expires_at ? (
                new Date(o.expires_at).toLocaleDateString()
              ) : (
                <span className="muted small">permanent</span>
              ),
          },
          { header: 'Reason', render: (o) => <span className="muted small">{o.reason}</span> },
          {
            header: '',
            render: (o) =>
              mayDecide ? (
                <button
                  className="linklike small"
                  disabled={busy === o.subject + o.name}
                  onClick={() => void revoke(o)}
                >
                  revoke
                </button>
              ) : null,
          },
        ]}
      />
    </>
  );
}

function AskForm({
  onDone,
  onError,
}: {
  onDone: () => void;
  onError: (message: string | null) => void;
}) {
  const twins = useFetch<Twin[]>('/twins');
  const [open, setOpen] = useState(false);
  const [tier, setTier] = useState('llm-tokens');
  const [twin, setTwin] = useState('');
  const [cap, setCap] = useState('');
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);

  if (!open) {
    return (
      <div className="row" style={{ marginBottom: 16 }}>
        <button className="primary" onClick={() => setOpen(true)}>
          Ask for more
        </button>
      </div>
    );
  }

  const submit = async () => {
    setBusy(true);
    onError(null);
    try {
      await api.post('/quota/requests', {
        tier,
        twin,
        cap: Number(cap),
        reason,
      });
      setOpen(false);
      setCap('');
      setReason('');
      onDone();
    } catch (e) {
      onError(describe(e));
    } finally {
      setBusy(false);
    }
  };

  // The request tier is per principal and defaults to you, so it needs no twin.
  const needsTwin = tier !== 'request';

  return (
    <div className="card" style={{ marginBottom: 16 }}>
      <h3>Ask for more</h3>
      <div className="row" style={{ flexWrap: 'wrap' }}>
        <select value={tier} onChange={(e) => setTier(e.target.value)} aria-label="Tier">
          {tiers.map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </select>
        {needsTwin && (
          <select value={twin} onChange={(e) => setTwin(e.target.value)} aria-label="Twin">
            <option value="">choose a twin…</option>
            {(twins.data ?? []).map((t) => (
              <option key={t.id} value={t.id}>
                {t.name}
              </option>
            ))}
          </select>
        )}
        <input
          type="number"
          placeholder="new cap"
          value={cap}
          onChange={(e) => setCap(e.target.value)}
          aria-label="Cap"
        />
        <input
          placeholder="why you need it"
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          aria-label="Reason"
          style={{ minWidth: 240 }}
        />
        <button
          className="primary"
          disabled={busy || !cap || !reason || (needsTwin && !twin)}
          onClick={() => void submit()}
        >
          Submit
        </button>
        <button onClick={() => setOpen(false)}>Cancel</button>
      </div>
      <p className="muted small" style={{ marginTop: 8 }}>
        A reason is required: a limit raised for a reason nobody wrote down is one nobody
        can review later. Nothing changes until somebody approves it.
      </p>
    </div>
  );
}
