import { Link, useParams } from 'react-router-dom';
import { useFetch } from '@/api/useApi';
import { Actions } from '@/auth/permissions';
import { RequirePermission } from '@/components/RequirePermission';

interface AuditEvent {
  id: string;
  objective_id: string;
  agent_id?: string;
  capability?: string;
  adapter?: string;
  success: boolean;
  confidence?: number;
  payload_json?: string;
  kind: string;
  escalation_reason?: string;
  approver?: string;
  bounds_violation?: boolean;
  created_at: string;
}

export function AuditEventPage() {
  return (
    <RequirePermission action={Actions.auditRead} what="the audit log">
      <AuditEvent />
    </RequirePermission>
  );
}

/**
 * One audit row, reached by link.
 *
 * The list page expands a row inline, which is the right interaction while
 * scanning. This exists for the other case: a link in a ticket, a bookmark, a
 * row that has since scrolled past the page limit. Filtering the list to find
 * it would not work — the row may be older than the limit — which is why
 * GET /audit/{id} exists rather than being a query parameter.
 */
function AuditEvent() {
  const { id = '' } = useParams();
  const event = useFetch<AuditEvent>(`/audit/${encodeURIComponent(id)}`, [id]);

  if (event.loading) return <p className="muted">Loading…</p>;
  if (event.error) {
    return (
      <div className="card">
        <h2>Not found</h2>
        <p className="muted">
          No audit event with this ID, or none you may read. The two answer alike on
          purpose — an audit log that distinguished them would tell a prober which IDs
          exist.
        </p>
        <Link to="/audit">Back to the audit log</Link>
      </div>
    );
  }
  const e = event.data;
  if (!e) return null;

  let payload: unknown = null;
  try {
    payload = e.payload_json ? JSON.parse(e.payload_json) : null;
  } catch {
    payload = e.payload_json;
  }

  return (
    <>
      <h1>
        {e.kind}
        {e.bounds_violation && <span className="pill red">bounds violation</span>}
      </h1>
      <p className="muted small">
        <span className="mono">{e.id}</span> · {new Date(e.created_at).toLocaleString()}
      </p>

      <div className="card">
        <dl>
          <Field label="Objective">
            <Link to={`/objectives/${e.objective_id}`}>{e.objective_id}</Link>
          </Field>
          {e.agent_id && <Field label="Agent">{e.agent_id}</Field>}
          {e.capability && <Field label="Capability">{e.capability}</Field>}
          {e.adapter && <Field label="Adapter">{e.adapter}</Field>}
          <Field label="Outcome">
            <span className={e.success ? 'pill green' : 'pill red'}>
              {e.success ? 'succeeded' : 'failed'}
            </span>
          </Field>
          {e.confidence !== undefined && e.confidence > 0 && (
            <Field label="Confidence">{e.confidence.toFixed(2)}</Field>
          )}
          {e.escalation_reason && <Field label="Escalated because">{e.escalation_reason}</Field>}
          {e.approver && <Field label="Approved by">{e.approver}</Field>}
        </dl>
      </div>

      {payload != null && (
        <>
          <h2>Payload</h2>
          <pre className="card mono small" style={{ overflowX: 'auto' }}>
            {typeof payload === 'string' ? payload : JSON.stringify(payload, null, 2)}
          </pre>
        </>
      )}

      <p style={{ marginTop: 16 }}>
        <Link to={`/audit?objective=${encodeURIComponent(e.objective_id)}`}>
          Everything else on this objective
        </Link>
      </p>
    </>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="row" style={{ alignItems: 'baseline', marginBottom: 6 }}>
      <dt className="muted small" style={{ minWidth: 140 }}>
        {label}
      </dt>
      <dd style={{ margin: 0 }}>{children}</dd>
    </div>
  );
}
