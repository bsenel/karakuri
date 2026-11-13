import { useState } from 'react';
import { api } from '@/api/client';
import { useAuth } from '@/auth/AuthProvider';
import type { StopAllResult } from '@/api/types';
import { ReasonDialog } from './ReasonDialog';

/**
 * The one control that stops every running loop and pauses every standing
 * objective the caller may stop, through POST /stop-all (Phase 37 step 6).
 *
 * Offered to a principal who may do either, since the server stops what the
 * caller may stop and names the rest as not reached. The answer is shown in
 * full, so a caller whose scope is narrower than the deployment is never left
 * thinking everything stopped.
 */
export function StopAllControl() {
  const { can } = useAuth();
  const [open, setOpen] = useState(false);
  const [result, setResult] = useState<StopAllResult | null>(null);

  if (!can('loop:stop') && !can('objective:pause')) return null;

  const stopAll = async (reason: string) => {
    setResult(await api.post<StopAllResult>('/stop-all', { reason }));
  };

  return (
    <>
      <button className="small" onClick={() => setOpen(true)}>Stop all</button>
      {open && (
        <ReasonDialog
          title="Stop every run"
          confirmLabel="Stop everything"
          onClose={() => setOpen(false)}
          onConfirm={stopAll}
        />
      )}
      {result && <StopAllSummary result={result} onClose={() => setResult(null)} />}
    </>
  );
}

function StopAllSummary({ result, onClose }: { result: StopAllResult; onClose: () => void }) {
  const loops = result.loops_stopped ?? [];
  const objectives = result.objectives_paused ?? [];
  const notReached = result.not_reached ?? [];
  const nothing = loops.length === 0 && objectives.length === 0;

  return (
    <div className="card small" style={{ position: 'absolute', right: 16, top: 48, zIndex: 10, maxWidth: 420 }}>
      {nothing && <p>Nothing was stopped or paused: no run was going.</p>}
      {loops.length > 0 && (
        <>
          <h4>Loops stopped</h4>
          <ul aria-label="Loops stopped">
            {loops.map((l) => <li key={l.loop_id} className="mono">{l.loop_id}</li>)}
          </ul>
        </>
      )}
      {objectives.length > 0 && (
        <>
          <h4>Objectives paused</h4>
          <ul aria-label="Objectives paused">
            {objectives.map((o) => (
              <li key={o.objective_id}>
                <span className="mono">{o.objective_id}</span>
                {o.pass_cancelled && <span className="muted"> · pass cancelled</span>}
              </li>
            ))}
          </ul>
        </>
      )}
      {notReached.length > 0 && (
        <>
          <h4>Not reached</h4>
          <ul aria-label="Not reached">
            {notReached.map((n) => (
              <li key={`${n.kind}:${n.id}`}>
                {n.kind} <span className="mono">{n.id}</span>: {n.reason}
              </li>
            ))}
          </ul>
        </>
      )}
      <div className="row" style={{ justifyContent: 'flex-end' }}>
        <button className="linklike small" onClick={onClose}>Dismiss</button>
      </div>
    </div>
  );
}
