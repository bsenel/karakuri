package storage

import (
	"context"
	"time"

	"github.com/bsenel/karakuri/internal/core/checkpoint"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/memory"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/twin"
	"github.com/bsenel/karakuri/internal/core/vfs"
)

// TwinFilter filters twin list queries.
type TwinFilter struct {
	Kind   string
	Domain string
	Limit  int
	Offset int
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
type ToolEvent struct {
	ID          string
	ObjectiveID string
	AgentID     string
	Capability  string
	Adapter     string
	Success     bool
	Confidence  float64
	PayloadJSON string
	// Audit fields (Phase 13). Default Kind is "execute"; escalation
	// records use "escalation" and human approvals use "approval".
	Kind             string
	EscalationReason string
	Approver         string
	BoundsViolation  bool
	CreatedAt        time.Time
}

// ToolEventKind enumerates the audit-relevant event types.
const (
	ToolEventExecute    = "execute"
	ToolEventEscalation = "escalation"
	ToolEventApproval   = "approval"
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
	ListObjectives(ctx context.Context, twinID string, status string) ([]objective.Objective, error)
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

	// Loop state (Phase 11 — durable execution across server restarts)
	SaveLoopState(ctx context.Context, s coreloop.State) error
	GetLoopState(ctx context.Context, loopID string) (coreloop.State, error)
	ListActiveLoopStates(ctx context.Context) ([]coreloop.State, error)
	DeleteLoopState(ctx context.Context, loopID string) error
}

func Now() time.Time { return time.Now().UTC() }
