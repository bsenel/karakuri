import { describe, expect, it } from 'vitest';
import { formatMoney } from './CostPage';

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
