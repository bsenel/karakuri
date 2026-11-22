import { useState } from 'react';
import { useFetch } from '@/api/useApi';
import type { Block, BlockKind, Digest, DigestObjective } from '@/api/types';

interface TwinOption {
  id: string;
  name: string;
}

const windows = [
  { label: 'default', value: '' },
  { label: '24 hours', value: '24h' },
  { label: '7 days', value: '168h' },
  { label: '30 days', value: '720h' },
];

// A person reads these, not the enum the API carries.
const blockLabels: Record<BlockKind, string> = {
  waiting_for_quota: 'Waiting for quota',
  bounds_violation: 'Bounds violation',
  remote_unreadable: 'Remote unreadable',
};

const when = (iso: string) => new Date(iso).toLocaleString();

export function OversightPage() {
  const [twinId, setTwinId] = useState('');
  const [window, setWindow] = useState('');
  const twins = useFetch<TwinOption[]>('/twins');

  const params = new URLSearchParams({ twin_id: twinId });
  if (window) params.set('window', window);
  // No twin picked, no request: an empty digest would read as "nothing happened".
  const digest = useFetch<Digest>(twinId ? `/reports/preview?${params.toString()}` : null);

  const objectives = digest.data?.objectives ?? [];

  return (
    <>
      <h1>Oversight</h1>
      <p className="muted small">
        What each standing objective did in the window: its passes, what the bounds refused and
        what it is blocked on.
      </p>
      <div className="row" style={{ marginBottom: 16, flexWrap: 'wrap' }}>
        <select value={twinId} onChange={(e) => setTwinId(e.target.value)} aria-label="Twin">
          <option value="">pick a twin</option>
          {(twins.data ?? []).map((t) => (
            <option key={t.id} value={t.id}>
              {t.name}
            </option>
          ))}
        </select>
        <select value={window} onChange={(e) => setWindow(e.target.value)} aria-label="Window">
          {windows.map((w) => (
            <option key={w.value} value={w.value}>
              {w.label}
            </option>
          ))}
        </select>
      </div>

      {twins.error && <p className="error" role="alert">{twins.error}</p>}
      {digest.error ? (
        <>
          <p className="error" role="alert">{digest.error}</p>
          <button onClick={digest.reload}>Retry</button>
        </>
      ) : digest.loading ? (
        <p className="muted" role="status">Loading…</p>
      ) : digest.data && objectives.length === 0 ? (
        <p className="muted">This twin has no standing objectives.</p>
      ) : (
        objectives.map((o) => <Stream key={o.id} objective={o} />)
      )}
    </>
  );
}

function Stream({ objective: o }: { objective: DigestObjective }) {
  const headingId = `oversight-${o.id}`;
  const refusals = [...(o.refusals ?? [])].sort((a, b) => a.at.localeCompare(b.at));
  const blocks: Block[] = o.blocks ?? [];
  const quiet = refusals.length === 0 && blocks.length === 0;

  return (
    <section className="card" aria-labelledby={headingId} style={{ marginBottom: 12 }}>
      <h3 id={headingId}>{o.title}</h3>
      {o.paused && <p className="small">Paused{o.paused_why ? `: ${o.paused_why}` : ''}</p>}
      <p className="small muted">
        <span>Senses: {o.senses}</span> · <span>Reconciles: {o.reconciles}</span> ·{' '}
        <span>Converged: {o.converged}</span> · <span>Escalations: {o.escalations}</span> ·{' '}
        <span>Failures: {o.failures}</span>
      </p>
      {quiet && <p className="muted">No refusals and no blocks in this window.</p>}
      {refusals.length > 0 && (
        <ul aria-label="Refusals">
          {refusals.map((r, i) => (
            <li key={`${r.at}-${i}`}>
              <span className="muted">{when(r.at)}</span>{' '}
              <span className="mono">{r.capability}</span> {r.reason}
            </li>
          ))}
        </ul>
      )}
      {blocks.length > 0 && (
        <ul aria-label="Blocks">
          {blocks.map((b, i) => (
            <li key={`${b.at}-${i}`}>
              <strong>{blockLabels[b.kind] ?? b.kind}</strong> {b.reason}{' '}
              <span className="muted">{when(b.at)}</span>
              {b.until && <span> · until {when(b.until)}</span>}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
