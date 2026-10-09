import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// vi.mock is hoisted above the imports, so the stub has to be created inside
// the factory and reached through vi.hoisted rather than closed over.
const { post } = vi.hoisted(() => ({ post: vi.fn() }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get: vi.fn(), post, put: vi.fn(), del: vi.fn() } };
});

import { MemoryPage } from './MemoryPage';

const recall = () => fireEvent.click(screen.getByRole('button', { name: 'Recall' }));

describe('MemoryPage', () => {
  // Braces matter: mockReset returns the mock, and vitest runs a function
  // returned from beforeEach as that test's cleanup — it would call post().
  beforeEach(() => {
    post.mockReset();
  });

  it('asks for a query before one has been run', () => {
    render(<MemoryPage />);
    expect(screen.getByText('No results yet — run a recall query.')).toBeInTheDocument();
  });

  it('says nothing matched, rather than asking for a query that was just run', async () => {
    // The server answers a nil slice as null.
    post.mockResolvedValue(null);

    render(<MemoryPage />);
    recall();
    await waitFor(() => expect(screen.getByText(/No memories matched/)).toBeInTheDocument());
    expect(screen.queryByText(/run a recall query/)).not.toBeInTheDocument();
  });

  it('does not report a failed recall as no results yet', async () => {
    post.mockRejectedValue(new Error('API 500: boom'));

    render(<MemoryPage />);
    recall();
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('API 500: boom'));
    expect(screen.getByText(/The recall failed/)).toBeInTheDocument();
    expect(screen.queryByText(/run a recall query/)).not.toBeInTheDocument();
  });

  it('says it is recalling while the request is pending', async () => {
    post.mockReturnValue(new Promise(() => {}));

    render(<MemoryPage />);
    recall();
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('Recalling…'));
  });
});
