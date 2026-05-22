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
	CreatedAt             time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt             time.Time  `gorm:"column:updated_at;autoUpdateTime"`
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
	ID           string     `gorm:"primaryKey;column:id"`
	ObjectiveID  string     `gorm:"column:objective_id;not null;index"`
	TwinID       string     `gorm:"column:twin_id;not null"`
	Reason       string     `gorm:"column:reason;not null;default:''"`
	Summary      string     `gorm:"column:summary;not null;default:''"`
	OptionsJSON  string     `gorm:"column:options_json;not null;default:'[]'"`
	Capability   string     `gorm:"column:capability;not null;default:''"`
	Confidence   float64    `gorm:"column:confidence;not null;default:0.0"`
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
	ID          string    `gorm:"primaryKey;column:id"`
	ObjectiveID string    `gorm:"column:objective_id;not null;default:'';index"`
	AgentID     string    `gorm:"column:agent_id;not null;default:''"`
	Capability  string    `gorm:"column:capability;not null;default:''"`
	Adapter     string    `gorm:"column:adapter;not null;default:''"`
	Success     bool      `gorm:"column:success;not null;default:false"`
	Confidence  float64   `gorm:"column:confidence;not null;default:0.0"`
	PayloadJSON string    `gorm:"column:payload_json"`
	// Audit fields (Phase 13). Kind distinguishes routine execution
	// ("execute") from escalation events ("escalation") and approval
	// resolutions ("approval"). Most operators only filter by kind +
	// objective; the other audit columns surface for forensics.
	Kind             string `gorm:"column:kind;not null;default:'execute';index"`
	EscalationReason string `gorm:"column:escalation_reason;not null;default:''"`
	Approver         string `gorm:"column:approver;not null;default:''"`
	BoundsViolation  bool   `gorm:"column:bounds_violation;not null;default:false;index"`
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
