import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { ModifyCheckpointDialog } from './ModifyCheckpointDialog';
import type { Checkpoint } from '@/api/types';

function checkpoint(over: Partial<Checkpoint> = {}): Checkpoint {
  return {
    id: 'cp-1',
    objective_id: 'obj-1',
    reason: 'low confidence',
    status: 'pending',
    created_at: '2026-03-04T12:00:00Z',
    actions: [{ capability: 'git.push', reason: 'publish the branch' }],
    ...over,
  };
}

describe('ModifyCheckpointDialog', () => {
  it('is a modal dialog named by its heading', () => {
    render(<ModifyCheckpointDialog checkpoint={checkpoint()} onClose={vi.fn()} onSubmit={vi.fn()} />);

    // Without the role and the name a screen reader lands in an unnamed group
    // of fields with no hint that the page behind it is inert.
    const dialog = screen.getByRole('dialog', { name: 'Modify checkpoint' });
    expect(dialog).toHaveAttribute('aria-modal', 'true');
  });

  it('names every field, not only the ones a placeholder describes', () => {
    render(<ModifyCheckpointDialog checkpoint={checkpoint()} onClose={vi.fn()} onSubmit={vi.fn()} />);

    // The headings above the fields are not labels, and a placeholder
    // disappears on the first keystroke.
    expect(screen.getByRole('textbox', { name: 'Constraints' })).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Note' })).toBeInTheDocument();
    expect(screen.getByRole('spinbutton', { name: 'Confidence floor (optional)' })).toBeInTheDocument();
  });

  it('closes on Escape and on nothing else', () => {
    const onClose = vi.fn();
    render(<ModifyCheckpointDialog checkpoint={checkpoint()} onClose={onClose} onSubmit={vi.fn()} />);

    fireEvent.keyDown(window, { key: 'Enter' });
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('announces a failed submit and stays open', async () => {
    const onClose = vi.fn();
    const onSubmit = vi.fn().mockRejectedValue(new Error('checkpoint already resolved'));
    render(<ModifyCheckpointDialog checkpoint={checkpoint()} onClose={onClose} onSubmit={onSubmit} />);

    fireEvent.click(screen.getByRole('button', { name: 'Submit modification' }));

    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('checkpoint already resolved'));
    expect(onClose).not.toHaveBeenCalled();
  });
});
