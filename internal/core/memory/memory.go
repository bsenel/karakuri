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
	AgentID agent.AgentID `json:"agent_id,omitempty"`
	TwinID  string        `json:"twin_id,omitempty"`
	Tiers   []Tier        `json:"tiers,omitempty"`
	Query   string        `json:"query,omitempty"`
	TopK    int           `json:"top_k,omitempty"`
	Since   *time.Time    `json:"since,omitempty"`
	Domain  string        `json:"domain,omitempty"`
}

type RetentionPolicy struct {
	AgentID  agent.AgentID `json:"agent_id,omitempty"`
	TwinID   string        `json:"twin_id,omitempty"`
	Before   *time.Time    `json:"before,omitempty"`
	Tiers    []Tier        `json:"tiers,omitempty"`
	MinScore float64       `json:"min_score,omitempty"`
}

type Memory interface {
	Store(ctx context.Context, e Entry) error
	Recall(ctx context.Context, q Query) ([]Entry, error)
	Forget(ctx context.Context, p RetentionPolicy) error
	Consolidate(ctx context.Context, agentID agent.AgentID) error
}
