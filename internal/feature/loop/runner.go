package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/memory"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// stepContext is passed to each step function.
type stepContext struct {
	loopID     string
	state      *loopState
	agent      coreagent.Agent
	agentDef   coreagent.Definition
	envs       []environment.Environment
	obj        objective.Objective
	twinID     string
	iteration  int
	svc        *serviceImpl
	memEntries []memory.Entry
}

func (s *serviceImpl) runLoop(ctx context.Context, loopID string, req loop.Request) {
	s.mu.RLock()
	state := s.states[loopID]
	s.mu.RUnlock()

	// 1. Fetch the full objective from storage
	obj, err := s.store.GetObjective(ctx, req.Objective.ID)
	if err != nil {
		s.finalizeLoop(ctx, state, obj, nil, false, fmt.Errorf("fetch objective: %w", err))
		return
	}

	// 2. Select agent definition
	var agentDef coreagent.Definition
	if req.Agent.ID != "" {
		agentDef = req.Agent
	} else {
		// Pick first agent from domain pack matching obj.Domain
		pack, ok := s.domReg.Get(obj.Domain)
		if !ok || len(pack.AgentDefinitions()) == 0 {
			// Use a default minimal agent definition
			agentDef = coreagent.Definition{
				ID:                coreagent.AgentID(obj.Domain + "-default"),
				Name:              obj.Domain + " Agent",
				Domain:            obj.Domain,
				ReasoningStrategy: coreagent.ReasoningReAct,
			}
		} else {
			agentDef = pack.AgentDefinitions()[0]
		}
	}

	// 3. Create the agent
	agent, err := s.factory.New(ctx, agentDef)
	if err != nil {
		s.finalizeLoop(ctx, state, obj, nil, false, fmt.Errorf("create agent: %w", err))
		return
	}

	// 4. Build all environments for the domain
	var envs []environment.Environment
	for _, fac := range s.envReg.ListByDomain(obj.Domain) {
		env, err := fac.Build(nil)
		if err != nil {
			// Log but don't fail — some envs may be optional
			s.hub.Publish(ctx, event.Event{
				Type:        event.TypeAdapterSkipped,
				ObjectiveID: string(obj.ID),
				Payload:     map[string]any{"env_id": string(fac.EnvID), "error": err.Error()},
				Timestamp:   time.Now().UTC(),
			})
			continue
		}
		envs = append(envs, env)
	}

	// 5. Set objective status to active
	_ = s.store.UpdateObjectiveStatus(ctx, obj.ID, objective.StatusActive)

	// 6. Run the iteration loop
	maxIter := req.MaxIter
	if maxIter <= 0 {
		maxIter = obj.MaxIterations
	}
	if maxIter <= 0 {
		maxIter = 50
	}

	sc := &stepContext{
		loopID:   loopID,
		state:    state,
		agent:    agent,
		agentDef: agentDef,
		envs:     envs,
		obj:      obj,
		twinID:   obj.TwinID,
		svc:      s,
	}

	var (
		score      float64
		criteriaMet bool
		iterations []loop.Iteration
	)

	for iter := 0; iter < maxIter; iter++ {
		sc.iteration = iter

		// Update state step
		state.mu.Lock()
		state.status.Iteration = iter
		state.status.Step = loop.StepObserve
		state.mu.Unlock()

		// observe
		ws := stepObserve(ctx, sc)
		iterations = append(iterations, loop.Iteration{
			Number:    iter,
			Step:      loop.StepObserve,
			Input:     nil,
			Output:    ws,
			Timestamp: time.Now().UTC(),
		})

		// reason
		state.mu.Lock()
		state.status.Step = loop.StepReason
		state.mu.Unlock()

		p := stepReason(ctx, sc, ws)
		inputJSON, _ := json.Marshal(ws)
		outputJSON, _ := json.Marshal(p)
		iterations = append(iterations, loop.Iteration{
			Number:    iter,
			Step:      loop.StepReason,
			Input:     string(inputJSON),
			Output:    string(outputJSON),
			Timestamp: time.Now().UTC(),
		})

		// decide
		state.mu.Lock()
		state.status.Step = loop.StepDecide
		state.mu.Unlock()

		p, paused := stepDecide(ctx, sc, p)
		if paused {
			state.mu.Lock()
			state.status.Paused = true
			state.mu.Unlock()

			// Wait for resume signal
			select {
			case <-ctx.Done():
				s.finalizeLoop(ctx, state, obj, iterations, false, ctx.Err())
				return
			case <-state.decisionCh:
			}

			state.mu.Lock()
			state.status.Paused = false
			state.mu.Unlock()
			// Fall through to act with the approved plan
		}

		// act
		state.mu.Lock()
		state.status.Step = loop.StepAct
		state.mu.Unlock()

		results := stepAct(ctx, sc, p)
		iterations = append(iterations, loop.Iteration{
			Number:    iter,
			Step:      loop.StepAct,
			Input:     p,
			Output:    results,
			Timestamp: time.Now().UTC(),
		})

		// verify
		state.mu.Lock()
		state.status.Step = loop.StepVerify
		state.mu.Unlock()

		score, criteriaMet = stepVerify(ctx, sc, results)
		iterations = append(iterations, loop.Iteration{
			Number:    iter,
			Step:      loop.StepVerify,
			Input:     results,
			Output:    score,
			Timestamp: time.Now().UTC(),
		})

		state.mu.Lock()
		state.status.CriteriaMet = score
		state.mu.Unlock()

		// learn
		state.mu.Lock()
		state.status.Step = loop.StepLearn
		state.mu.Unlock()

		stepLearn(ctx, sc, ws, p, results, score)

		if score >= 1.0 {
			break
		}
	}

	s.finalizeLoop(ctx, state, obj, iterations, criteriaMet, nil)

	// Watch mode: after completing, subscribe to environment events and wait
	if req.WatchMode && len(envs) > 0 {
		s.runWatchMode(ctx, state, obj, envs, sc)
	}
}

func (s *serviceImpl) finalizeLoop(ctx context.Context, state *loopState, obj objective.Objective, iterations []loop.Iteration, criteriaMet bool, runErr error) {
	finalStatus := objective.StatusCompleted
	evtType := event.TypeObjectiveCompleted

	if runErr != nil || !criteriaMet {
		finalStatus = objective.StatusFailed
		evtType = event.TypeObjectiveFailed
	}

	if obj.ID != "" {
		_ = s.store.UpdateObjectiveStatus(ctx, obj.ID, finalStatus)
	}

	result := loop.Result{
		LoopID:      state.id,
		ObjectiveID: obj.ID,
		Status:      finalStatus,
		Iterations:  iterations,
	}

	state.mu.Lock()
	state.result = result
	state.status.Step = loop.StepLearn
	state.mu.Unlock()

	if obj.ID != "" {
		s.hub.Publish(ctx, event.Event{
			Type:        evtType,
			ObjectiveID: string(obj.ID),
			Payload:     map[string]any{"loop_id": state.id, "criteria_met": criteriaMet},
			Timestamp:   time.Now().UTC(),
		})
	}
}
