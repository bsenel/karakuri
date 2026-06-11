package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/memory"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
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

	// 2. Select agent definition. For cross-domain objectives, walk the
	// declared domains in order and pick the first pack that exposes an
	// agent. If none do, fall back to a minimal default tagged with the
	// primary domain.
	var agentDef coreagent.Definition
	if req.Agent.ID != "" {
		agentDef = req.Agent
	} else {
		domains := obj.AllDomains()
		if len(domains) == 0 {
			domains = []string{obj.Domain}
		}
		var picked bool
		for _, d := range domains {
			pack, ok := s.domReg.Get(d)
			if !ok || len(pack.AgentDefinitions()) == 0 {
				continue
			}
			agentDef = pack.AgentDefinitions()[0]
			picked = true
			break
		}
		if !picked {
			agentDef = coreagent.Definition{
				ID:                coreagent.AgentID(obj.Domain + "-default"),
				Name:              obj.Domain + " Agent",
				Domain:            obj.Domain,
				ReasoningStrategy: coreagent.ReasoningReAct,
			}
		}
	}

	// 3. Create the agent
	agent, err := s.factory.New(ctx, agentDef)
	if err != nil {
		s.finalizeLoop(ctx, state, obj, nil, false, fmt.Errorf("create agent: %w", err))
		return
	}

	// 4. Build all environments for the domain. Fetch twin bindings so envs
	// can resolve the correct adapter instance per tenant (ADR 006).
	var adapterBindings map[string]string
	if obj.TwinID != "" {
		if t, terr := s.store.GetTwin(ctx, obj.TwinID); terr == nil {
			adapterBindings = t.AdapterBindings
		}
	}
	buildCtx := environment.BuildContext{TwinID: obj.TwinID, AdapterBindings: adapterBindings}
	var envs []environment.Environment
	seenEnv := make(map[string]bool)
	for _, d := range obj.AllDomains() {
		for _, fac := range s.envReg.ListByDomain(d) {
			key := string(fac.EnvID)
			if seenEnv[key] {
				continue
			}
			seenEnv[key] = true
			env, err := fac.Build(buildCtx)
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

		p, paused := stepDecide(ctx, sc, p, nil)
		if paused {
			state.mu.Lock()
			state.status.Paused = true
			state.mu.Unlock()

			// Persist the paused state so a server restart picks up the loop
			// in the right shape (Phase 11). The decision channel itself
			// remains in-memory — a new Resume() call will recreate it.
			s.persistState(ctx, state, false)

			// Wait for resume signal. Phase 13.5: branch on Choice.
			// Approve falls through with the draft. Reject finalizes
			// the loop. Modify trims actions + runs a revise pass +
			// re-enters stepDecide once; a second escalation
			// auto-rejects to prevent ping-pong.
			var decision corecheckpoint.Decision
			select {
			case <-ctx.Done():
				s.finalizeLoop(ctx, state, obj, iterations, false, ctx.Err())
				return
			case decision = <-state.decisionCh:
			}

			state.mu.Lock()
			state.status.Paused = false
			state.mu.Unlock()

			switch decision.Choice {
			case "reject":
				s.recordCheckpointTerminal(ctx, state, "rejected_at_checkpoint", decision)
				s.finalizeLoop(ctx, state, obj, iterations, false, fmt.Errorf("rejected_at_checkpoint"))
				return
			case "modify":
				revised, modPaused := s.applyModification(ctx, sc, p, decision)
				p = revised
				if modPaused {
					// Re-approval required. Wait once more — the
					// second escalation auto-rejects if it trips again.
					s.persistState(ctx, state, false)
					var second corecheckpoint.Decision
					select {
					case <-ctx.Done():
						s.finalizeLoop(ctx, state, obj, iterations, false, ctx.Err())
						return
					case second = <-state.decisionCh:
					}
					if second.Choice != "approve" {
						s.recordCheckpointTerminal(ctx, state, "modify_loop_exceeded", second)
						s.finalizeLoop(ctx, state, obj, iterations, false, fmt.Errorf("modify_loop_exceeded"))
						return
					}
					state.mu.Lock()
					state.status.Paused = false
					state.mu.Unlock()
				}
			}
			// approve: fall through to act with the (possibly revised) plan.
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

		// Persist progress at iteration boundary so a server crash never loses
		// more than one iteration of work (Phase 11).
		s.persistState(ctx, state, false)

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

// applyModification realizes a Decision.Choice="modify" against the
// current draft (Phase 13.5):
//
//  1. Drops any actions matching Modifications.RemovedActions (one
//     match per ID; duplicates after the first match are kept so the
//     operator can drop a single occurrence of a repeated capability).
//  2. Runs stepReasonRevise with the operator note + AddedConstraints
//     as critique input. Never-regress: the trimmed draft is returned
//     on any revise failure.
//  3. Re-enters stepDecide. If the revised plan still trips bounds
//     (modPaused=true), the caller waits for one more approve/reject
//     and auto-rejects on anything else.
//
// Returns the revised plan and whether re-approval is required.
func (s *serviceImpl) applyModification(ctx context.Context, sc *stepContext, draft plan, decision corecheckpoint.Decision) (plan, bool) {
	draft = trimRemovedActions(draft, decision.Modifications)
	revised, applied := stepReasonRevise(ctx, sc, draft, decision)
	revised, paused := stepDecide(ctx, sc, revised, decision.Modifications)

	sc.svc.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopStepCompleted,
		ObjectiveID: string(sc.obj.ID),
		Payload: map[string]any{
			"step":                  "modify",
			"iteration":             sc.iteration,
			"modify_revise_applied": applied,
			"modify_re_escalated":   paused,
			"plan_action_count":     len(revised.Actions),
			"confidence":            revised.Confidence,
		},
		Timestamp: time.Now().UTC(),
	})
	return revised, paused
}

// trimRemovedActions drops each capability ID in Modifications.RemovedActions
// from the draft exactly once (Phase 13.5). Duplicates after the first
// match are kept so an operator can drop a single occurrence of a
// repeated capability. Returns the draft unchanged when mods is nil or
// RemovedActions is empty.
func trimRemovedActions(p plan, mods *corecheckpoint.Modifications) plan {
	if mods == nil || len(mods.RemovedActions) == 0 {
		return p
	}
	remove := make(map[string]int, len(mods.RemovedActions))
	for _, id := range mods.RemovedActions {
		remove[id]++
	}
	kept := make([]plannedAction, 0, len(p.Actions))
	for _, a := range p.Actions {
		if n, ok := remove[a.CapabilityID]; ok && n > 0 {
			remove[a.CapabilityID] = n - 1
			continue
		}
		kept = append(kept, a)
	}
	p.Actions = kept
	return p
}

// recordCheckpointTerminal writes an audit row marking a checkpoint
// resolution that terminates the loop (Phase 13.5). reason is one of
// "rejected_at_checkpoint" or "modify_loop_exceeded".
func (s *serviceImpl) recordCheckpointTerminal(ctx context.Context, state *loopState, reason string, d corecheckpoint.Decision) {
	state.mu.RLock()
	objID := state.status.ObjectiveID
	state.mu.RUnlock()
	payload, _ := json.Marshal(map[string]any{
		"reason":   reason,
		"choice":   d.Choice,
		"note":     d.Note,
		"approver": d.Approver,
	})
	_ = s.store.SaveToolEvent(ctx, storage.ToolEvent{
		ID:               fmt.Sprintf("audit-%d", time.Now().UnixNano()),
		ObjectiveID:      string(objID),
		Kind:             storage.ToolEventRejection,
		EscalationReason: reason,
		Approver:         d.Approver,
		PayloadJSON:      string(payload),
		Success:          false,
	})
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

	// Persist the terminal state (Phase 11): Completed=true keeps it out of
	// the ListActiveLoopStates set used by the restart-resume path.
	s.persistState(ctx, state, true)

	if obj.ID != "" {
		s.hub.Publish(ctx, event.Event{
			Type:        evtType,
			ObjectiveID: string(obj.ID),
			Payload:     map[string]any{"loop_id": state.id, "criteria_met": criteriaMet},
			Timestamp:   time.Now().UTC(),
		})
	}
}
