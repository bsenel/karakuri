package memory

import (
	"context"
	"time"

	"github.com/bsenel/karakuri/internal/core/agent"
)

type Tier string

const (
	TierWorking    Tier = "working"
	TierEpisodic   Tier = "episodic"
	TierSemantic   Tier = "semantic"
	TierProcedural Tier = "procedural"
)

type Entry = agent.MemoryEntry

type Query struct {
	AgentID agent.AgentID
	TwinID  string
	Tiers   []Tier
	Query   string // natural language; used for semantic similarity
	TopK    int
	Since   *time.Time
	Domain  string
}

type RetentionPolicy struct {
	AgentID   agent.AgentID
	TwinID    string
	Before    *time.Time
	Tiers     []Tier
	MinScore  float64
}

type Memory interface {
	Store(ctx context.Context, e Entry) error
	Recall(ctx context.Context, q Query) ([]Entry, error)
	Forget(ctx context.Context, p RetentionPolicy) error
	Consolidate(ctx context.Context, agentID agent.AgentID) error
}
