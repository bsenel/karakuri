import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { Fetched } from '@/api/useApi';
import { ChartState, formatMoney } from './CostPage';

// A chart card used to read "Nothing recorded" while its request was in flight
// and after it failed; neither is a claim that nothing was spent.
describe('ChartState', () => {
  const fetched = (over: Partial<Fetched<unknown>>): Fetched<unknown> => ({
    data: null,
    loading: false,
    error: null,
    reload: vi.fn(),
    ...over,
  });

  it('says it is loading rather than that nothing was recorded', () => {
    render(
      <ChartState fetched={fetched({ loading: true })} empty emptyText="Nothing to attribute.">
        <p>chart</p>
      </ChartState>,
    );
    expect(screen.getByRole('status')).toHaveTextContent('Loading…');
    expect(screen.queryByText('Nothing to attribute.')).not.toBeInTheDocument();
  });

  it('shows the message of a failed request with a retry', () => {
    const reload = vi.fn();
    render(
      <ChartState
        fetched={fetched({ error: 'cost:read is not granted', reload })}
        empty
        emptyText="Nothing to attribute."
      >
        <p>chart</p>
      </ChartState>,
    );
    expect(screen.getByRole('alert')).toHaveTextContent('cost:read is not granted');
    expect(screen.queryByText('Nothing to attribute.')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it('says nothing was recorded once the answer is known to be empty', () => {
    render(
      <ChartState fetched={fetched({ data: [] })} empty emptyText="Nothing to attribute.">
        <p>chart</p>
      </ChartState>,
    );
    expect(screen.getByText('Nothing to attribute.')).toBeInTheDocument();
    expect(screen.queryByText('chart')).not.toBeInTheDocument();
  });

  it('keeps a drawn chart up while a live update refetches', () => {
    render(
      <ChartState
        fetched={fetched({ data: [1], loading: true })}
        empty={false}
        emptyText="Nothing to attribute."
      >
        <p>chart</p>
      </ChartState>,
    );
    expect(screen.getByText('chart')).toBeInTheDocument();
    expect(screen.queryByText('Loading…')).not.toBeInTheDocument();
  });
});

// The ledger stores whole currency units and never learns which currency, so
// the formatter's job is to be readable without inventing a symbol.
describe('formatMoney', () => {
  it('shows enough decimals for a single model call', () => {
    // A million tokens at $15/M is 15; one call is very much less.
    expect(formatMoney(15)).toBe('15.00');
    expect(formatMoney(0.0234)).toBe('0.0234');
  });

  it('says "small" rather than rounding a real cost to zero', () => {
    // Rounding this to "0.00" would say the call was free, which is a
    // different claim from "less than we can show".
    expect(formatMoney(0.00001)).toBe('<0.0001');
  });

  it('shows a true zero plainly', () => {
    // Zero is a legitimate answer — an unpriced model, or no rate table at all
    // — and it should not read as a rounding artefact.
    expect(formatMoney(0)).toBe('0');
  });
});
