package storage

import (
	"context"
	"time"

	"github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/container"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/memory"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/twin"
	"github.com/bsenel/karakuri/internal/core/vfs"
)

// ScopeSelector selects rows by identity or by containment (Phase 17).
//
// It is the storage-shaped form of the scopes a principal's bindings cover:
// a binding on one twin contributes an ID, a binding on an org contributes a
// label, and a binding on "org:*" contributes a prefix. Translating patterns
// into these three lists is the caller's job (internal/auth.ListSelectors),
// because pattern grammar belongs to the auth module and SQL belongs here.
type ScopeSelector struct {
	// IDs names rows directly — a binding scoped "twin:abc".
	IDs []string

	// Labels names containers the row belongs to — "team:t_7f2a".
	Labels []string

	// LabelPrefixes covers a whole kind of container — a binding scoped
	// "org:*" contributes "org:". Rare, but supported so a filtered listing
	// cannot be narrower than the per-row check, which would show a 403 on a
	// list for a row the same principal can fetch by ID.
	LabelPrefixes []string
}

// Empty reports whether the selector matches nothing at all.
func (s ScopeSelector) Empty() bool {
	return len(s.IDs) == 0 && len(s.Labels) == 0 && len(s.LabelPrefixes) == 0
}

// TwinFilter filters twin list queries.
type TwinFilter struct {
	Kind   string
	Domain string
	Limit  int
	Offset int

	// Visible restricts the listing to what a principal may see. Nil means no
	// restriction — an unauthenticated internal caller, or a principal whose
	// grants cover everything.
	//
	// It is a pointer rather than an empty-means-everything value on purpose:
	// a principal with no grants at all must see nothing, and a filter that
	// widens to every row when its input is empty is how a listing leaks.
	Visible *ScopeSelector

	// Hidden removes rows an unconditional deny covers. Conditional denies
	// cannot appear here — whether one bites depends on the row — so a filtered
	// list is a narrowing and the per-resource check stays authoritative.
	Hidden ScopeSelector
}

// ObjectiveFilter filters objective list queries. TwinID and Status are the
// original query parameters; the scope fields mean what TwinFilter's do.
type ObjectiveFilter struct {
	TwinID string
	Status string

	Visible *ScopeSelector
	Hidden  ScopeSelector
}

// LoopIteration is the storage DTO for loop step records.
type LoopIteration struct {
	ID          string
	ObjectiveID string
	Number      int
	Step        string
	InputJSON   string
	OutputJSON  string
	TokensUsed  int
	DurationMS  int64
	CreatedAt   time.Time
}

// ProceduralRecord is the storage DTO for procedural memory entries.
type ProceduralRecord struct {
	ID            string
	AgentID       string
	TwinID        string
	CapabilityID  string
	SuccessCount  int
	FailureCount  int
	AvgConfidence float64
	UpdatedAt     time.Time
}

