import { useState } from 'react';
import { api } from '@/api/client';
import { describe, useFetch } from '@/api/useApi';
import { useAuth } from '@/auth/AuthProvider';
import { Actions } from '@/auth/permissions';

interface PolicySummary {
  algorithm: string;
  limit: number;
  window: string;
  per_second: number;
}

interface QuotaSummary {
  cap: number;
  period: string;
}

interface Config {
  request: PolicySummary;
  capability: QuotaSummary;
  llm_tokens: QuotaSummary;
  adapter: QuotaSummary;
  pressure_threshold: number;
  editable: boolean;
  configured: {
    request: PolicySummary;
    capability: QuotaSummary;
    llm_tokens: QuotaSummary;
    adapter: QuotaSummary;
  };
}

interface StoredTier {
  name: string;
  cap: number;
  window?: number;
  rate?: number;
  reason: string;
  updated_by: string;
  updated_at: string;
}

const quotaTiers = [
  { name: 'llm-tokens', key: 'llm_tokens' as const, label: 'LLM tokens', unit: 'tokens per day' },
  { name: 'capability', key: 'capability' as const, label: 'Capability', unit: 'calls per day' },
  { name: 'adapter', key: 'adapter' as const, label: 'Adapter', unit: 'calls per day' },
];

/**
 * The limits themselves.
 *
 * This is the page the database-backed tiers exist for, and the pairing it
 * renders is the whole reason that change needed care: **configured** is what
 * the YAML file says and **in force** is what the server is enforcing. Once the
 * database wins, an operator reading the file is reading the seed — so the two
 * numbers appear together, always, and never one without the other.
 */
export function QuotaSettings() {
  const { can } = useAuth();
  const config = useFetch<Config>('/quota');
  const tiers = useFetch<{ stored: StoredTier[]; editable: boolean }>('/quota/tiers');
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<string | null>(null);

  const mayEdit = can(Actions.quotaAdmin);
  const stored = new Map((tiers.data?.stored ?? []).map((t) => [t.name, t]));

  const reload = () => {
    config.reload();
    tiers.reload();
  };

  const reset = async (name: string) => {
    setError(null);
    try {
      await api.del(`/quota/tiers/${encodeURIComponent(name)}`);
      reload();
    } catch (e) {
      setError(describe(e));
    }
  };

  if (config.error) return <p className="error">{config.error}</p>;
  if (config.loading || !config.data) return <p className="muted">Loading…</p>;
  const c = config.data;

  return (
    <>
      <p className="muted small">
        The configuration file seeds these; a limit set here is stored in the database and
        takes precedence over it. That is why both numbers are shown — the file you are
        looking at is not necessarily the limit being enforced.
      </p>

      {!c.editable && (
        <p className="muted">
          This deployment keeps no database, so limits come from configuration only. An
          edit here would not survive a restart, and the server refuses one.
        </p>
      )}
      {error && <p className="error">{error}</p>}

      {quotaTiers.map((tier) => {
        const inForce = c[tier.key].cap;
        const configured = c.configured[tier.key].cap;
        const row = stored.get(tier.name);
        return (
          <div className="card" key={tier.name} style={{ marginTop: 12 }}>
            <div className="row">
              <strong>{tier.label}</strong>
              <div className="pair">
                <span>{inForce.toLocaleString()}</span>
                {configured !== inForce && (
                  <span className="was small">{configured.toLocaleString()}</span>
                )}
                <span className="muted small">{tier.unit}</span>
              </div>
              <div className="grow" />
              {mayEdit && c.editable && (
                <>
                  <button className="linklike small" onClick={() => setEditing(tier.name)}>
                    change
                  </button>
                  {row && (
                    <button className="linklike small" onClick={() => void reset(tier.name)}>
                      reset to configured
                    </button>
                  )}
                </>
              )}
            </div>
            {row && (
              <p className="muted small" style={{ marginTop: 4 }}>
                Set by {row.updated_by} — {row.reason}
              </p>
            )}
            {editing === tier.name && (
              <EditForm
                name={tier.name}
                current={inForce}
                onDone={() => {
                  setEditing(null);
                  reload();
                }}
                onCancel={() => setEditing(null)}
                onError={setError}
              />
            )}
          </div>
        );
      })}

      <div className="card" style={{ marginTop: 12 }}>
        <div className="row">
          <strong>Request rate</strong>
          <div className="pair">
            <span>
              {Math.round(c.request.per_second * 60)}/min, bursting to {c.request.limit}
            </span>
            {c.configured.request.limit !== c.request.limit && (
              <span className="was small">
                {Math.round(c.configured.request.per_second * 60)}/min to{' '}
                {c.configured.request.limit}
              </span>
            )}
          </div>
          <div className="grow" />
          {mayEdit && c.editable && (
            <>
              <button className="linklike small" onClick={() => setEditing('request')}>
                change
              </button>
              {stored.has('request') && (
                <button className="linklike small" onClick={() => void reset('request')}>
                  reset to configured
                </button>
              )}
            </>
          )}
        </div>
        <p className="muted small">
          Counted by {c.request.algorithm.replace('_', ' ')}, which is not editable here:
          changing how a limit is counted is a decision about the shape of the traffic,
          not about how much of it to allow.
        </p>
        {editing === 'request' && (
          <RateForm
            perMinute={Math.round(c.request.per_second * 60)}
            burst={c.request.limit}
            onDone={() => {
              setEditing(null);
              reload();
            }}
            onCancel={() => setEditing(null)}
            onError={setError}
          />
        )}
      </div>

      <p className="muted small" style={{ marginTop: 16 }}>
        A twin reaching {Math.round(c.pressure_threshold * 100)}% of any tier publishes a{' '}
        <code>quota_pressure</code> event, so somebody learns about a ceiling before it
        starts refusing work.
      </p>
    </>
  );
}

