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

export interface Checkpoint {
  id: string;
  objective_id: string;
  agent_id: string;
  twin_id?: string;
  reason: string;
  context?: Record<string, unknown>;
  options?: string[];
  status: 'pending' | 'resolved';
  decision?: string;
  notes?: string;
  created_at: string;
  resolved_at?: string;
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
