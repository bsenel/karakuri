import type { ReactNode } from 'react';

/**
 * One column of a table: a heading and how to render a cell.
 *
 * `render` returns a node rather than a string so a cell can carry a link or a
 * pill without every table growing a special case for it.
 */
export interface Column<T> {
  header: string;
  render: (row: T) => ReactNode;
  /** Right-align numbers, which is the only alignment worth the option. */
  numeric?: boolean;
}

/**
 * DataTable renders rows, or says why there are none.
 *
 * The empty state is the reason this exists rather than each page writing its
 * own <table>. "No rows" and "no rows *you* can see" and "not loaded yet" look
 * identical in a bare table, and on a system whose whole point is per-tenant
 * filtering, that ambiguity is exactly the thing an operator needs resolved.
 */
export function DataTable<T>({
  columns,
  rows,
  keyOf,
  loading,
  error,
  empty,
}: {
  columns: Column<T>[];
  rows: T[];
  keyOf: (row: T) => string;
  loading?: boolean;
  error?: string | null;
  /** Shown when there are no rows and nothing went wrong. */
  empty?: ReactNode;
}) {
  if (error) {
    return (
      <div className="card">
        <p className="error">{error}</p>
      </div>
    );
  }
  if (loading) return <p className="muted">Loading…</p>;
  if (rows.length === 0) {
    return <p className="muted">{empty ?? 'Nothing here.'}</p>;
  }

  return (
    <div className="tablewrap">
      <table>
        <thead>
          <tr>
            {columns.map((c) => (
              <th key={c.header} style={c.numeric ? { textAlign: 'right' } : undefined}>
                {c.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={keyOf(row)}>
              {columns.map((c) => (
                <td key={c.header} style={c.numeric ? { textAlign: 'right' } : undefined}>
                  {c.render(row)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
