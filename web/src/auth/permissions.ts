/**
 * The permissions the interface reasons about, named once.
 *
 * They are the same strings `internal/auth/catalog.go` defines, and a typo here
 * is a menu item that never appears — so they live in one place rather than
 * inline at every call site.
 */
export const Actions = {
  twinRead: 'twin:read',
  objectiveRead: 'objective:read',
  checkpointRead: 'checkpoint:read',
  memoryRead: 'memory:read',
  artifactRead: 'artifact:read',
  auditRead: 'audit:read',
  authRead: 'auth:read',
  authWrite: 'auth:write',
  containerRead: 'container:read',
  containerWrite: 'container:write',
  quotaRead: 'quota:read',
  quotaAdmin: 'quota:admin',
  quotaRequest: 'quota:request',
  quotaApprove: 'quota:approve',
  costRead: 'cost:read',
} as const;

export type Action = (typeof Actions)[keyof typeof Actions];

/**
 * A page the navigation can offer, and the permission it needs.
 *
 * Hiding a link is a courtesy, not a control. The server refuses the request
 * either way — what this prevents is a menu full of things that answer 403,
 * which trains people to ignore errors. Anything genuinely secret has to be
 * absent from the API's response, not from the menu.
 */
export interface NavEntry {
  to: string;
  label: string;
  /** Omitted means every authenticated principal may see it. */
  action?: Action;
}

export const navigation: NavEntry[] = [
  { to: '/twins', label: 'Twins', action: Actions.twinRead },
  { to: '/objectives', label: 'Objectives', action: Actions.objectiveRead },
  { to: '/checkpoints', label: 'Checkpoints', action: Actions.checkpointRead },
  { to: '/cost', label: 'Cost', action: Actions.costRead },
  { to: '/quota', label: 'Quota', action: Actions.quotaRead },
  { to: '/orgs', label: 'Organisations', action: Actions.containerRead },
  { to: '/users', label: 'Users', action: Actions.authRead },
  { to: '/audit', label: 'Audit', action: Actions.auditRead },
  { to: '/memory', label: 'Memory', action: Actions.memoryRead },
  { to: '/artifacts', label: 'Artifacts', action: Actions.artifactRead },
  { to: '/health', label: 'Health' },
];

/** visibleNavigation is the subset of navigation a principal may reach. */
export function visibleNavigation(can: (action: string) => boolean): NavEntry[] {
  return navigation.filter((entry) => !entry.action || can(entry.action));
}

/**
 * landingFor picks where to send somebody who has just signed in.
 *
 * A fixed default sends a viewer with no objective access straight to a page
 * that 403s, which is a poor first impression of a system that is working
 * correctly. This picks the first page they can actually open.
 */
export function landingFor(can: (action: string) => boolean): string {
  return visibleNavigation(can)[0]?.to ?? '/health';
}
