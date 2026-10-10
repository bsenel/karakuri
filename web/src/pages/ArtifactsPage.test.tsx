import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// vi.mock is hoisted above the imports, so the stub has to be created inside
// the factory and reached through vi.hoisted rather than closed over.
const { get } = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post: vi.fn(), put: vi.fn(), del: vi.fn() } };
});

import { ArtifactsPage } from './ArtifactsPage';

describe('ArtifactsPage', () => {
  // Braces matter: mockReset returns the mock, and vitest runs a function
  // returned from beforeEach as that test's cleanup — it would call get().
  beforeEach(() => {
    get.mockReset();
  });

  it('says it is loading rather than that there are no artifacts', () => {
    // A request that never settles: the table must not claim to be empty.
    get.mockReturnValue(new Promise(() => {}));

    render(<ArtifactsPage />);
    expect(screen.getByRole('status')).toHaveTextContent('Loading…');
    expect(screen.queryByText(/No artifacts/)).not.toBeInTheDocument();
  });

  it('does not report a failed request as an empty list', async () => {
    get.mockRejectedValue(new Error('API 500: boom'));

    render(<ArtifactsPage />);
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('API 500: boom'));
    expect(screen.getByText('Could not load artifacts.')).toBeInTheDocument();
    expect(screen.queryByText(/No artifacts/)).not.toBeInTheDocument();
  });

  it('says the list is empty once the server has answered with none', async () => {
    // The server answers a nil slice as null.
    get.mockResolvedValue(null);

    render(<ArtifactsPage />);
    await waitFor(() => expect(screen.getByText(/No artifacts yet/)).toBeInTheDocument());
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('labels the filter and diff fields and names the per-row diff buttons', async () => {
    const sha = 'abcdef0123456789abcdef0123456789';
    get.mockResolvedValue([
      { sha, objective_id: 'obj-1', agent_id: 'agent-1', created_at: '2026-03-04T12:00:00Z' },
    ]);

    render(<ArtifactsPage />);

    // The visible labels are tied to their fields, so each has a name.
    expect(screen.getByLabelText('Objective ID')).toBeInTheDocument();
    // A bare "A" says nothing in a list of buttons; the name carries the
    // artifact and the side of the diff it fills.
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Use abcdef0123456789 as SHA A' })).toBeInTheDocument(),
    );
    fireEvent.click(screen.getByRole('button', { name: 'Use abcdef0123456789 as SHA A' }));
    fireEvent.click(screen.getByRole('button', { name: 'Use abcdef0123456789 as SHA B' }));

    expect(screen.getByLabelText('SHA A')).toHaveValue(sha);
    expect(screen.getByLabelText('SHA B')).toHaveValue(sha);
  });
});
