import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// vi.mock is hoisted above the imports, so the stub has to be created inside
// the factory and reached through vi.hoisted rather than closed over.
const { get, post } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }));
vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client');
  return { ...actual, api: { get, post, put: vi.fn(), del: vi.fn() } };
});
// jsdom has no EventSource, and the timeline is not what these tests are about.
vi.mock('@/api/sse', () => ({ streamObjective: () => ({ close: () => {} }) }));
// The standing panel makes its own requests; it has its own test.
vi.mock('@/components/StandingPanel', () => ({ StandingPanel: () => null }));

import { ObjectiveDetailPage } from './ObjectiveDetailPage';
import type { Criterion } from '@/api/types';

// The page links back to the list, so it needs a router.
const renderPage = () => render(<MemoryRouter><ObjectiveDetailPage /></MemoryRouter>);

const objective = { id: 'o1', title: 'Ship the docs', domain: 'software', status: 'active', created_at: '', updated_at: '' };

describe('ObjectiveDetailPage', () => {
  // Braces matter: mockReset returns the mock, and vitest runs a function
  // returned from beforeEach as that test's cleanup — it would call get().
  beforeEach(() => {
    get.mockReset();
    post.mockReset();
  });

  it('announces that it is loading', () => {
    // Never resolves: the page is still waiting on its first answer.
    get.mockReturnValue(new Promise(() => {}));

    renderPage();
    expect(screen.getByRole('status')).toHaveTextContent('Loading…');
  });

  it('says the objective could not be loaded, with a way back and a retry', async () => {
    get.mockRejectedValue(new Error('API 404: objective not found'));

    renderPage();
    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent(
        'Could not load this objective: Error: API 404: objective not found',
      ),
    );
    expect(screen.getByRole('link', { name: '← Objectives' })).toBeInTheDocument();

    get.mockResolvedValue(objective);
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Ship the docs' })).toBeInTheDocument());
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('keeps the objective on screen when the loop fails to start', async () => {
    get.mockResolvedValue(objective);
    post.mockRejectedValue(new Error('API 500: boom'));

    renderPage();
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Ship the docs' })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: 'Start loop' }));

    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('API 500: boom'));
    expect(screen.getByRole('heading', { name: 'Ship the docs' })).toBeInTheDocument();
  });

  it('announces the run status once the loop reports it', async () => {
    const status = { loop_id: 'l1', objective_id: 'o1', iteration: 2, paused: false, completed: true };
    // The page asks for the objective and then polls the loop; answer each by path.
    get.mockImplementation((path: string) => Promise.resolve(path.startsWith('/loops/') ? status : objective));
    post.mockResolvedValue({ loop_id: 'l1' });

    renderPage();
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Ship the docs' })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: 'Start loop' }));

    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('iteration 2 · completed'));
  });
});

