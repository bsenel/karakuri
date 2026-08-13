import { useEffect, useMemo, useState } from 'react';
import {
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { useFetch } from '@/api/useApi';
import { streamAll } from '@/api/sse';
import { Actions } from '@/auth/permissions';
import { DataTable } from '@/components/DataTable';
import { RequirePermission } from '@/components/RequirePermission';

interface Bucket {
  key: string[];
  units: number;
  cost: number;
  events: number;
}

interface Container {
  id: string;
  kind: string;
  name: string;
}

const ranges = [
  { label: '24 hours', hours: 24 },
  { label: '7 days', hours: 24 * 7 },
  { label: '30 days', hours: 24 * 30 },
];

export function CostPage() {
  return (
    <RequirePermission action={Actions.costRead} what="spend">
      <Cost />
    </RequirePermission>
  );
}

function Cost() {
  const [hours, setHours] = useState(24 * 7);
  const [container, setContainer] = useState('');
  const [live, setLive] = useState(0);

  const since = useMemo(
    () => new Date(Date.now() - hours * 3600_000).toISOString(),
    // `live` is in the dependency list so an arriving cost event refetches the
    // report. Recomputing `since` on every event would also work and would move
    // the window under the reader mid-glance, which is worse than one extra
    // request.
    [hours],
  );

  const query = (groupBy: string) => {
    const params = new URLSearchParams({ since, group_by: groupBy });
    if (container) params.set('label', container);
    return `/cost?${params.toString()}`;
  };

  const byDay = useFetch<Bucket[]>(query('day'), [since, container, live]);
  const byProvider = useFetch<Bucket[]>(query('provider'), [since, container, live]);
  const byModel = useFetch<Bucket[]>(query('model'), [since, container, live]);
  const containers = useFetch<Container[]>('/containers');

  // Live updates. The stream is already narrowed to what this caller may see —
  // the server tests every event against the same bindings that decide which
  // twins they can list — so there is nothing to filter here, and adding a
  // filter would only hide events the server judged visible.
  useEffect(() => {
    const stream = streamAll((event) => {
      if (event.type === 'cost_recorded') setLive((n) => n + 1);
    });
    return () => stream.close();
  }, []);

  const total = (byDay.data ?? []).reduce((sum, b) => sum + b.cost, 0);
  const units = (byDay.data ?? []).reduce((sum, b) => sum + b.units, 0);

  const dayRows = useMemo(
    () =>
      [...(byDay.data ?? [])]
        .map((b) => ({ day: b.key[0] ?? '', cost: round(b.cost), events: b.events }))
        .sort((a, b) => a.day.localeCompare(b.day)),
    [byDay.data],
  );

  const providerRows = useMemo(
    () =>
      (byProvider.data ?? []).map((b) => ({
        provider: b.key[0] || 'unattributed',
        cost: round(b.cost),
      })),
    [byProvider.data],
  );

  return (
    <>
      <h1>Cost</h1>
      <p className="muted small">
        What the work spent, priced from the rate table in configuration. With no table
        configured the units are still counted and the money reads zero — an honest answer
        rather than an invented one. This report shows only what you may see.
      </p>

      <div className="row" style={{ marginBottom: 16, flexWrap: 'wrap' }}>
        <select
          value={hours}
          onChange={(e) => setHours(Number(e.target.value))}
          aria-label="Range"
        >
          {ranges.map((r) => (
            <option key={r.hours} value={r.hours}>
              {r.label}
            </option>
          ))}
        </select>
        <select
          value={container}
          onChange={(e) => setContainer(e.target.value)}
          aria-label="Container"
        >
          {/* Naming a container narrows the report. It can never widen it: the
              server intersects this with the tenancy filter, so asking for
              another tenant returns nothing rather than their spend. */}
          <option value="">everything you can see</option>
          {(containers.data ?? []).map((c) => (
            <option key={c.id} value={`${c.kind}:${c.id}`}>
              {c.kind}: {c.name}
            </option>
          ))}
        </select>
        <div className="grow" />
        <span className="small muted">
          {formatMoney(total)} · {units.toLocaleString()} units
        </span>
      </div>

      {byDay.error && <p className="error">{byDay.error}</p>}

      <div className="card">
        <h3>Per day</h3>
        {dayRows.length === 0 ? (
          <p className="muted">
            Nothing recorded in this range. A deployment with no database records no
            spend, and neither does one that has not run a loop yet.
          </p>
        ) : (
          <div className="chart">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={dayRows}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                <XAxis dataKey="day" tick={{ fontSize: 11 }} stroke="var(--fg-muted)" />
                <YAxis tick={{ fontSize: 11 }} stroke="var(--fg-muted)" />
                <Tooltip
                  contentStyle={{
                    background: 'var(--bg-elev)',
                    border: '1px solid var(--border)',
                    fontSize: 12,
                  }}
                  formatter={(value) => formatMoney(Number(value ?? 0))}
                />
                <Bar dataKey="cost" fill="var(--accent)" />
              </BarChart>
            </ResponsiveContainer>
          </div>
        )}
      </div>

      <div className="card" style={{ marginTop: 12 }}>
        <h3>Per provider</h3>
        {providerRows.length === 0 ? (
          <p className="muted">Nothing to attribute.</p>
        ) : (
          <div className="chart">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={providerRows} layout="vertical">
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                <XAxis type="number" tick={{ fontSize: 11 }} stroke="var(--fg-muted)" />
                <YAxis
                  type="category"
                  dataKey="provider"
                  width={110}
                  tick={{ fontSize: 11 }}
                  stroke="var(--fg-muted)"
                />
                <Tooltip
                  contentStyle={{
                    background: 'var(--bg-elev)',
                    border: '1px solid var(--border)',
                    fontSize: 12,
                  }}
                  formatter={(value) => formatMoney(Number(value ?? 0))}
                />
                <Legend wrapperStyle={{ fontSize: 12 }} />
                <Bar dataKey="cost" fill="var(--green)" name="cost" />
              </BarChart>
            </ResponsiveContainer>
          </div>
        )}
      </div>

      <h2>Per model</h2>
      <DataTable
        loading={byModel.loading}
        error={byModel.error}
        rows={byModel.data ?? []}
        keyOf={(b) => b.key.join('|') || 'total'}
        empty="Nothing recorded in this range."
        columns={[
          {
            header: 'Model',
            render: (b) => <span className="mono">{b.key[0] || 'not a model call'}</span>,
          },
          { header: 'Units', numeric: true, render: (b) => b.units.toLocaleString() },
          {
            header: 'Events',
            numeric: true,
            // What tells a reader whether a number is one expensive call or a
            // thousand cheap ones.
            render: (b) => b.events.toLocaleString(),
          },
          { header: 'Cost', numeric: true, render: (b) => formatMoney(b.cost) },
        ]}
      />
    </>
  );
}

function round(n: number): number {
  return Math.round(n * 10000) / 10000;
}

/**
 * Money, in whatever currency the rate table was written in.
 *
 * No symbol: the ledger stores whole currency units and never learns which
 * currency, so printing a "$" would be inventing information. Four decimals
 * because a single model call routinely costs less than a cent.
 */
export function formatMoney(n: number): string {
  if (n === 0) return '0';
  if (n < 0.0001) return '<0.0001';
  return n.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 4 });
}
