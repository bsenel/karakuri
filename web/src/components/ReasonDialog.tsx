import { useEffect, useId, useState } from 'react';

// ReasonDialog asks why before a stop, a pause or a stop-all goes out (Phase
// 37 step 6). Confirm stays disabled until a reason is written: a stop nobody
// can explain later is the one the record most needed.
export interface ReasonDialogProps {
  title: string;
  confirmLabel: string;
  onClose: () => void;
  onConfirm: (reason: string) => Promise<void>;
}

export function ReasonDialog({ title, confirmLabel, onClose, onConfirm }: ReasonDialogProps) {
  const titleID = useId();
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  // Escape closes the dialog, the same as Cancel and a click on the backdrop.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  const submit = async () => {
    setBusy(true);
    setErr(null);
    try {
      await onConfirm(reason.trim());
      onClose();
    } catch (e) {
      setErr(String(e));
      setBusy(false);
    }
  };

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleID}
        onClick={(e) => e.stopPropagation()}
      >
        <h2 id={titleID}>{title}</h2>
        <textarea
          aria-label="Reason"
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          rows={3}
          placeholder="why — kept on the record of each run it touches"
        />
        {err && <p className="pill red" role="alert" style={{ marginTop: 12 }}>{err}</p>}
        <div className="row" style={{ marginTop: 16, justifyContent: 'flex-end' }}>
          <button onClick={onClose} disabled={busy}>Cancel</button>
          <button className="primary" onClick={() => void submit()} disabled={busy || reason.trim() === ''}>
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
