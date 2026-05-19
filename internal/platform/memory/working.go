package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/memory"
)

// WorkingMemory is an in-process map scoped per agent invocation.
type WorkingMemory struct {
	mu      sync.RWMutex
	entries map[string]memory.Entry
}

func NewWorkingMemory() *WorkingMemory {
	return &WorkingMemory{entries: make(map[string]memory.Entry)}
}

func (m *WorkingMemory) Store(_ context.Context, e memory.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e.ID == "" {
		e.ID = fmt.Sprintf("wk-%d", time.Now().UnixNano())
	}
	e.Tier = string(memory.TierWorking)
	m.entries[e.ID] = e
	return nil
}

func (m *WorkingMemory) Recall(_ context.Context, q memory.Query) ([]memory.Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []memory.Entry
	for _, e := range m.entries {
		if string(q.AgentID) != "" && e.AgentID != q.AgentID {
			continue
		}
		out = append(out, e)
		if q.TopK > 0 && len(out) >= q.TopK {
			break
		}
	}
	return out, nil
}

func (m *WorkingMemory) Forget(_ context.Context, p memory.RetentionPolicy) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, e := range m.entries {
		if p.Before != nil && e.CreatedAt.Before(*p.Before) {
			delete(m.entries, id)
		}
	}
	return nil
}

func (m *WorkingMemory) Consolidate(_ context.Context, _ coreagent.AgentID) error {
	return nil
}
