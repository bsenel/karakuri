import { useState } from 'react';
import { useFetch } from '@/api/useApi';
import { DataTable } from '@/components/DataTable';

interface Tier {
  limit: number;
  remaining: number;
  used: number;
  reset_at: string;
  allowed: boolean;
}

interface Usage {
  twin_id: string;
  tiers: Record<string, Tier>;
}

interface Twin {
  id: string;
  name: string;
}

/**
 * What one twin has spent of its allowances.
 *
 * `used` is a fraction rather than a count — the server reports it that way
 * because the pressure threshold is a fraction — so the bar is the honest
 * rendering and the numbers beside it are the detail.
 */
export function QuotaUsage() {
  const twins = useFetch<Twin[]>('/twins');
  const [twinID, setTwinID] = useState('');
  const usage = useFetch<Usage>(twinID ? `/quota/usage?twin=${encodeURIComponent(twinID)}` : null, [twinID]);

  const rows = Object.entries(usage.data?.tiers ?? {}).map(([name, tier]) => ({ name, ...tier }));

  return (
    <>
      <div className="row" style={{ marginBottom: 12 }}>
        <select
          value={twinID}
          onChange={(e) => setTwinID(e.target.value)}
          aria-label="Twin"
        >
          <option value="">choose a twin…</option>
          {(twins.data ?? []).map((t) => (
            <option key={t.id} value={t.id}>
              {t.name}
            </option>
          ))}
        </select>
      </div>

      {!twinID && (
        <p className="muted">
          Pick a twin. Quotas are counted per twin, so there is no deployment-wide answer
          to show here — the limits themselves are under Limits.
        </p>
      )}

      {twinID && (
        <DataTable
          loading={usage.loading}
          error={usage.error}
          rows={rows}
          keyOf={(r) => r.name}
          empty="No tiers are counted for this twin."
          columns={[
            { header: 'Tier', render: (r) => <span className="mono">{r.name}</span> },
            {
              header: 'Used',
              render: (r) => (
                <div style={{ minWidth: 160 }}>
                  <div className="progress">
                    <div
                      style={{
                        width: `${Math.min(100, Math.round(r.used * 100))}%`,
                        // Amber at the pressure threshold, which is where the
                        // server starts publishing quota_pressure — the colour
                        // and the event should mean the same thing.
                        background: r.used >= 0.8 ? 'var(--amber)' : 'var(--accent)',
                      }}
                    />
                  </div>
                  <span className="muted small">{Math.round(r.used * 100)}%</span>
                </div>
              ),
            },
            { header: 'Remaining', numeric: true, render: (r) => r.remaining.toLocaleString() },
            { header: 'Limit', numeric: true, render: (r) => r.limit.toLocaleString() },
            {
              header: 'Resets',
              render: (r) => (
                <span className="muted small">
                  {r.reset_at ? new Date(r.reset_at).toLocaleString() : '—'}
                </span>
              ),
            },
            {
              header: '',
              render: (r) =>
                r.allowed ? null : <span className="pill red">exhausted</span>,
            },
          ]}
        />
      )}
    </>
  );
}
