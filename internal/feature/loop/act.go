package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/platform/git"
	"github.com/bsenel/karakuri/internal/platform/storage"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	"github.com/bsenel/karakuri/quota/cost"
)

// actionOutcome pairs an action with what it produced.
//
// stepAct returns these rather than bare results because a result cannot say
// which capability made it, and the two steps downstream both need to know:
// verify, to match a criterion's verifier against the action that was supposed
// to satisfy it, and learn, to attribute a success rate to the right
// capability.
//
// They were paired by slice index before — stepLearn read results[i] beside
// p.Actions[i] — which holds only while nothing ever skips or reorders. verify
// did not pair them at all: a criterion verified by run_tests was met if *any*
// action succeeded, so an unrelated send_message could satisfy it.
type actionOutcome struct {
	CapabilityID string
	EnvID        string
	Result       environment.ActionResult
}

// results extracts the bare results, for the callers that genuinely do not
// care which action produced which.
func results(outcomes []actionOutcome) []environment.ActionResult {
	out := make([]environment.ActionResult, 0, len(outcomes))
	for _, o := range outcomes {
		out = append(out, o.Result)
	}
	return out
}

func stepAct(ctx context.Context, sc *stepContext, p plan) []actionOutcome {
	// 1. Emit step started
	sc.svc.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopStepStarted,
		ObjectiveID: string(sc.obj.ID),
		Payload: map[string]any{
			"step":      string(loop.StepAct),
			"iteration": sc.iteration,
		},
		Timestamp: time.Now().UTC(),
	})

	outcomes := make([]actionOutcome, 0, len(p.Actions))
	successCount := 0

	for i, action := range p.Actions {
		// a. Find the environment that runs this action.
		targetEnv, routedBy := sc.svc.resolveEnv(sc.envs, action)

		params := action.Params
		if params == nil {
			params = make(map[string]any)
		}

		// a2. Charge the twin's daily allowance for this capability.
		//
		// Before the action rather than after, because the point of the tier is
		// to stop a misconfigured watcher hammering an external service — most
		// capabilities are not model calls, they reach GitHub or Linear, and a
		// charge levied afterwards would have already made the call.
		//
		// A refusal skips the action and records why, rather than failing the
		// iteration: the loop can still make progress on whatever else it
		// planned, and the operator sees a bounded twin rather than a broken one.
		if !sc.svc.allowCapability(ctx, sc, action.CapabilityID, i) {
			outcomes = append(outcomes, actionOutcome{
				CapabilityID: action.CapabilityID,
				Result: environment.ActionResult{
					Success: false,
					Error:   "capability quota exhausted for the day",
				},
			})
			continue
		}

		// b. Worktree for capabilities that declare they write files.
		//
		// Asked of the capability registry rather than matched on the name:
		// the suffix test gave a worktree to two capabilities that had no
		// implementation and withheld one from delegate_to_cli, which is the
		// only one that can actually write. See ADR 019.
		if sc.svc.needsWorkspace(action.CapabilityID) {
			taskID := fmt.Sprintf("%s-%d", sc.loopID[:8], i)
			wt, err := sc.svc.wt.Create(ctx, git.WorktreeOptions{
				ObjectiveID: sc.obj.ID,
				TaskID:      taskID,
			})
			if err == nil {
				params["worktree_path"] = wt.Path
				params["branch"] = wt.Branch
				// Persist worktree record
				_ = sc.svc.store.SaveWorktree(ctx, storage.Worktree{
					TaskID:      wt.TaskID,
					ObjectiveID: string(wt.ObjectiveID),
					Path:        wt.Path,
					Branch:      wt.Branch,
					CreatedAt:   wt.CreatedAt,
				})
				sc.svc.hub.Publish(ctx, event.Event{
					Type:        event.TypeWorktreeCreated,
					ObjectiveID: string(sc.obj.ID),
					Payload:     map[string]any{"task_id": taskID, "path": wt.Path, "branch": wt.Branch},
					Timestamp:   time.Now().UTC(),
				})
			}
		}

		var result environment.ActionResult
		if targetEnv != nil {
			var err error
			result, err = targetEnv.Act(ctx, environment.Action{
				CapabilityID: capability.CapabilityID(action.CapabilityID),
				Params:       params,
			})
			if err != nil {
				result = environment.ActionResult{
					Success: false,
					Error:   err.Error(),
				}
			}
		} else {
			// Nothing declared it serves this capability, and the plan's
			// env_id matched nothing either. Used to silently succeed with
			// Success=true; now fails honestly so the audit trail and verify
			// step see the gap instead of treating it as work done.
			available := make([]string, 0, len(sc.envs))
			for _, env := range sc.envs {
				available = append(available, string(env.ID()))
			}

			// Which failure this is decides what an operator does about it, so
			// the message has to distinguish them rather than defaulting to
			// "no environment matches env_id" and sending whoever reads it
			// looking in the wrong place.
			var serves []string
			activeServers := 0
			if sc.svc.envReg != nil {
				for _, id := range sc.svc.envReg.ServedBy(capability.CapabilityID(action.CapabilityID)) {
					serves = append(serves, string(id))
					for _, env := range sc.envs {
						if env.ID() == id {
							activeServers++
							break
						}
					}
				}
			}

			var errMsg string
			switch {
			case len(serves) == 0:
				// Nothing anywhere claims it: the plan named a capability no
				// enabled pack serves.
				errMsg = fmt.Sprintf("no environment matches env_id=%q (available: %v)", action.EnvID, available)
			case activeServers == 0:
				// A pack to enable or an adapter to bind.
				errMsg = fmt.Sprintf("capability %q is served by %v, none of which this twin has active (available: %v)",
					action.CapabilityID, serves, available)
			default:
				// Several active environments claim it, so routing declined to
				// choose and the plan's env_id matched nothing. The pack is
				// what needs fixing; conformance names it.
				errMsg = fmt.Sprintf("capability %q is served by %v — more than one, so routing deferred to env_id=%q, which matches no active environment (available: %v)",
					action.CapabilityID, serves, action.EnvID, available)
			}

			result = environment.ActionResult{
				Success: false,
				Error:   errMsg,
				StateDelta: map[string]any{
					"capability": action.CapabilityID,
					"status":     "unrouted",
					"env_id":     action.EnvID,
					"available":  available,
					"served_by":  serves,
				},
			}
			sc.svc.hub.Publish(ctx, event.Event{
				Type:        event.TypeAdapterSkipped,
				ObjectiveID: string(sc.obj.ID),
				Payload: map[string]any{
					"capability": action.CapabilityID,
					"reason":     "unrouted",
					"env_id":     action.EnvID,
					"available":  available,
					"served_by":  serves,
				},
				Timestamp: time.Now().UTC(),
			})
		}

		// d. Emit artifact_written if blobs produced
		if len(result.ArtifactSHAs) > 0 {
			sc.svc.hub.Publish(ctx, event.Event{
				Type:        event.TypeArtifactWritten,
				ObjectiveID: string(sc.obj.ID),
				Payload:     map[string]any{"shas": result.ArtifactSHAs, "capability": action.CapabilityID},
				Timestamp:   time.Now().UTC(),
			})
		}

		if result.Success {
			successCount++
		}

		// f. Save ToolEvent. routed_by records which rule picked the
		// environment, because "the pack said so" and "the model said so" are
		// different claims about the same successful action, and the second
		// one is the one worth noticing when it stops being true.
		payloadJSON, _ := json.Marshal(map[string]any{
			"params":    params,
			"result":    result,
			"routed_by": routedBy,
		})
		agentIDStr := string(sc.agentDef.ID)
		envAdapter := ""
		if targetEnv != nil {
			envAdapter = string(targetEnv.ID())
		}
		_ = sc.svc.store.SaveToolEvent(ctx, storage.ToolEvent{
			ID:          fmt.Sprintf("te-%d-%d", time.Now().UnixNano(), i),
			ObjectiveID: string(sc.obj.ID),
			AgentID:     agentIDStr,
			Capability:  action.CapabilityID,
			Adapter:     envAdapter,
			Success:     result.Success,
			Confidence:  p.Confidence,
			PayloadJSON: string(payloadJSON),
			CreatedAt:   time.Now().UTC(),
		})

		// g. And what it cost. Attributed to the objective rather than the twin,
		// because "which piece of work spent this" is the question a bill
		// prompts — the twin is still the subject that pays, and its containers
		// are what a per-team report groups on.
		sc.svc.costs.Record(ctx, karakuriquota.Spend{
			TwinID:       sc.twinID,
			ResourceType: "objective",
			ResourceID:   string(sc.obj.ID),
			Provider:     envAdapter,
			Units:        1,
			UnitKind:     cost.UnitCalls,
		})

		outcomes = append(outcomes, actionOutcome{
			CapabilityID: action.CapabilityID,
			EnvID:        envAdapter,
			Result:       result,
		})
	}

	// 3. Emit step completed
	successRate := 0.0
	if len(p.Actions) > 0 {
		successRate = float64(successCount) / float64(len(p.Actions))
	}
	sc.svc.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopStepCompleted,
		ObjectiveID: string(sc.obj.ID),
		Payload: map[string]any{
			"step":         string(loop.StepAct),
			"action_count": len(p.Actions),
			"success_rate": successRate,
		},
		Timestamp: time.Now().UTC(),
	})

	return outcomes
}