// ToolEvent is the storage DTO for tool operation audit records.
// JSON tags align the wire shape with the frontend AuditEvent type
// (snake_case) so the /api/v1/audit response and the React /audit page
// share one schema.
type ToolEvent struct {
	ID          string  `json:"id"`
	ObjectiveID string  `json:"objective_id"`
	AgentID     string  `json:"agent_id,omitempty"`
	Capability  string  `json:"capability,omitempty"`
	Adapter     string  `json:"adapter,omitempty"`
	Success     bool    `json:"success"`
	Confidence  float64 `json:"confidence,omitempty"`
	PayloadJSON string  `json:"payload_json,omitempty"`
	// Audit fields (Phase 13). Default Kind is "execute"; escalation
	// records use "escalation" and human approvals use "approval".
	// Phase 13.5 adds "modification" + "rejection".
	Kind             string    `json:"kind"`
	EscalationReason string    `json:"escalation_reason,omitempty"`
	Approver         string    `json:"approver,omitempty"`
	BoundsViolation  bool      `json:"bounds_violation,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// ToolEventKind enumerates the audit-relevant event types.
const (
	ToolEventExecute    = "execute"
	ToolEventEscalation = "escalation"
	ToolEventApproval   = "approval"
	// ToolEventModification (Phase 13.5) records a checkpoint resolved with
	// Decision.Choice = "modify". Payload carries the structured diff so
	// reviewers see what changed, not just that something changed.
	ToolEventModification = "modification"
	// ToolEventRejection (Phase 13.5) records a checkpoint resolved with
	// Decision.Choice = "reject". The loop terminates on this kind.
	ToolEventRejection = "rejection"
	// ToolEventAuthzDenied (Phase 14) records an API request refused by RBAC.
	// Attempts belong beside approvals: reviewing who approved what should also
	// show who was turned away. The payload carries the full decision trace.
	ToolEventAuthzDenied = "authz_denied"
)

// ToolEventFilter narrows the audit log query. All fields are optional;
// CreatedAtSince applies an inclusive lower bound on event timestamps.
type ToolEventFilter struct {
	ObjectiveID     string
	AgentID         string
	Kind            string
	BoundsViolation *bool      // tri-state: nil = ignore, &true = only violations, &false = only clean
	CreatedAtSince  *time.Time // events at or after this time only
	Limit           int        // 0 = no cap (caller should usually set this)
}

// Worktree is the storage DTO for worktree records.
type Worktree struct {
	TaskID      string
	ObjectiveID string
	Path        string
	Branch      string
	CreatedAt   time.Time
}

// StorageAdapter is the single database abstraction for all Karakuri persistence.
type StorageAdapter interface {
	// Blobs (VFS)
	SaveBlob(ctx context.Context, sha string, content []byte, meta vfs.BlobMetadata) error
	GetBlob(ctx context.Context, sha string) ([]byte, vfs.BlobMetadata, error)
	ListBlobs(ctx context.Context, objectiveID, agentID string) ([]vfs.BlobMetadata, error)

	// Twins
	SaveTwin(ctx context.Context, t twin.DigitalTwin) error
	GetTwin(ctx context.Context, id string) (twin.DigitalTwin, error)
	ListTwins(ctx context.Context, f TwinFilter) ([]twin.DigitalTwin, error)
	UpdateTwin(ctx context.Context, t twin.DigitalTwin) error

	// Objectives
	SaveObjective(ctx context.Context, o objective.Objective) error
	GetObjective(ctx context.Context, id objective.ObjectiveID) (objective.Objective, error)
	ListObjectives(ctx context.Context, f ObjectiveFilter) ([]objective.Objective, error)
	UpdateObjectiveStatus(ctx context.Context, id objective.ObjectiveID, s objective.ObjectiveStatus) error

	// Loop iterations
	SaveLoopIteration(ctx context.Context, i LoopIteration) error
	ListLoopIterations(ctx context.Context, objectiveID objective.ObjectiveID) ([]LoopIteration, error)

	// Episodic memory
	SaveMemoryEpisodic(ctx context.Context, e memory.Entry) error
	QueryEpisodic(ctx context.Context, q memory.Query) ([]memory.Entry, error)
	DeleteMemoryEntry(ctx context.Context, id string) error

	// Semantic memory
	SaveMemorySemantic(ctx context.Context, e memory.Entry) error
	QuerySemantic(ctx context.Context, q memory.Query) ([]memory.Entry, error)

	// Procedural memory
	UpsertProcedural(ctx context.Context, r ProceduralRecord) error
	QueryProcedural(ctx context.Context, agentID, capabilityID string) (ProceduralRecord, error)

	// Checkpoints
	SaveCheckpoint(ctx context.Context, c checkpoint.Checkpoint) error
	GetCheckpoint(ctx context.Context, id string) (checkpoint.Checkpoint, error)
	ResolveCheckpoint(ctx context.Context, id string, d checkpoint.Decision) error
	ListPendingCheckpoints(ctx context.Context, twinID string) ([]checkpoint.Checkpoint, error)

	// Worktrees
	SaveWorktree(ctx context.Context, w Worktree) error
	GetWorktree(ctx context.Context, taskID string) (Worktree, error)
	ListWorktrees(ctx context.Context, objectiveID objective.ObjectiveID) ([]Worktree, error)
	DeleteWorktree(ctx context.Context, taskID string) error

	// Tool events
	SaveToolEvent(ctx context.Context, e ToolEvent) error
	ListToolEvents(ctx context.Context, f ToolEventFilter) ([]ToolEvent, error)

	// GetToolEvent returns one audit row by ID, for a detail view reached by
	// link rather than by scrolling.
	GetToolEvent(ctx context.Context, id string) (ToolEvent, error)

	// Loop state (Phase 11 — durable execution across server restarts)
	SaveLoopState(ctx context.Context, s coreloop.State) error
	GetLoopState(ctx context.Context, loopID string) (coreloop.State, error)
	ListActiveLoopStates(ctx context.Context) ([]coreloop.State, error)
	DeleteLoopState(ctx context.Context, loopID string) error

	// Containers and scopes (Phase 17 — the tenancy tree and the flattened
	// closure authorization matches against).
	SaveContainer(ctx context.Context, c container.Container) error
	GetContainer(ctx context.Context, id string) (container.Container, error)
	ListContainers(ctx context.Context, f container.Filter) ([]container.Container, error)
	DeleteContainer(ctx context.Context, id string) error

	PutResourceScopes(ctx context.Context, s container.ResourceScopes) error
	GetResourceScopes(ctx context.Context, resourceType, resourceID string) (container.ResourceScopes, error)
	ListScopedResources(ctx context.Context, f container.ScopeFilter) ([]container.ResourceScopes, error)
	DeleteResourceScopes(ctx context.Context, resourceType, resourceID string) error
}

func Now() time.Time { return time.Now().UTC() }
