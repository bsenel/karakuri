import type { ReactNode } from 'react';
import { useAuth } from '@/auth/AuthProvider';

/**
 * RequirePermission renders its children only for a principal holding the
 * action, and explains itself otherwise.
 *
 * It is not a security boundary and does not pretend to be one — every route
 * behind it is enforced by the server, which is the only place a check counts.
 * What it buys is an honest answer in place of a broken page: somebody who
 * reaches /users without auth:read sees why, rather than an empty table and a
 * console full of 403s.
 *
 * The distinction matters when reading this code later. Do not put anything
 * here that the API would hand over anyway.
 */
export function RequirePermission({
  action,
  children,
  what,
}: {
  action: string;
  children: ReactNode;
  /** What the page shows, for the refusal message: "the audit log". */
  what?: string;
}) {
  const { can } = useAuth();
  if (can(action)) return <>{children}</>;
  return (
    <div className="card">
      <h2>Not available to you</h2>
      <p className="muted">
        Viewing {what ?? 'this page'} needs <code>{action}</code>, which none of your roles
        grant. An administrator can change that with <code>krk auth bindings add</code>.
      </p>
    </div>
  );
}
