import { useState } from 'react';
import { NavLink, Route, Routes } from 'react-router-dom';
import { Actions } from '@/auth/permissions';
import { RequirePermission } from '@/components/RequirePermission';
import { QuotaUsage } from './quota/QuotaUsage';
import { QuotaRequests } from './quota/QuotaRequests';
import { QuotaSettings } from './quota/QuotaSettings';

/**
 * Quota has three views that answer three different questions, and they are
 * tabs rather than three top-level pages because an operator moves between them
 * in one sitting: what is this twin using, who has asked for more, and what are
 * the limits anyway.
 */
export function QuotaPage() {
  return (
    <RequirePermission action={Actions.quotaRead} what="quota usage and limits">
      <>
        <h1>Quota</h1>
        <nav className="row" style={{ marginBottom: 16 }}>
          <NavLink to="/quota" end className={({ isActive }) => (isActive ? 'active' : '')}>
            Usage
          </NavLink>
          <NavLink to="/quota/requests" className={({ isActive }) => (isActive ? 'active' : '')}>
            Requests
          </NavLink>
          <NavLink to="/quota/settings" className={({ isActive }) => (isActive ? 'active' : '')}>
            Limits
          </NavLink>
        </nav>
        <Routes>
          <Route index element={<QuotaUsage />} />
          <Route path="requests" element={<QuotaRequests />} />
          <Route path="settings" element={<QuotaSettings />} />
        </Routes>
      </>
    </RequirePermission>
  );
}

/** Shared by the tabs: a twin picker, since every usage question is per twin. */
export function TwinPicker({
  value,
  onChange,
}: {
  value: string;
  onChange: (id: string) => void;
}) {
  const [manual, setManual] = useState(false);
  if (manual) {
    return (
      <input
        placeholder="twin id"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-label="Twin"
      />
    );
  }
  return (
    <button className="linklike small" onClick={() => setManual(true)}>
      enter a twin id
    </button>
  );
}
