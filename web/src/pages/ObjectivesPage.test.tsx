import { render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// vi.mock is hoisted above the imports, so the stub has to be created inside
// the factory and reached through vi.hoisted rather than closed over.
const { get } = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post: vi.fn(), put: vi.fn(), del: vi.fn() } };
});

import { APIError } from '@/api/client';
import { ObjectivesPage } from './ObjectivesPage';

describe('ObjectivesPage', () => {
  // Braces matter: mockReset returns the mock, and vitest runs a function
  // returned from beforeEach as that test's cleanup — it would call get().
  beforeEach(() => {
    get.mockReset();
  });

  it('says it is loading rather than that there are no objectives', () => {
    // Never resolves: the page is still waiting on its first answer.
    get.mockReturnValue(new Promise(() => {}));

    render(<ObjectivesPage />);
    expect(screen.getByText('Loading…')).toBeInTheDocument();
    expect(screen.queryByText(/No objectives yet/)).not.toBeInTheDocument();
  });

  it('says how to create the first objective once the list is known to be empty', async () => {
    // The objectives, the templates and the twins are all empty.
    get.mockResolvedValue([]);

    render(<ObjectivesPage />);
    await waitFor(() => expect(screen.getByText(/No objectives yet/)).toBeInTheDocument());
    expect(screen.getByText('krk objective create')).toBeInTheDocument();
    expect(screen.queryByText('Loading…')).not.toBeInTheDocument();
    expect(screen.queryByText('Retry')).not.toBeInTheDocument();
  });

  it('shows the message of a failed request with a retry, and does not claim the list is empty', async () => {
    get.mockRejectedValue(new APIError(403, '{"message":"objective:read is not granted"}'));

    render(<ObjectivesPage />);
    await waitFor(() =>
      expect(screen.getByText('objective:read is not granted')).toBeInTheDocument(),
    );
    expect(screen.getByText('Retry')).toBeInTheDocument();
    expect(screen.queryByText(/No objectives yet/)).not.toBeInTheDocument();
    expect(screen.queryByText('Loading…')).not.toBeInTheDocument();
  });

  it('ties every label of the create form to its field', async () => {
    get.mockResolvedValue([]);

    render(<ObjectivesPage />);
    await waitFor(() => expect(screen.getByText(/No objectives yet/)).toBeInTheDocument());

    // A <label> beside a field names nothing until htmlFor points at it: a
    // screen reader announced five unnamed text fields and combo boxes.
    expect(screen.getByRole('textbox', { name: 'Title' })).toBeRequired();
    expect(screen.getByRole('spinbutton', { name: 'Max iterations' })).toHaveValue(20);
    expect(screen.getByRole('combobox', { name: 'Twin' })).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Domain' })).toHaveValue('software');
    expect(screen.getByRole('combobox', { name: 'Template' })).toBeInTheDocument();
  });
});
