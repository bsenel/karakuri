import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { DataTable, type Column } from './DataTable';

interface Row {
  id: string;
  name: string;
}

const columns: Column<Row>[] = [{ header: 'Name', render: (r) => r.name }];
const keyOf = (r: Row) => r.id;

describe('DataTable', () => {
  it('announces a failed load as an alert', () => {
    render(<DataTable columns={columns} rows={[]} keyOf={keyOf} error="forbidden" />);

    // A red paragraph is only red; without the role a screen reader user is
    // left on a page that silently has no table.
    expect(screen.getByRole('alert')).toHaveTextContent('forbidden');
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
  });

  it('announces loading as a status', () => {
    render(<DataTable columns={columns} rows={[]} keyOf={keyOf} loading />);

    expect(screen.getByRole('status')).toHaveTextContent('Loading…');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('renders rows under header cells, with neither role', () => {
    render(<DataTable columns={columns} rows={[{ id: 'a', name: 'alpha' }]} keyOf={keyOf} />);

    expect(screen.getByRole('columnheader', { name: 'Name' })).toBeInTheDocument();
    expect(screen.getByRole('cell', { name: 'alpha' })).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });
});
