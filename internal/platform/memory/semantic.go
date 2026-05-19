package memory

import (
	"context"
	"fmt"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/memory"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// SemanticMemory stores embedded knowledge using sqlite-vec (falls back to keyword recall in v1).
type SemanticMemory struct {
	store storage.StorageAdapter
}

func NewSemanticMemory(store storage.StorageAdapter) *SemanticMemory {
	return &SemanticMemory{store: store}
}

func (m *SemanticMemory) Store(ctx context.Context, e memory.Entry) error {
	if e.ID == "" {
		e.ID = fmt.Sprintf("sm-%d", time.Now().UnixNano())
	}
	e.Tier = string(memory.TierSemantic)
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	return m.store.SaveMemorySemantic(ctx, e)
}

func (m *SemanticMemory) Recall(ctx context.Context, q memory.Query) ([]memory.Entry, error) {
	return m.store.QuerySemantic(ctx, q)
}

func (m *SemanticMemory) Forget(ctx context.Context, p memory.RetentionPolicy) error {
	entries, err := m.store.QuerySemantic(ctx, memory.Query{
		AgentID: p.AgentID, TwinID: p.TwinID,
	})
	if err != nil {
		return err
	}
	for _, e := range entries {
		if p.Before != nil && e.CreatedAt.Before(*p.Before) {
			_ = m.store.DeleteMemoryEntry(ctx, e.ID)
		}
	}
	return nil
}

func (m *SemanticMemory) Consolidate(_ context.Context, _ coreagent.AgentID) error {
	return nil
}
