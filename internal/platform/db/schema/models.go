package schema

import "time"

type TwinModel struct {
	ID                  string    `gorm:"primaryKey;column:id"`
	Name                string    `gorm:"column:name;not null"`
	Kind                string    `gorm:"column:kind;not null"`
	Domain              string    `gorm:"column:domain;not null"`
	AgentsJSON          string    `gorm:"column:agents_json;not null;default:'[]'"`
	EnvsJSON            string    `gorm:"column:envs_json;not null;default:'[]'"`
	ObjectivesJSON      string    `gorm:"column:objectives_json;not null;default:'[]'"`
	MemoryJSON          string    `gorm:"column:memory_json;not null;default:'{}'"`
	ChildrenJSON        string    `gorm:"column:children_json;not null;default:'[]'"`
	AdapterBindingsJSON string    `gorm:"column:adapter_bindings_json;not null;default:'{}'"`
	OwnerID             string    `gorm:"column:owner_id;index;default:''"`
	CreatedAt           time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt           time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (TwinModel) TableName() string { return "twins" }

type ObjectiveModel struct {
	ID                    string     `gorm:"primaryKey;column:id"`
	Title                 string     `gorm:"column:title;not null"`
	Description           string     `gorm:"column:description;not null;default:''"`
	Domain                string     `gorm:"column:domain;not null"`
	AdditionalDomainsJSON string     `gorm:"column:additional_domains_json;not null;default:'[]'"`
	Priority              int        `gorm:"column:priority;not null;default:0"`
	MaxIterations         int        `gorm:"column:max_iterations;not null;default:0"`
	Deadline              *time.Time `gorm:"column:deadline"`
	CriteriaJSON          string     `gorm:"column:criteria_json;not null;default:'[]'"`
	ConstraintsJSON       string     `gorm:"column:constraints_json;not null;default:'[]'"`
	ParentID              *string    `gorm:"column:parent_id"`
	Status                string     `gorm:"column:status;not null;default:'pending'"`
	TwinID                string     `gorm:"column:twin_id;index"`
	// Standing-objective declaration (Phase 20). Mode is indexed because the
	// supervisor's boot scan asks for standing objectives and nothing else.
	// Empty means oneshot, which is what every row written before Phase 20
	// reads as — adding a column must not change what an existing row means.
	Mode         string    `gorm:"column:mode;not null;default:'';index"`
	CadenceJSON  string    `gorm:"column:cadence_json;not null;default:''"`
	AutonomyJSON string    `gorm:"column:autonomy_json;not null;default:''"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (ObjectiveModel) TableName() string { return "objectives" }

type LoopIterationModel struct {
	ID          string    `gorm:"primaryKey;column:id"`
	ObjectiveID string    `gorm:"column:objective_id;not null;index"`
	Number      int       `gorm:"column:number;not null"`
	Step        string    `gorm:"column:step;not null"`
	InputJSON   string    `gorm:"column:input_json"`
	OutputJSON  string    `gorm:"column:output_json"`
	TokensUsed  int       `gorm:"column:tokens_used;not null;default:0"`
	DurationMS  int64     `gorm:"column:duration_ms;not null;default:0"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (LoopIterationModel) TableName() string { return "loop_iterations" }

type MemoryEpisodicModel struct {
	ID          string     `gorm:"primaryKey;column:id"`
	AgentID     string     `gorm:"column:agent_id;not null;index"`
	TwinID      string     `gorm:"column:twin_id;not null;index"`
	Domain      string     `gorm:"column:domain;not null;default:''"`
	Content     string     `gorm:"column:content;not null"`
	Confidence  float64    `gorm:"column:confidence;not null;default:1.0"`
	SourcesJSON string     `gorm:"column:sources_json;not null;default:'[]'"`
	CreatedAt   time.Time  `gorm:"column:created_at;autoCreateTime"`
	ExpiresAt   *time.Time `gorm:"column:expires_at"`
}

func (MemoryEpisodicModel) TableName() string { return "memory_episodic" }

type MemoryProceduralModel struct {
	ID            string    `gorm:"primaryKey;column:id"`
	AgentID       string    `gorm:"column:agent_id;not null;index;uniqueIndex:idx_agent_cap"`
	TwinID        string    `gorm:"column:twin_id;not null"`
	CapabilityID  string    `gorm:"column:capability_id;not null;uniqueIndex:idx_agent_cap"`
	SuccessCount  int       `gorm:"column:success_count;not null;default:0"`
	FailureCount  int       `gorm:"column:failure_count;not null;default:0"`
	AvgConfidence float64   `gorm:"column:avg_confidence;not null;default:0.0"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (MemoryProceduralModel) TableName() string { return "memory_procedural" }

type MemorySemanticModel struct {
	ID          string     `gorm:"primaryKey;column:id"`
	AgentID     string     `gorm:"column:agent_id;not null;index"`
	TwinID      string     `gorm:"column:twin_id;not null;index"`
	Domain      string     `gorm:"column:domain;not null;default:''"`
	Content     string     `gorm:"column:content;not null"`
	Embedding   []byte     `gorm:"column:embedding"`
	Confidence  float64    `gorm:"column:confidence;not null;default:1.0"`
	SourcesJSON string     `gorm:"column:sources_json;not null;default:'[]'"`
	CreatedAt   time.Time  `gorm:"column:created_at;autoCreateTime"`
	ExpiresAt   *time.Time `gorm:"column:expires_at"`
}

func (MemorySemanticModel) TableName() string { return "memory_semantic" }

type CheckpointModel struct {
	ID          string  `gorm:"primaryKey;column:id"`
	ObjectiveID string  `gorm:"column:objective_id;not null;index"`
	TwinID      string  `gorm:"column:twin_id;not null"`
	Reason      string  `gorm:"column:reason;not null;default:''"`
	Summary     string  `gorm:"column:summary;not null;default:''"`
	OptionsJSON string  `gorm:"column:options_json;not null;default:'[]'"`
	Capability  string  `gorm:"column:capability;not null;default:''"`
	Confidence  float64 `gorm:"column:confidence;not null;default:0.0"`
	// ActionsJSON serializes the planner draft surfaced to reviewers
	// (Phase 13.5). Empty array is the default for older rows.
	ActionsJSON string `gorm:"column:actions_json;not null;default:'[]'"`
	// AuditEventID links the checkpoint to its kind=escalation audit row
	// (Phase 13.5). Empty when the audit write failed at escalation time.
	AuditEventID string     `gorm:"column:audit_event_id;not null;default:''"`
	Status       string     `gorm:"column:status;not null;default:'pending';index"`
	DecisionJSON string     `gorm:"column:decision_json"`
	CreatedAt    time.Time  `gorm:"column:created_at;autoCreateTime"`
	ResolvedAt   *time.Time `gorm:"column:resolved_at"`
}

func (CheckpointModel) TableName() string { return "checkpoints" }

type BlobModel struct {
	SHA         string    `gorm:"primaryKey;column:sha"`
	Content     []byte    `gorm:"column:content;not null"`
	ContentType string    `gorm:"column:content_type;not null;default:'text/plain'"`
	Size        int64     `gorm:"column:size;not null;default:0"`
	ObjectiveID string    `gorm:"column:objective_id;not null;default:''"`
	AgentID     string    `gorm:"column:agent_id;not null;default:''"`
	Capability  string    `gorm:"column:capability;not null;default:''"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (BlobModel) TableName() string { return "blobs" }

type WorktreeModel struct {
	TaskID      string    `gorm:"primaryKey;column:task_id"`
	ObjectiveID string    `gorm:"column:objective_id;not null;index"`
	Path        string    `gorm:"column:path;not null"`
	Branch      string    `gorm:"column:branch;not null"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (WorktreeModel) TableName() string { return "worktrees" }

type ToolEventModel struct {
	ID          string  `gorm:"primaryKey;column:id"`
	ObjectiveID string  `gorm:"column:objective_id;not null;default:'';index"`
	AgentID     string  `gorm:"column:agent_id;not null;default:''"`
	Capability  string  `gorm:"column:capability;not null;default:''"`
	Adapter     string  `gorm:"column:adapter;not null;default:''"`
	Success     bool    `gorm:"column:success;not null;default:false"`
	Confidence  float64 `gorm:"column:confidence;not null;default:0.0"`
	PayloadJSON string  `gorm:"column:payload_json"`
	// Audit fields (Phase 13). Kind distinguishes routine execution
	// ("execute") from escalation events ("escalation") and approval
	// resolutions ("approval"). Most operators only filter by kind +
	// objective; the other audit columns surface for forensics.
	Kind             string    `gorm:"column:kind;not null;default:'execute';index"`
	EscalationReason string    `gorm:"column:escalation_reason;not null;default:''"`
	Approver         string    `gorm:"column:approver;not null;default:''"`
	BoundsViolation  bool      `gorm:"column:bounds_violation;not null;default:false;index"`
	CreatedAt        time.Time `gorm:"column:created_at;autoCreateTime;index"`
}

func (ToolEventModel) TableName() string { return "tool_events" }

// LoopStateModel persists the per-loop progress slice that survives server
// restarts (Phase 11). Transient per-process resources — the checkpoint
// decision channel, the live agent — are NOT persisted; they're rebuilt by
// the runner when a loop resumes.
type LoopStateModel struct {
	LoopID       string    `gorm:"primaryKey;column:loop_id"`
	ObjectiveID  string    `gorm:"column:objective_id;not null;index"`
	TwinID       string    `gorm:"column:twin_id;not null;default:''"`
	AgentID      string    `gorm:"column:agent_id;not null;default:''"`
	Iteration    int       `gorm:"column:iteration;not null;default:0"`
	Paused       bool      `gorm:"column:paused;not null;default:false"`
	Completed    bool      `gorm:"column:completed;not null;default:false;index"`
	LastStep     string    `gorm:"column:last_step;not null;default:''"`
	Status       string    `gorm:"column:status;not null;default:''"`
	CriteriaMet  float64   `gorm:"column:criteria_met;not null;default:0.0"`
	CheckpointID string    `gorm:"column:checkpoint_id;not null;default:''"`
	RequestJSON  string    `gorm:"column:request_json;not null;default:'{}'"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (LoopStateModel) TableName() string { return "loop_states" }

// ReconcileStateModel persists one standing objective's outer control loop
// (Phase 20): when it is next due, what the world looked like when it last
// converged, how it has been going, and who is currently working on it.
//
// The primary key is the objective ID rather than a synthetic one. A standing
// objective has exactly one control loop by definition, and keying on the
// objective makes "claim this objective" a single-row conditional UPDATE with
// nothing to disambiguate.
//
// Three indices carry the design. next_due_at is the due-wheel query and the
// only one on the hot path — the supervisor asks "what is due" every tick and
// must not scan. lease_until lets an operator find claims that outlived their
// holder. paused keeps stopped objectives out of the due scan entirely.
type ReconcileStateModel struct {
	ObjectiveID  string `gorm:"primaryKey;column:objective_id"`
	TwinID       string `gorm:"column:twin_id;not null;default:'';index"`
	Phase        string `gorm:"column:phase;not null;default:'idle'"`
	Paused       bool   `gorm:"column:paused;not null;default:false;index"`
	PausedReason string `gorm:"column:paused_reason;not null;default:''"`

	// Nullable because "never due on its own" is a real state — a standing
	// objective that reconciles only when asked. A zero timestamp here would
	// read as overdue since year one and put it in every due scan forever.
	NextDueAt       *time.Time `gorm:"column:next_due_at;index"`
	NextSenseAt     *time.Time `gorm:"column:next_sense_at"`
	NextReconcileAt *time.Time `gorm:"column:next_reconcile_at"`

	// ConvergedJSON is the fingerprint at the last convergence: the composite
	// hash, the per-environment hashes behind it, and which environments
	// could not be hashed at all. Stored as JSON because the set of
	// environments is the objective's to change, not the schema's.
	ConvergedJSON   string     `gorm:"column:converged_json;not null;default:'{}'"`
	LastConvergedAt *time.Time `gorm:"column:last_converged_at"`

	LastRunAt        *time.Time `gorm:"column:last_run_at"`
	LastReconciledAt *time.Time `gorm:"column:last_reconciled_at"`
	LastTrigger      string     `gorm:"column:last_trigger;not null;default:''"`
	LastOutcomeID    string     `gorm:"column:last_outcome_id;not null;default:''"`
	LastError        string     `gorm:"column:last_error;not null;default:''"`
	// The loop left running when a reconcile escalated. The supervisor does
	// not sit on a paused loop — a human may take days — so it lets go of the
	// lease and remembers what it let go of.
	ActiveLoopID string `gorm:"column:active_loop_id;not null;default:''"`

	CriteriaMet         float64 `gorm:"column:criteria_met;not null;default:0.0"`
	ScoreStreak         int     `gorm:"column:score_streak;not null;default:0"`
	ConsecutiveFailures int     `gorm:"column:consecutive_failures;not null;default:0"`

	Autonomy  string `gorm:"column:autonomy;not null;default:''"`
	CleanRuns int    `gorm:"column:clean_runs;not null;default:0"`

	Holder     string     `gorm:"column:holder;not null;default:''"`
	LeaseUntil *time.Time `gorm:"column:lease_until;index"`

	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (ReconcileStateModel) TableName() string { return "reconcile_states" }

// ReconcileOutcomeModel is one completed pass of the outer loop, cheap or
// expensive (Phase 20).
//
// Sense-only passes are recorded too, and they are the majority. That is the
// point of keeping them: "this objective has been checked forty-eight times
// today and cost nothing" is the evidence that the two-tier split is working,
// and without the cheap rows the history would show only the expensive ones
// and look like the system was barely watching.
type ReconcileOutcomeModel struct {
	ID          string `gorm:"primaryKey;column:id"`
	ObjectiveID string `gorm:"column:objective_id;not null;index:idx_reconcile_outcomes_objective_time,priority:1"`
	TwinID      string `gorm:"column:twin_id;not null;default:''"`

	Trigger string `gorm:"column:trigger;not null"`
	LoopID  string `gorm:"column:loop_id;not null;default:''"`

	DriftJSON string `gorm:"column:drift_json;not null;default:'{}'"`
	Autonomy  string `gorm:"column:autonomy;not null;default:''"`

	CriteriaMet float64 `gorm:"column:criteria_met;not null;default:0.0"`
	Converged   bool    `gorm:"column:converged;not null;default:false"`

	Escalated    bool   `gorm:"column:escalated;not null;default:false"`
	CheckpointID string `gorm:"column:checkpoint_id;not null;default:''"`
	Error        string `gorm:"column:error;not null;default:''"`

	StartedAt time.Time `gorm:"column:started_at;index:idx_reconcile_outcomes_objective_time,priority:2,sort:desc"`
	EndedAt   time.Time `gorm:"column:ended_at"`
}

func (ReconcileOutcomeModel) TableName() string { return "reconcile_outcomes" }

// ReportScheduleModel is a standing instruction to report (Phase 21): who to
// tell, how often, and through which adapter.
//
// Keyed on a twin rather than an objective. The reader wants one morning brief
// covering everything they are responsible for; a twin holding nine standing
// objectives should produce one message a day, not nine.
//
// It carries a lease for the same reason reconcile_states does, and with more
// at stake: two replicas reconciling the same objective wastes money, while two
// replicas sending the same morning report send it to a person twice.
type ReportScheduleModel struct {
	ID     string `gorm:"primaryKey;column:id"`
	TwinID string `gorm:"column:twin_id;not null;index"`

	CadenceJSON string `gorm:"column:cadence_json;not null;default:'{}'"`

	Channel  string `gorm:"column:channel;not null"`
	Instance string `gorm:"column:instance;not null;default:''"`
	Target   string `gorm:"column:target;not null;default:''"`

	Window string `gorm:"column:window;not null;default:''"`
	// Off by default: a daily mail that says "nothing happened" is a mail
	// people stop reading, which costs the ones that matter their audience.
	SendWhenEmpty bool `gorm:"column:send_when_empty;not null;default:false"`
	Enabled       bool `gorm:"column:enabled;not null;default:true;index"`

	NextDueAt  *time.Time `gorm:"column:next_due_at;index"`
	LastSentAt *time.Time `gorm:"column:last_sent_at"`
	LastError  string     `gorm:"column:last_error;not null;default:''"`

	Holder     string     `gorm:"column:holder;not null;default:''"`
	LeaseUntil *time.Time `gorm:"column:lease_until;index"`

	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (ReportScheduleModel) TableName() string { return "report_schedules" }

// ContainerModel is one node of the tenancy tree — an org, a team or a project
// (Phase 17).
//
// The unique index is on (parent_id, kind, name), not on name: two
// organisations may both have a team called "Engineering", and the whole point
// of scoping on IDs is that they do not collide when they do. Projects have no
// parent, so their names are unique among projects.
type ContainerModel struct {
	ID        string    `gorm:"primaryKey;column:id"`
	Kind      string    `gorm:"column:kind;not null;index:idx_containers_sibling,unique,priority:2"`
	Name      string    `gorm:"column:name;not null;index:idx_containers_sibling,unique,priority:3"`
	ParentID  string    `gorm:"column:parent_id;not null;default:'';index;index:idx_containers_sibling,unique,priority:1"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (ContainerModel) TableName() string { return "containers" }

// ResourceScopeModel is one label a resource carries (Phase 17): the flattened
// ancestor closure that authorization matches a binding scope against.
//
// Direct separates what somebody declared from what was derived by closing over
// ancestry. Both live in one table because both are matched the same way — the
// listing query does not care how a label got there — while reparenting needs
// to find declarations and recompute the rest, which a closure alone cannot
// reproduce from itself.
//
// The index on label is what makes subtree listing an indexed IN clause rather
// than the prefix scan a path model would need.
type ResourceScopeModel struct {
	ResourceType string `gorm:"primaryKey;column:resource_type"`
	ResourceID   string `gorm:"primaryKey;column:resource_id"`
	Label        string `gorm:"primaryKey;column:label;index"`
	Direct       bool   `gorm:"column:direct;not null;default:false"`
}

func (ResourceScopeModel) TableName() string { return "resource_scopes" }
