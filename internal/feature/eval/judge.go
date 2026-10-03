package eval

import (
	"context"
	"sync"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/feature/loop"
)

// agentBuilder is the slice of the agent factory judging needs.
type agentBuilder interface {
	New(ctx context.Context, def coreagent.Definition) (coreagent.Agent, error)
}

// LoopJudge resolves an objective's judge the way the loop does: SelectAgent
// picks the definition and the factory builds it, so a calibration asks the
// agent that would have judged the objective's criteria.
//
// Built agents are kept per definition. A calibration asks about the same few
// agents once per checkpoint, and an agent holds no state between calls.
func LoopJudge(factory agentBuilder, domReg *domain.Registry) JudgeFor {
	var mu sync.Mutex
	built := map[string]coreagent.Agent{}
	return func(ctx context.Context, obj objective.Objective) (coreagent.Agent, error) {
		def := loop.SelectAgent(domReg, obj, coreagent.Definition{})
		mu.Lock()
		defer mu.Unlock()
		if a, ok := built[string(def.ID)]; ok {
			return a, nil
		}
		a, err := factory.New(ctx, def)
		if err != nil {
			return nil, err
		}
		built[string(def.ID)] = a
		return a, nil
	}
}
