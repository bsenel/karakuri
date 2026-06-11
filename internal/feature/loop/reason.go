package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/loop"
)

type plannedAction struct {
	CapabilityID string         `json:"capability"`
	Params       map[string]any `json:"params"`
	Reason       string         `json:"reason"`
	EnvID        string         `json:"env_id"`
}

type plan struct {
	Actions    []plannedAction `json:"actions"`
	Confidence float64         `json:"confidence"`
	Reasoning  string          `json:"reasoning"`
}

func stepReason(ctx context.Context, sc *stepContext, ws loop.WorldState) plan {
	// 1. Emit step started
	sc.svc.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopStepStarted,
		ObjectiveID: string(sc.obj.ID),
		Payload: map[string]any{
			"step":      string(loop.StepReason),
			"iteration": sc.iteration,
		},
		Timestamp: time.Now().UTC(),
	})

	// 2. Build agent input
	memEntries := make([]coreagent.MemoryEntry, len(sc.memEntries))
	for i, e := range sc.memEntries {
		memEntries[i] = e
	}

	input := coreagent.Input{
		Objective:  sc.obj,
		WorldState: ws,
		Memory:     memEntries,
		Task: "Plan the next actions to make progress on this objective. " +
			"Return a JSON object with 'actions' (array of {capability, params, reason, env_id}), " +
			"'confidence' (0.0-1.0), and 'reasoning' (string). " +
			"Return raw JSON only — no Markdown code fences, no commentary before or after.",
	}

	// 3. Call agent
	output, err := sc.agent.Run(ctx, input)

	var p plan
	if err == nil {
		// 4. Try to parse output as JSON plan. Tolerates Markdown code
		// fences and leading/trailing prose — most chat models default to
		// wrapping JSON in ```json … ``` even when asked not to.
		cleaned := extractJSON(output.Content)
		if jsonErr := json.Unmarshal([]byte(cleaned), &p); jsonErr != nil {
			// Fallback: create default plan
			p = plan{
				Actions: []plannedAction{
					{
						CapabilityID: "reason.plan",
						Params:       map[string]any{"content": output.Content},
						Reason:       "Agent produced non-JSON output; wrapping as reasoning action",
					},
				},
				Confidence: 0.7,
				Reasoning:  output.Content,
			}
		}
	} else {
		// On error create a minimal plan
		p = plan{
			Actions: []plannedAction{
				{
					CapabilityID: "reason.plan",
					Params:       map[string]any{"error": err.Error()},
					Reason:       "Agent call failed",
				},
			},
			Confidence: 0.3,
			Reasoning:  "Agent call failed: " + err.Error(),
		}
	}

	// 5. Use output confidence if plan has none set
	if p.Confidence == 0 && err == nil {
		p.Confidence = output.Confidence
	}

	// 5a. Reflexion strategy: self-critique pass + revision pass.
	// Only applied when the agent declares ReasoningReflexion and the first
	// pass succeeded. The critique runs over the draft plan; the revision
	// pass receives the critique and is asked to produce a refined plan.
	// A failure in either pass falls back to the original plan — Reflexion
	// is additive, never regressive.
	revised, refl := false, ""
	if err == nil && sc.agentDef.ReasoningStrategy == coreagent.ReasoningReflexion {
		if rp, critique, ok := reflexionPass(ctx, sc, p); ok {
			p = rp
			revised = true
			refl = critique
		}
	}

	// 6. Emit step completed
	payload := map[string]any{
		"step":             string(loop.StepReason),
		"plan_action_count": len(p.Actions),
		"confidence":       p.Confidence,
	}
	if revised {
		payload["reflexion_applied"] = true
		payload["reflexion_critique"] = refl
	}
	sc.svc.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopStepCompleted,
		ObjectiveID: string(sc.obj.ID),
		Payload:     payload,
		Timestamp:   time.Now().UTC(),
	})

	return p
}

// reflexionPass runs a two-stage self-correction on top of an initial plan:
// (1) ask the agent to critique its own draft, (2) ask it to produce a
// revised plan informed by that critique. Returns the revised plan, the
// critique text, and ok=true only if both stages produced parseable JSON
// (for the revision) and non-empty text (for the critique). Anything else
// falls back to the caller's draft — Reflexion never makes the plan worse
// than the baseline ChainOfThought output.
func reflexionPass(ctx context.Context, sc *stepContext, draft plan) (plan, string, bool) {
	draftJSON, _ := json.Marshal(draft)

	critiqueTask := fmt.Sprintf(
		"You produced this draft plan for the objective %q:\n\n%s\n\n"+
			"Critique it: identify the weakest assumption, the most likely "+
			"failure mode, and any missing step. Respond with a single "+
			"paragraph — no JSON, no bullet list.",
		sc.obj.Title, string(draftJSON),
	)
	critOut, err := sc.agent.Run(ctx, coreagent.Input{
		Objective:  sc.obj,
		WorldState: nil,
		Memory:     nil,
		Task:       critiqueTask,
	})
	if err != nil || critOut.Content == "" {
		return draft, "", false
	}

	reviseTask := fmt.Sprintf(
		"Given this draft plan:\n\n%s\n\nAnd this critique:\n\n%s\n\n"+
			"Produce a revised plan in the same JSON shape as before "+
			"({actions, confidence, reasoning}). Keep what works; fix the "+
			"weaknesses called out in the critique.",
		string(draftJSON), critOut.Content,
	)
	revOut, err := sc.agent.Run(ctx, coreagent.Input{
		Objective:  sc.obj,
		WorldState: nil,
		Memory:     nil,
		Task:       reviseTask,
	})
	if err != nil {
		return draft, critOut.Content, false
	}
	var revised plan
	cleanedRev := extractJSON(revOut.Content)
	if jsonErr := json.Unmarshal([]byte(cleanedRev), &revised); jsonErr != nil {
		return draft, critOut.Content, false
	}
	if len(revised.Actions) == 0 {
		// Revision is unusable — keep the draft.
		return draft, critOut.Content, false
	}
	if revised.Confidence == 0 {
		revised.Confidence = revOut.Confidence
	}
	return revised, critOut.Content, true
}

