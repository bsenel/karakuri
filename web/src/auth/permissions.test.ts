import { describe, expect, it } from 'vitest';
import { Actions, landingFor, navigation, visibleNavigation } from './permissions';

// A permission set, in the shape /auth/me returns.
const holding = (...actions: string[]) => (action: string) => actions.includes(action);

describe('visibleNavigation', () => {
  it('offers a viewer only what a viewer can open', () => {
    // The built-in viewer role, as internal/auth/roles.go defines it.
    const can = holding(
      Actions.twinRead,
      Actions.objectiveRead,
      Actions.checkpointRead,
      Actions.memoryRead,
      Actions.artifactRead,
      Actions.containerRead,
      Actions.quotaRead,
      Actions.quotaRequest,
    );
    const labels = visibleNavigation(can).map((e) => e.label);

    expect(labels).toContain('Objectives');
    expect(labels).toContain('Quota');
    // A viewer holds neither audit:read nor cost:read, and the menu should not
    // offer two pages that answer 403.
    expect(labels).not.toContain('Audit');
    expect(labels).not.toContain('Cost');
    expect(labels).not.toContain('Users');
  });

  it('offers an administrator everything', () => {
    const all = navigation.map((e) => e.action).filter(Boolean) as string[];
    const labels = visibleNavigation(holding(...all)).map((e) => e.label);
    expect(labels).toEqual(navigation.map((e) => e.label));
  });

  it('keeps the pages that need no permission', () => {
    // Health is reachable by anybody signed in: an operator should be able to
    // see the server is up without holding anything else.
    const labels = visibleNavigation(() => false).map((e) => e.label);
    expect(labels).toEqual(['Health']);
  });
});

describe('landingFor', () => {
  it('sends somebody to a page they can actually open', () => {
    // A cost-only auditor landing on /objectives would meet a 403 as their
    // first impression of a system that is working correctly.
    expect(landingFor(holding(Actions.costRead))).toBe('/cost');
  });

  it('falls back to health rather than nowhere', () => {
    expect(landingFor(() => false)).toBe('/health');
  });
});
