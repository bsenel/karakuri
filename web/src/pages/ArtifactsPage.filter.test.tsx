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

const artifact = {
  sha: 'abcdef0123456789abcdef',
  objective_id: 'obj-1',
  agent_id: 'agent-1',
  kind: 'plan',
  size: 12,
  created_at: '2026-11-27T10:00:00Z',
};

describe('ArtifactsPage filter', () => {
  // Braces matter: mockReset returns the mock, and vitest runs a function
  // returned from beforeEach as that test's cleanup.
  beforeEach(() => {
    get.mockReset();
  });

  it('clears the old rows and says it is loading while a filter is applied', async () => {
    get.mockResolvedValueOnce([artifact]);

    render(<ArtifactsPage />);
    await waitFor(() => expect(screen.getByText('obj-1')).toBeInTheDocument());

    // The filtered request is still in flight: it never resolves.
    get.mockReturnValue(new Promise(() => {}));
    fireEvent.change(screen.getByLabelText('Objective ID'), { target: { value: 'obj-2' } });
    fireEvent.click(screen.getByText('Apply'));

    expect(screen.getByText('Loading…')).toBeInTheDocument();
    expect(screen.queryByText('obj-1')).not.toBeInTheDocument();
    expect(get).toHaveBeenLastCalledWith('/artifacts?objective=obj-2');
  });

  it('names the objective when a filtered list is empty, rather than saying none exist', async () => {
    get.mockResolvedValueOnce([artifact]);

    render(<ArtifactsPage />);
    await waitFor(() => expect(screen.getByText('obj-1')).toBeInTheDocument());

    get.mockResolvedValue([]);
    fireEvent.change(screen.getByLabelText('Objective ID'), { target: { value: 'obj-2' } });
    fireEvent.click(screen.getByText('Apply'));

    await waitFor(() => expect(screen.getByText(/No artifacts for objective obj-2/)).toBeInTheDocument());
    expect(screen.queryByText(/No artifacts yet/)).not.toBeInTheDocument();
  });
});
