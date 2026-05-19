package memory

import (
	"context"
	"fmt"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/memory"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// ProceduralMemory tracks capability success/failure rates for informed decision-making.
type ProceduralMemory struct {
	store storage.StorageAdapter
}

func NewProceduralMemory(store storage.StorageAdapter) *ProceduralMemory {
	return &ProceduralMemory{store: store}
}

func (m *ProceduralMemory) Store(ctx context.Context, e memory.Entry) error {
	capID := e.Domain // Domain field repurposed as capability ID for procedural entries
	rec, err := m.store.QueryProcedural(ctx, string(e.AgentID), capID)
	if err != nil {
		rec = storage.ProceduralRecord{
			ID:           fmt.Sprintf("proc-%d", time.Now().UnixNano()),
			AgentID:      string(e.AgentID),
			TwinID:       e.TwinID,
			CapabilityID: capID,
		}
	}
	if e.Confidence >= 0.5 {
		rec.SuccessCount++
	} else {
		rec.FailureCount++
	}
	total := float64(rec.SuccessCount + rec.FailureCount)
	rec.AvgConfidence = (rec.AvgConfidence*(total-1) + e.Confidence) / total
	rec.UpdatedAt = time.Now().UTC()
	return m.store.UpsertProcedural(ctx, rec)
}

func (m *ProceduralMemory) Recall(ctx context.Context, q memory.Query) ([]memory.Entry, error) {
	rec, err := m.store.QueryProcedural(ctx, string(q.AgentID), q.Domain)
	if err != nil {
		return nil, nil
	}
	return []memory.Entry{{
		ID:         rec.ID,
		AgentID:    coreagent.AgentID(rec.AgentID),
		TwinID:     rec.TwinID,
		Tier:       string(memory.TierProcedural),
		Domain:     rec.CapabilityID,
		Content:    fmt.Sprintf("success=%d failure=%d avg_confidence=%.2f", rec.SuccessCount, rec.FailureCount, rec.AvgConfidence),
		Confidence: rec.AvgConfidence,
	}}, nil
}

func (m *ProceduralMemory) Forget(_ context.Context, _ memory.RetentionPolicy) error { return nil }

func (m *ProceduralMemory) Consolidate(_ context.Context, _ coreagent.AgentID) error { return nil }
