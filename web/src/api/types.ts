// TypeScript types matching the Go core structs. Names + tags mirror the
// `json:"..."` field tags in internal/core/*; keep these in sync when the Go
// side changes shape.

export interface Twin {
  id: string;
  name: string;
  kind: 'person' | 'team' | 'organization';
  domain: string;
  agents?: unknown[];
  environments?: string[];
  objectives?: string[];
  memory?: Record<string, unknown>;
  children?: string[];
  adapter_bindings?: Record<string, string>;
  created_at: string;
  updated_at: string;
}

export interface Criterion {
  id: string;
  description: string;
  verifier?: string;
  weight?: number;
}

export type ObjectiveStatus =
  | 'pending'
  | 'active'
  // Where a standing objective rests: desired and actual agree, and the
  // supervisor is watching for them to stop agreeing. Deliberately not
  // 'completed', which says the work is over.
  | 'converged'
  // A standing objective the circuit breaker or the stall detector stopped.
  | 'blocked'
  | 'completed'
  | 'failed'
  | 'cancelled';

/** Empty means oneshot — every objective written before standing mode. */
export type ObjectiveMode = '' | 'oneshot' | 'standing';

/**
 * When a standing objective is looked at, and when it is acted on.
 *
 * Two intervals rather than one because the tiers cost wildly different
 * amounts: `sense` is adapter calls, and the reconcile schedule is where the
 * model spend goes.
 */
export interface Cadence {
  sense?: string;
  every?: string;
  cron?: string;
  daily_at?: string;
  timezone?: string;
  resync?: string;
  min_interval?: string;
  quiet?: string[];
}

/** The autonomy ladder. `ceiling` is the rung an objective may never pass. */
export type AutonomyLevel = 'sense' | 'propose' | 'act_with_notice' | 'act';

export interface Autonomy {
  level?: AutonomyLevel;
  ceiling?: AutonomyLevel;
  promote_after?: number;
  demote_on_failure?: boolean;
}

export interface Objective {
  id: string;
  title: string;
  description?: string;
  domain: string;
  twin_id?: string;
  status: ObjectiveStatus;
  success_criteria?: Criterion[];
  constraints?: unknown[];
  max_iterations?: number;
  mode?: ObjectiveMode;
  cadence?: Cadence;
  autonomy?: Autonomy;
  created_at: string;
  updated_at: string;
}

export type ReconcilePhase = 'idle' | 'sensing' | 'reconciling' | 'waiting' | 'paused';
export type ReconcileTrigger = 'schedule' | 'drift' | 'resync' | 'manual' | 'event';

export interface Drift {
  changed: boolean;
  environments?: string[];
  from?: string;
  to?: string;
  blind: boolean;
}

/** One completed pass of the outer loop, cheap or expensive. */
export interface ReconcileOutcome {
  id: string;
  objective_id: string;
  trigger: ReconcileTrigger;
  /** Empty on a sense-only pass, which is the majority and the point. */
  loop_id?: string;
  drift: Drift;
  autonomy?: AutonomyLevel;
  criteria_met: number;
  converged: boolean;
  escalated: boolean;
  checkpoint_id?: string;
  error?: string;
  started_at: string;
  ended_at: string;
}

/** The durable state of one standing objective's control loop. */
export interface ReconcileState {
  objective_id: string;
  twin_id?: string;
  phase: ReconcilePhase;
  paused: boolean;
  paused_reason?: string;
  /** Null means never due on its own — it reconciles only when asked. */
  next_due_at?: string | null;
  next_sense_at?: string | null;
  next_reconcile_at?: string | null;
  last_converged_at?: string | null;
  last_run_at?: string | null;
  last_reconciled_at?: string | null;
  last_trigger?: ReconcileTrigger;
  last_error?: string;
  criteria_met: number;
  score_streak: number;
  consecutive_failures: number;
  autonomy?: AutonomyLevel;
  clean_runs: number;
  holder?: string;
}

export interface ReconcileView {
  state: ReconcileState;
  history: ReconcileOutcome[];
}

export interface ObjectiveTemplate {
  id: string;
  title: string;
  description: string;
  domain: string;
  success_criteria?: Criterion[];
}

export type LoopStep = 'observe' | 'reason' | 'decide' | 'act' | 'verify' | 'learn';

export interface LoopStatus {
  loop_id: string;
  objective_id: string;
  iteration: number;
  paused: boolean;
  completed: boolean;
  checkpoint_id?: string;
  weighted_score?: number;
  last_step?: LoopStep;
}

// CheckpointAction mirrors internal/core/checkpoint.Action — the planner
// draft surfaced to reviewers on a pending checkpoint (Phase 13.5).
export interface CheckpointAction {
  capability: string;
  params?: Record<string, unknown>;
  reason?: string;
  env_id?: string;
}

// CheckpointModifications mirrors internal/core/checkpoint.Modifications
// — the structured edits an operator can submit when resolving with
// decision="modify" (Phase 13.5).
export interface CheckpointModifications {
  removed_actions?: string[];
  added_constraints?: string[];
  revised_confidence?: number;
}

export interface CheckpointDecision {
  choice: string;
  note?: string;
  approver?: string;
  modifications?: CheckpointModifications;
}

export interface Checkpoint {
  id: string;
  objective_id: string;
  agent_id?: string;
  twin_id?: string;
  reason: string;
  summary?: string;
  options?: string[];
  capability?: string;
  confidence?: number;
  actions?: CheckpointAction[];
  audit_event_id?: string;
  context?: Record<string, unknown>;
  status: 'pending' | 'resolved';
  decision?: CheckpointDecision;
  created_at: string;
  resolved_at?: string;
}

// AuditEvent mirrors storage.ToolEvent — the audit log row served by
// GET /api/v1/audit (Phase 13 + Phase 13.5).
export interface AuditEvent {
  id: string;
  objective_id: string;
  agent_id?: string;
  capability?: string;
  adapter?: string;
  success: boolean;
  confidence?: number;
  kind: 'execute' | 'escalation' | 'approval' | 'modification' | 'rejection' | string;
  escalation_reason?: string;
  approver?: string;
  bounds_violation?: boolean;
  payload_json?: string;
  created_at: string;
}

export interface Artifact {
  sha: string;
  objective_id: string;
  agent_id: string;
  kind?: string;
  size?: number;
  mime?: string;
  created_at: string;
}

export interface MemoryEntry {
  id: string;
  agent_id: string;
  twin_id?: string;
  tier: 'working' | 'episodic' | 'semantic' | 'procedural';
  domain?: string;
  content: string;
  confidence?: number;
  sources?: string[];
  created_at: string;
  expires_at?: string;
}

export interface MemoryQuery {
  agent_id?: string;
  twin_id?: string;
  tiers?: string[];
  query?: string;
  top_k?: number;
  domain?: string;
}

export interface HealthAdapter {
  slot: string;
  instance: string;
  type: string;
  active: boolean;
  is_default: boolean;
}

export interface HealthResponse {
  status: string;
  adapters: HealthAdapter[];
  providers: Record<string, boolean>;
  exporters?: string[] | null;
  git?: { repo_path: string; worktree_manager: boolean };
}

// SSE event envelope. Type is enumerated in internal/core/event/event.go.
export interface SSEEvent {
  type: string;
  objective_id?: string;
  twin_id?: string;
  loop_id?: string;
  payload?: Record<string, unknown>;
  timestamp: string;
}

export interface Domain {
  id: string;
  name: string;
  description: string;
  version: string;
}

export interface ConformanceResult {
  check: string;
  passed: boolean;
  message: string;
}
