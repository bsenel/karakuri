import { render, screen, waitFor } from '@testing-library/react';
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
});