// Phase 41 step 6: the success criteria panel shows what verify settled about
// each criterion, the same states the digest and `krk objective criteria` show.
describe('ObjectiveDetailPage success criteria', () => {
  beforeEach(() => {
    get.mockReset();
    post.mockReset();
  });

  const withCriteria = (...success_criteria: Criterion[]) => ({ ...objective, success_criteria });

  // Each criterion is one item of the panel's list, so a state is read beside
  // its own description and never beside another's.
  const criterion = (description: string) => {
    const item = screen.getAllByRole('listitem').find((li) => li.textContent?.includes(description));
    if (!item) throw new Error(`no list item for criterion "${description}"`);
    return item;
  };

  // "met" as a word of its own, so "not_met" does not count as met.
  const met = /\bmet\b/;

  it('shows a criterion verify settled met', async () => {
    get.mockResolvedValue(withCriteria(
      { id: 'pr-open', description: 'an open PR with tests', weight: 1, state: 'met', met: true },
    ));

    renderPage();
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Success criteria' })).toBeInTheDocument());
    expect(criterion('an open PR with tests')).toHaveTextContent(met);
  });

  it('shows a criterion verify settled not_met', async () => {
    get.mockResolvedValue(withCriteria(
      { id: 'ci-green', description: 'CI is green on the PR head', weight: 1, state: 'not_met', met: false },
    ));

    renderPage();
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Success criteria' })).toBeInTheDocument());
    const item = criterion('CI is green on the PR head');
    expect(item).toHaveTextContent('not_met');
    expect(item).not.toHaveTextContent(met);
  });

  it('shows an inconclusive criterion with its reason', async () => {
    get.mockResolvedValue(withCriteria(
      { id: 'ci-green', description: 'CI is green on the PR head', weight: 1, state: 'inconclusive', reason: 'not_run', met: false },
      { id: 'coverage', description: 'coverage holds', weight: 1, state: 'inconclusive', reason: 'unsupported', met: false },
    ));

    renderPage();
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Success criteria' })).toBeInTheDocument());
    expect(criterion('CI is green on the PR head')).toHaveTextContent('inconclusive: not_run');
    expect(criterion('coverage holds')).toHaveTextContent('inconclusive: unsupported');
  });

  it('shows a criterion whose verifier never ran as inconclusive: not_run, never met', async () => {
    // No state at all: no verify step has settled it. That is not a pass, the
    // way Gherkin's undefined step once was.
    get.mockResolvedValue(withCriteria(
      { id: 'ci-green', description: 'CI is green on the PR head', verifier: 'software.verify.ci_green', weight: 1, met: false },
    ));

    renderPage();
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Success criteria' })).toBeInTheDocument());
    const item = criterion('CI is green on the PR head');
    expect(item).toHaveTextContent('inconclusive: not_run');
    expect(item).not.toHaveTextContent(met);
  });

  it('shows optional weighted criteria as progress apart from the gate, not as a failure', async () => {
    get.mockResolvedValue(withCriteria(
      { id: 'pr-open', description: 'an open PR with tests', weight: 1, state: 'met', met: true },
      { id: 'docs', description: 'the docs mention the change', weight: 0.7, required: false, state: 'met', met: true },
      { id: 'changelog', description: 'the changelog has a line', weight: 0.3, required: false, state: 'not_met', met: false },
    ));

    renderPage();
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Success criteria' })).toBeInTheDocument());
    // The weight the optional criteria earned out of their total, as the
    // digest renders it.
    expect(screen.getByText(/0\.7 of 1\.0/)).toBeInTheDocument();
    // An expected partial score is not a failure: the optional criterion
    // nobody needed is not shown as not_met.
    expect(criterion('the changelog has a line')).not.toHaveTextContent('not_met');
    // The required criterion keeps its own state beside the progress.
    expect(criterion('an open PR with tests')).toHaveTextContent(met);
  });

  // Phase 41 step 7: the person sees the baseline value beside the target,
  // because a baseline taken from a start that was already bad is met by
  // construction. baseline_value is the frozen number; the API leaves it out
  // for at_start until it has been measured.
  it('shows a frozen at_start baseline with its unit beside the target', async () => {
    get.mockResolvedValue(withCriteria(
      {
        id: 'p95-regress', description: 'search p95 latency stays within 10% of the start', kind: 'measured',
        verifier: 'ci.latency', metric: 'p95_latency', op: '<=', target: 10, unit: 'ms',
        baseline: 'at_start', baseline_value: 212.5, change: 'percent', weight: 1, state: 'met', met: true,
      },
    ));

    renderPage();
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Success criteria' })).toBeInTheDocument());
    const item = criterion('search p95 latency stays within 10% of the start');
    expect(item).toHaveTextContent('p95_latency <= 10');
    expect(item).toHaveTextContent('baseline 212.5 ms');
    expect(item).toHaveTextContent('at_start');
    expect(item).not.toHaveTextContent(/fixed/);
  });

  it('shows a baseline fixed by hand with its unit beside the target', async () => {
    get.mockResolvedValue(withCriteria(
      {
        id: 'bundle-growth', description: 'the bundle grows by at most 20 kB', kind: 'measured',
        verifier: 'ci.bundle', metric: 'bundle_size', op: '<=', target: 20, unit: 'kB',
        baseline: 300, baseline_value: 300, change: 'absolute', weight: 1, state: 'not_met', met: false,
      },
    ));

    renderPage();
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Success criteria' })).toBeInTheDocument());
    const item = criterion('the bundle grows by at most 20 kB');
    expect(item).toHaveTextContent('bundle_size <= 20');
    expect(item).toHaveTextContent('baseline 300 kB');
    expect(item).toHaveTextContent(/fixed/);
    expect(item).not.toHaveTextContent('at_start');
  });

  it('says an at_start baseline has not been measured yet rather than showing a value', async () => {
    get.mockResolvedValue(withCriteria(
      {
        id: 'p99-regress', description: 'search p99 latency stays within 15% of the start', kind: 'measured',
        verifier: 'ci.latency', metric: 'p99_latency', op: '<=', target: 15, unit: 'ms',
        baseline: 'at_start', change: 'percent', weight: 1, state: 'inconclusive', reason: 'no_baseline', met: false,
      },
    ));

    renderPage();
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Success criteria' })).toBeInTheDocument());
    const item = criterion('search p99 latency stays within 15% of the start');
    expect(item).toHaveTextContent('inconclusive: no_baseline');
    expect(item).toHaveTextContent('p99_latency <= 15');
    expect(item).toHaveTextContent('at_start');
    expect(item).toHaveTextContent(/not measured/);
    // No number stands in for a baseline nobody measured.
    expect(item).not.toHaveTextContent(/baseline \d/);
  });

  it('shows nothing about a baseline on a criterion that has none', async () => {
    get.mockResolvedValue(withCriteria(
      {
        id: 'p50', description: 'search p50 latency is at most 80 ms', kind: 'measured',
        verifier: 'ci.latency', metric: 'p50_latency', op: '<=', target: 80, unit: 'ms',
        weight: 1, state: 'met', met: true,
      },
      { id: 'pr-open', description: 'an open PR with tests', weight: 1, state: 'met', met: true },
    ));

    renderPage();
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Success criteria' })).toBeInTheDocument());
    expect(criterion('search p50 latency is at most 80 ms')).not.toHaveTextContent(/baseline/);
    expect(criterion('search p50 latency is at most 80 ms')).toHaveTextContent(met);
    expect(criterion('an open PR with tests')).not.toHaveTextContent(/baseline/);
  });
});