// stepReasonRevise runs a single Reflexion-style revise pass driven by an
// operator's modify decision (Phase 13.5). The note + AddedConstraints
// become the critique input — semantically identical to the agent's own
// self-critique in reflexionPass, but human-authored.
//
// Inputs:
//
//	draft — the plan the operator was shown, AFTER any RemovedActions
//	        have been trimmed by the caller.
//	dec   — the operator decision; must have Choice == "modify".
//
// The function:
//
//  1. Skips entirely if there's nothing to feed the agent (no note + no
//     constraints) — returns the trimmed draft unchanged.
//  2. Calls the agent with the critique prompt, asking for the same
//     {actions, confidence, reasoning} shape.
//  3. Falls back to the trimmed draft if the agent errors, returns
//     unparseable JSON, or returns zero actions. Never-regress: a modify
//     pass can only improve the plan, never replace it with something
//     worse than what the operator already saw.
//
// Note: RevisedConfidence does NOT touch the plan's confidence here.
// It's consumed by stepDecide as a per-iteration threshold override —
// see effectiveThreshold in decide.go.
//
// The returned bool indicates whether the revision was actually applied
// (true) or the draft was kept (false); the caller uses this to emit the
// right telemetry on the step_completed event.
func stepReasonRevise(ctx context.Context, sc *stepContext, draft plan, dec corecheckpoint.Decision) (plan, bool) {
	if dec.Choice != "modify" {
		return draft, false
	}
	mods := dec.Modifications
	hasNote := strings.TrimSpace(dec.Note) != ""
	hasConstraints := mods != nil && len(mods.AddedConstraints) > 0
	if !hasNote && !hasConstraints {
		// Nothing to feed the agent. The caller has already trimmed
		// RemovedActions; emit the trimmed draft unchanged.
		return draft, false
	}

	draftJSON, _ := json.Marshal(draft)

	var critique strings.Builder
	if hasNote {
		critique.WriteString(strings.TrimSpace(dec.Note))
	}
	if hasConstraints {
		if critique.Len() > 0 {
			critique.WriteString("\n\n")
		}
		critique.WriteString("Additional constraints:")
		for _, c := range mods.AddedConstraints {
			critique.WriteString("\n- ")
			critique.WriteString(strings.TrimSpace(c))
		}
	}

	reviseTask := fmt.Sprintf(
		"You produced this draft plan for the objective %q:\n\n%s\n\n"+
			"The operator reviewed it and provided this feedback:\n\n%s\n\n"+
			"Produce a revised plan in the same JSON shape "+
			"({actions, confidence, reasoning}). Honor the feedback "+
			"literally — drop or replace actions the operator flagged, "+
			"respect every stated constraint. Keep what works.",
		sc.obj.Title, string(draftJSON), critique.String(),
	)
	revOut, err := sc.agent.Run(ctx, coreagent.Input{
		Objective:  sc.obj,
		WorldState: nil,
		Memory:     nil,
		Task:       reviseTask,
	})
	if err != nil {
		return draft, false
	}
	var revised plan
	cleaned := extractJSON(revOut.Content)
	if jsonErr := json.Unmarshal([]byte(cleaned), &revised); jsonErr != nil {
		return draft, false
	}
	if len(revised.Actions) == 0 {
		return draft, false
	}
	if revised.Confidence == 0 {
		revised.Confidence = revOut.Confidence
	}
	return revised, true
}

// extractJSON returns the JSON payload from agent output, tolerating
// Markdown code fences and surrounding prose. Two stages:
//
//  1. If the content starts with a Markdown fence (```json … ``` or
//     ``` … ```), strip the opening fence + language tag and the
//     closing fence.
//  2. If the remaining content has prose around a JSON object/array,
//     find the first { or [ and the matching last } or ] and return
//     just that substring.
//
// Returns the original (trimmed) input when neither pattern matches, so
// downstream json.Unmarshal can still produce a meaningful parse error
// that surfaces to the fallback path.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		rest := strings.TrimPrefix(s, "```")
		// Drop the optional language tag — either everything up to the
		// first newline (``` json\n{…}\n``` style) or, when the entire
		// fence is on one line, a leading "json" / "JSON" token.
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			rest = rest[nl+1:]
		} else {
			rest = strings.TrimPrefix(rest, "json")
			rest = strings.TrimPrefix(rest, "JSON")
		}
		if end := strings.LastIndex(rest, "```"); end >= 0 {
			rest = rest[:end]
		}
		s = strings.TrimSpace(rest)
	}
	// Fallback: scan for the first { or [ and its matching closing brace
	// in case the model wrapped JSON in prose without a fence.
	if i := strings.IndexAny(s, "{["); i > 0 {
		open := s[i]
		close := byte('}')
		if open == '[' {
			close = ']'
		}
		if j := strings.LastIndexByte(s, close); j > i {
			return s[i : j+1]
		}
	}
	return s
}
