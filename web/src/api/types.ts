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
  | 'completed'
  | 'failed'
  | 'cancelled';

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
  created_at: string;
  updated_at: string;
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
