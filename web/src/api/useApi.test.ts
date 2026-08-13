import { describe, expect, it } from 'vitest';
import { APIError } from './client';
import { describe as describeError } from './useApi';

// The API answers errors as JSON carrying a message, and that message is the
// whole point of the response — "you can only approve a raise for a subject you
// already hold" tells somebody what to do next, and "API 403" does not.
describe('describe', () => {
  it('reads the message out of a JSON error body', () => {
    const err = new APIError(
      403,
      JSON.stringify({
        error: 'forbidden',
        message: 'you can only approve a raise for a subject you already hold',
      }),
    );
    expect(describeError(err)).toBe(
      'you can only approve a raise for a subject you already hold',
    );
  });

  it('falls back to the error code when there is no message', () => {
    const err = new APIError(409, JSON.stringify({ error: 'conflict' }));
    expect(describeError(err)).toBe('conflict');
  });

  it('shows a plain-text body as it came', () => {
    // Some handlers answer http.Error, which is not JSON.
    expect(describeError(new APIError(400, 'twin is required'))).toBe('twin is required');
  });

  it('says something useful for an empty body', () => {
    expect(describeError(new APIError(500, ''))).toBe('Request failed (500)');
  });

  it('handles what a network failure throws', () => {
    expect(describeError(new TypeError('Failed to fetch'))).toBe('Failed to fetch');
    expect(describeError('something odd')).toBe('something odd');
  });
});
