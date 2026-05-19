package agent

import (
	"context"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
)

type AgentID string

type ReasoningStrategy string

const (
	ReasoningChainOfThought ReasoningStrategy = "chain_of_thought"
	ReasoningTreeOfThought  ReasoningStrategy = "tree_of_thought"
	ReasoningReAct          ReasoningStrategy = "react"
	ReasoningReflexion      ReasoningStrategy = "reflexion"
)

type Definition struct {
	ID                AgentID
	Name              string
	Domain            string
	Capabilities      []capability.CapabilityID
	Memory            MemoryConfig
	ReasoningStrategy ReasoningStrategy
	Authority         AuthorityBounds
	LLMHints          capability.LLMHints
}

type AuthorityBounds struct {
	MaxAutonomousActions int
	RequiresApprovalFor  []capability.CapabilityID
	CanDelegate          bool
	CanModifyObjective   bool
	ConfidenceThreshold  float64 // below this, escalate to human
}

type MemoryConfig struct {
	WorkingWindowSize int           `json:"working_window_size,omitempty"`
	EpisodicRetention time.Duration `json:"episodic_retention,omitempty"`
	SemanticEnabled   bool          `json:"semantic_enabled,omitempty"`
	ProceduralEnabled bool          `json:"procedural_enabled,omitempty"`
}

// Agent is a runtime handle produced by AgentFactory per loop invocation.
type Agent interface {
	Run(ctx context.Context, input Input) (Output, error)
	Stream(ctx context.Context, input Input) (<-chan OutputChunk, error)
}

type Input struct {
	Objective   any // objective.Objective — avoids import cycle; cast in feature layer
	WorldState  any // loop.WorldState
	Memory      []MemoryEntry
	LoopContext any // loop.LoopContext
	Task        string
}

type Output struct {
	Content    string
	Actions    []any // []environment.Action
	Confidence float64
	TokensUsed int
	Reasoning  string // chain-of-thought trace
}

type OutputChunk struct {
	Content string
	Done    bool
	Err     error
}

type MemoryEntry struct {
	ID         string     `json:"id"`
	AgentID    AgentID    `json:"agent_id"`
	TwinID     string     `json:"twin_id,omitempty"`
	Tier       string     `json:"tier"`
	Domain     string     `json:"domain,omitempty"`
	Content    string     `json:"content"`
	Embedding  []float32  `json:"embedding,omitempty"`
	Confidence float64    `json:"confidence"`
	Sources    []string   `json:"sources,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}
