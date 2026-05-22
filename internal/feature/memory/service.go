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
	semantic   memory.Memory // pluggable: SQLite keyword fallback or pgvector
	procedural *platmem.ProceduralMemory
	working    *platmem.WorkingMemory
	store      storage.StorageAdapter
	topK       int
}

// NewService wires the default semantic backend (SQLite keyword fallback).
// Callers that want pgvector should use NewServiceWithSemantic instead.
func NewService(store storage.StorageAdapter, topK int) *Service {
	return NewServiceWithSemantic(store, topK, platmem.NewSemanticMemory(store))
}

// NewServiceWithSemantic injects a specific semantic-tier backend — used by
// bootstrap when memory.vector_backend == "pgvector".
func NewServiceWithSemantic(store storage.StorageAdapter, topK int, semantic memory.Memory) *Service {
	return &Service{
		episodic:   platmem.NewEpisodicMemory(store),
		semantic:   semantic,
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
	if len(q.Tiers) == 0 {
		q.Tiers = []memory.Tier{memory.TierEpisodic, memory.TierSemantic, memory.TierProcedural, memory.TierWorking}
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

// RetentionPolicySet bundles per-tier policies for a single retention sweep.
// Each tier's policy is optional — a zero policy is treated as "skip this
// tier". This is the contract MemoryService.RunRetention executes on; the
// scheduler in bootstrap simply translates config into this shape and calls.
type RetentionPolicySet struct {
	Working  memory.RetentionPolicy
	Episodic memory.RetentionPolicy
	Semantic memory.RetentionPolicy
}

// RunRetention executes a single retention sweep across all configured tiers.
// Errors from one tier do not stop the others — the sweep is best-effort by
// design: a bad query against semantic shouldn't block episodic cleanup. The
// returned error wraps the last failure for surfacing in logs.
func (s *Service) RunRetention(ctx context.Context, set RetentionPolicySet) error {
	var lastErr error
	if !isEmptyPolicy(set.Working) {
		if err := s.working.Forget(ctx, set.Working); err != nil {
			lastErr = fmt.Errorf("working tier retention: %w", err)
		}
	}
	if !isEmptyPolicy(set.Episodic) {
		if err := s.episodic.Forget(ctx, set.Episodic); err != nil {
			lastErr = fmt.Errorf("episodic tier retention: %w", err)
		}
	}
	if !isEmptyPolicy(set.Semantic) {
		if err := s.semantic.Forget(ctx, set.Semantic); err != nil {
			lastErr = fmt.Errorf("semantic tier retention: %w", err)
		}
	}
	return lastErr
}

// isEmptyPolicy reports whether p would have no effect — no age cutoff and
// no confidence floor. Skipping these saves a pointless table scan.
func isEmptyPolicy(p memory.RetentionPolicy) bool {
	return p.Before == nil && p.MinScore <= 0
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