// resolveEnv picks the environment that will run an action, and reports which
// of the three rules chose it.
//
// The order is deliberate. A capability exactly one environment declares it
// serves goes there whatever the plan said, because that is not a choice: the
// pack already answered it, and the planner is the one party in the exchange
// that does not know. Deferring to Action.EnvID there is what let a plan that
// wrote code without naming software.env.cli_agent reach noopEnv and report
// "unimplemented" — the model got the routing wrong, and the system treated
// its guess as authoritative. See ADR 019.
//
// EnvID still decides when the registry cannot: a capability nothing claims,
// or one two environments both claim. Ambiguity is not resolved by picking —
// conformance fails the pack that created it, and until then the plan's
// preference is better than map iteration order.
func (s *serviceImpl) resolveEnv(envs []environment.Environment, action plannedAction) (environment.Environment, string) {
	byID := func(id string) environment.Environment {
		for _, env := range envs {
			if string(env.ID()) == id {
				return env
			}
		}
		return nil
	}

	// 1. What the pack declared, narrowed to the environments this loop built.
	var claimed []environment.EnvironmentID
	if s.envReg != nil {
		claimed = s.envReg.ServedBy(capability.CapabilityID(action.CapabilityID))
		var matched []environment.Environment
		for _, id := range claimed {
			if env := byID(string(id)); env != nil {
				matched = append(matched, env)
			}
		}
		if len(matched) == 1 {
			return matched[0], "capability"
		}
		// Something serves this capability and none of it was built here: the
		// twin has not enabled that pack, or its adapter is unbound. That is a
		// missing environment, not a free choice — falling through to the
		// plan's env_id would run a capability on an environment its own pack
		// never said could run it, which is the fall-through this replaced.
		if len(matched) == 0 && len(claimed) > 0 {
			return nil, "unrouted"
		}
	}

	// 2. What the plan asked for.
	if action.EnvID != "" {
		if env := byID(action.EnvID); env != nil {
			return env, "env_id"
		}
	}

	// 3. Nothing claims it, no EnvID given, and only one environment built —
	// unambiguous.
	if action.EnvID == "" && len(envs) == 1 {
		return envs[0], "sole_env"
	}

	return nil, "unrouted"
}

// needsWorkspace reports whether a capability declared that it writes files.
//
// Unknown capabilities get no workspace: a capability the registry has never
// heard of is one no pack declared, and provisioning a git worktree for a
// name a model invented would create a branch per hallucination.
func (s *serviceImpl) needsWorkspace(capID string) bool {
	if s.capReg == nil {
		return false
	}
	cap, ok := s.capReg.Get(capability.CapabilityID(capID))
	return ok && cap.NeedsWorkspace
}
