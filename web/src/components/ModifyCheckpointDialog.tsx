import { useState } from 'react';
import type { Checkpoint, CheckpointAction, CheckpointModifications } from '@/api/types';

// ModifyCheckpointDialog collects the structured edits an operator
// applies when resolving a checkpoint with decision="modify" (Phase 13.5):
//
//  - per-action drop checkboxes (RemovedActions)
//  - free-form constraints (one per line → AddedConstraints[])
//  - operator-asserted confidence floor (RevisedConfidence)
//  - free-text rationale (Decision.Note)
//
// Submit invokes onSubmit; the parent makes the POST and refreshes.
export interface ModifyCheckpointDialogProps {
  checkpoint: Checkpoint;
  onClose: () => void;
  onSubmit: (note: string, modifications: CheckpointModifications) => Promise<void>;
}

export function ModifyCheckpointDialog({ checkpoint, onClose, onSubmit }: ModifyCheckpointDialogProps) {
  const actions: CheckpointAction[] = checkpoint.actions ?? [];
  const [dropped, setDropped] = useState<boolean[]>(() => actions.map(() => false));
  const [constraintsText, setConstraintsText] = useState('');
  const [floor, setFloor] = useState<string>('');
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const toggle = (i: number) => {
    setDropped((arr) => arr.map((v, idx) => (idx === i ? !v : v)));
  };

  const submit = async () => {
    setBusy(true);
    setErr(null);
    try {
      const mods: CheckpointModifications = {};
      const removed = actions.filter((_, i) => dropped[i]).map((a) => a.capability);
      if (removed.length > 0) mods.removed_actions = removed;
      const constraints = constraintsText.split('\n').map((s) => s.trim()).filter((s) => s.length > 0);
      if (constraints.length > 0) mods.added_constraints = constraints;
      if (floor !== '') {
        const f = parseFloat(floor);
        if (!Number.isNaN(f)) mods.revised_confidence = f;
      }
      await onSubmit(note, mods);
      onClose();
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <h2>Modify checkpoint</h2>
        <p className="muted small">
          The agent will re-plan using your feedback as critique, then re-enter the bounds check.
          A second escalation auto-rejects.
        </p>

        {actions.length > 0 && (
          <>
            <h3 style={{ marginTop: 16 }}>Drop actions</h3>
            <div className="col">
              {actions.map((a, i) => (
                <label key={i} className="row" style={{ alignItems: 'flex-start' }}>
                  <input
                    type="checkbox"
                    checked={dropped[i]}
                    onChange={() => toggle(i)}
                  />
                  <span style={{ marginLeft: 8 }}>
                    <code>{a.capability}</code>
                    {a.reason && <span className="muted small"> — {a.reason}</span>}
                  </span>
                </label>
              ))}
            </div>
          </>
        )}

        <h3 style={{ marginTop: 16 }}>Constraints</h3>
        <textarea
          value={constraintsText}
          onChange={(e) => setConstraintsText(e.target.value)}
          rows={3}
          placeholder="one per line — fed into the revise pass as critique"
        />

        <h3 style={{ marginTop: 16 }}>Confidence floor (optional)</h3>
        <input
          type="number"
          min={0}
          max={1}
          step={0.05}
          value={floor}
          onChange={(e) => setFloor(e.target.value)}
          placeholder={`current: ${checkpoint.confidence ?? '—'}`}
        />

        <h3 style={{ marginTop: 16 }}>Note</h3>
        <textarea
          value={note}
          onChange={(e) => setNote(e.target.value)}
          rows={2}
          placeholder="free-form rationale — recorded on the audit row"
        />

        {err && <p className="pill red" style={{ marginTop: 12 }}>{err}</p>}

        <div className="row" style={{ marginTop: 16, justifyContent: 'flex-end' }}>
          <button onClick={onClose} disabled={busy}>Cancel</button>
          <button className="primary" onClick={() => void submit()} disabled={busy}>
            {busy ? 'Submitting…' : 'Submit modification'}
          </button>
        </div>
      </div>
    </div>
  );
}
