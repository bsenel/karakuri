package memory

import (
	"context"
	"fmt"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/memory"
	"github.com/bsenel/karakuri/internal/platform/storage"
	platmem "github.com/bsenel/karakuri/internal/platform/memory"
)

type Service struct {
	episodic   *platmem.EpisodicMemory
	semantic   *platmem.SemanticMemory
	procedural *platmem.ProceduralMemory
	working    *platmem.WorkingMemory
	store      storage.StorageAdapter
	topK       int
}

func NewService(store storage.StorageAdapter, topK int) *Service {
	return &Service{
		episodic:   platmem.NewEpisodicMemory(store),
		semantic:   platmem.NewSemanticMemory(store),
		procedural: platmem.NewProceduralMemory(store),
		working:    platmem.NewWorkingMemory(),
		store:      store,
		topK:       topK,
	}
}

func (s *Service) Store(ctx context.Context, e memory.Entry) error {
	switch memory.Tier(e.Tier) {
	case memory.TierWorking:
		return s.working.Store(ctx, e)
	case memory.TierEpisodic:
		return s.episodic.Store(ctx, e)
	case memory.TierSemantic:
		return s.semantic.Store(ctx, e)
	case memory.TierProcedural:
		return s.procedural.Store(ctx, e)
	default:
		return fmt.Errorf("unknown memory tier: %s", e.Tier)
	}
}

func (s *Service) Recall(ctx context.Context, q memory.Query) ([]memory.Entry, error) {
	if q.TopK == 0 {
		q.TopK = s.topK
	}
	var out []memory.Entry
	for _, tier := range q.Tiers {
		var entries []memory.Entry
		var err error
		tq := q
		tq.Tiers = nil
		switch tier {
		case memory.TierWorking:
			entries, err = s.working.Recall(ctx, tq)
		case memory.TierEpisodic:
			entries, err = s.episodic.Recall(ctx, tq)
		case memory.TierSemantic:
			entries, err = s.semantic.Recall(ctx, tq)
		case memory.TierProcedural:
			entries, err = s.procedural.Recall(ctx, tq)
		}
		if err != nil {
			return nil, err
		}
		out = append(out, entries...)
	}
	return out, nil
}

func (s *Service) Forget(ctx context.Context, p memory.RetentionPolicy) error {
	return s.episodic.Forget(ctx, p)
}

// Consolidate promotes high-confidence episodic entries to semantic memory.
func (s *Service) Consolidate(ctx context.Context, agentID coreagent.AgentID, threshold int) error {
	q := memory.Query{AgentID: agentID, Tiers: []memory.Tier{memory.TierEpisodic}, TopK: threshold * 2}
	entries, err := s.episodic.Recall(ctx, q)
	if err != nil {
		return err
	}
	count := 0
	for _, e := range entries {
		if e.Confidence >= 0.8 {
			sem := e
			sem.Tier = string(memory.TierSemantic)
			sem.ID = fmt.Sprintf("sm-%d", time.Now().UnixNano())
			if err := s.semantic.Store(ctx, sem); err != nil {
				return err
			}
			count++
		}
	}
	_ = count
	return nil
}