function EditForm({
  name,
  current,
  onDone,
  onCancel,
  onError,
}: {
  name: string;
  current: number;
  onDone: () => void;
  onCancel: () => void;
  onError: (message: string | null) => void;
}) {
  const [cap, setCap] = useState(String(current));
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    setBusy(true);
    onError(null);
    try {
      await api.put(`/quota/tiers/${encodeURIComponent(name)}`, {
        cap: Number(cap),
        reason,
      });
      onDone();
    } catch (e) {
      onError(describe(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="row" style={{ marginTop: 8, flexWrap: 'wrap' }}>
      <input
        type="number"
        value={cap}
        onChange={(e) => setCap(e.target.value)}
        aria-label="New cap"
      />
      <input
        placeholder="why — this changes the limit for everybody"
        value={reason}
        onChange={(e) => setReason(e.target.value)}
        aria-label="Reason"
        style={{ minWidth: 300 }}
      />
      <button className="primary" disabled={busy || !cap || !reason} onClick={() => void submit()}>
        Save
      </button>
      <button onClick={onCancel}>Cancel</button>
    </div>
  );
}

function RateForm({
  perMinute,
  burst,
  onDone,
  onCancel,
  onError,
}: {
  perMinute: number;
  burst: number;
  onDone: () => void;
  onCancel: () => void;
  onError: (message: string | null) => void;
}) {
  const [rate, setRate] = useState(String(perMinute));
  const [cap, setCap] = useState(String(burst));
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    setBusy(true);
    onError(null);
    try {
      await api.put('/quota/tiers/request', {
        cap: Number(cap),
        window: '1m0s',
        // The server stores a per-second refill; the form asks per minute
        // because that is the unit the limit is described in everywhere else.
        rate: Number(rate) / 60,
        reason,
      });
      onDone();
    } catch (e) {
      onError(describe(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="row" style={{ marginTop: 8, flexWrap: 'wrap' }}>
      <label className="small muted">
        per minute
        <input
          type="number"
          value={rate}
          onChange={(e) => setRate(e.target.value)}
          aria-label="Requests per minute"
        />
      </label>
      <label className="small muted">
        burst
        <input
          type="number"
          value={cap}
          onChange={(e) => setCap(e.target.value)}
          aria-label="Burst"
        />
      </label>
      <input
        placeholder="why"
        value={reason}
        onChange={(e) => setReason(e.target.value)}
        aria-label="Reason"
        style={{ minWidth: 260 }}
      />
      <button className="primary" disabled={busy || !reason} onClick={() => void submit()}>
        Save
      </button>
      <button onClick={onCancel}>Cancel</button>
    </div>
  );
}
