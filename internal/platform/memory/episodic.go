package memory

import (
	"context"
	"fmt"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/memory"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// EpisodicMemory stores agent reasoning traces and iteration records in SQLite.
type EpisodicMemory struct {
	store storage.StorageAdapter
}

func NewEpisodicMemory(store storage.StorageAdapter) *EpisodicMemory {
	return &EpisodicMemory{store: store}
}

func (m *EpisodicMemory) Store(ctx context.Context, e memory.Entry) error {
	if e.ID == "" {
		e.ID = fmt.Sprintf("ep-%d", time.Now().UnixNano())
	}
	e.Tier = string(memory.TierEpisodic)
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	return m.store.SaveMemoryEpisodic(ctx, e)
}

func (m *EpisodicMemory) Recall(ctx context.Context, q memory.Query) ([]memory.Entry, error) {
	return m.store.QueryEpisodic(ctx, q)
}

func (m *EpisodicMemory) Forget(ctx context.Context, p memory.RetentionPolicy) error {
	entries, err := m.store.QueryEpisodic(ctx, memory.Query{
		AgentID: p.AgentID, TwinID: p.TwinID, Since: nil,
	})
	if err != nil {
		return err
	}
	for _, e := range entries {
		if p.Before != nil && e.CreatedAt.Before(*p.Before) {
			if err := m.store.DeleteMemoryEntry(ctx, e.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// Consolidate promotes high-confidence episodic entries to semantic memory.
// Actual promotion is handled by MemoryService in the feature layer.
func (m *EpisodicMemory) Consolidate(_ context.Context, _ coreagent.AgentID) error {
	return nil
}
