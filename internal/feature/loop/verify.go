package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
)

func stepVerify(ctx context.Context, sc *stepContext, outcomes []actionOutcome) (float64, bool) {
	// 1. Emit step started
	sc.svc.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopStepStarted,
		ObjectiveID: string(sc.obj.ID),
		Payload: map[string]any{
			"step":      string(loop.StepVerify),
			"iteration": sc.iteration,
		},
		Timestamp: time.Now().UTC(),
	})

	criteria := sc.obj.SuccessCriteria

	// No criteria → refuse to score completion. Previously this path
	// returned 1.0/true unconditionally, which is how the Phase 13.5
	// dogfood "completed" an objective whose 17 actions had all been
	// no-ops: no criteria, trivial pass, falsified status. Honest
	// failure: returns 0.0/false so the loop iterates (or finalizes
	// as Failed) and operators add criteria before retrying.
	if len(criteria) == 0 {
		sc.svc.hub.Publish(ctx, event.Event{
			Type:        event.TypeLoopStepCompleted,
			ObjectiveID: string(sc.obj.ID),
			Payload: map[string]any{
				"step":               string(loop.StepVerify),
				"criteria_met_count": 0,
				"weighted_score":     0.0,
				"reason":             "no_success_criteria_defined",
			},
			Timestamp: time.Now().UTC(),
		})
		return 0.0, false
	}

	totalWeight := 0.0
	metWeight := 0.0
	metCount := 0

	// Per-domain weighted score tracking for cross-domain objectives. A
	// criterion's Domain field opts it into a sub-bucket; criteria without
	// a domain only count toward the aggregate. The aggregate is still the
	// authoritative gate for completion — sub-scores are emitted for
	// observers (checkpoints, dashboards, learn step).
	domainTotal := make(map[string]float64)
	domainMet := make(map[string]float64)

	for i, criterion := range criteria {
		weight := criterion.Weight
		if weight == 0 {
			weight = 1.0
		}
		totalWeight += weight
		if criterion.Domain != "" {
			domainTotal[criterion.Domain] += weight
		}

		met := false
		verifier := string(criterion.Verifier)

		// A criterion naming a verifier is settled by the action that ran that
		// verifier, when one did. Deterministic beats asking a model, and it
		// is the only reading of "verified by run_tests" that means anything.
		//
		// It used to be "met if any action succeeded", for verifiers whose ID
		// merely *contained* run_tests or lint — so a criterion about the test
		// suite was satisfied by an unrelated send_message, and only for two
		// hard-coded substrings. Everything else went to a model that was
		// shown no results at all.
		ran := outcomesFor(outcomes, verifier)
		switch {
		case verifier != "" && len(ran) > 0:
			// Every run of it has to have succeeded. One failing test run is
			// a failing test run, however many passed beside it.
			met = true
			for _, o := range ran {
				if !o.Result.Success {
					met = false
					break
				}
			}
		default:
			// No verifier declared, or the declared one never ran. The agent
			// judges — now from the actual outcomes, which is the difference
			// between a judgement and a guess.
			met = evaluateWithAgent(ctx, sc, criterion, outcomes)
		}

		criteria[i].Met = met
		if met {
			metWeight += weight
			metCount++
			if criterion.Domain != "" {
				domainMet[criterion.Domain] += weight
			}
		}
	}

	// 3. Compute weighted score
	score := 0.0
	if totalWeight > 0 {
		score = metWeight / totalWeight
	}
	allMet := metCount == len(criteria)

	// 4. Emit step completed
	payload := map[string]any{
		"step":               string(loop.StepVerify),
		"criteria_met_count": metCount,
		"weighted_score":     score,
	}
	if len(domainTotal) > 0 {
		perDomain := make(map[string]float64, len(domainTotal))
		for d, total := range domainTotal {
			if total > 0 {
				perDomain[d] = domainMet[d] / total
			}
		}
		payload["per_domain_score"] = perDomain
	}
	sc.svc.hub.Publish(ctx, event.Event{
		Type:        event.TypeLoopStepCompleted,
		ObjectiveID: string(sc.obj.ID),
		Payload:     payload,
		Timestamp:   time.Now().UTC(),
	})

	return score, allMet
}

// evaluateWithAgent asks the agent whether a criterion is met, given what the
// actions actually produced.
//
// The prompt said "based on the action results" and the results were never
// included: WorldState and Memory were nil and the results parameter was
// accepted and ignored. So every unverifiable criterion in every domain was
// scored by a model shown nothing but the criterion's own description — it
// could only guess, and a plausible-sounding criterion scored well for being
// plausible. The criterion that made this urgent is `self_improve`'s "the
// proposal names the telemetry that says the problem is real", which was
// judged by a model that had seen neither the telemetry nor the proposal.
func evaluateWithAgent(ctx context.Context, sc *stepContext, criterion objective.Criterion, outcomes []actionOutcome) bool {
	// No actions means nothing to judge. Asking anyway invites a model to
	// reason from the criterion's plausibility, which is how an objective
	// scores well for having done nothing.
	if len(outcomes) == 0 {
		return false
	}

	task := fmt.Sprintf(
		"Evaluate whether this criterion is met, based only on the action results below.\n\n"+
			"Criterion: %s\n\n"+
			"Action results:\n%s\n\n"+
			"If the results do not show the criterion being met, answer FAIL — "+
			"absence of evidence is not evidence it was met.\n"+
			"Answer with exactly one word: PASS or FAIL.",
		criterion.Description,
		renderOutcomes(outcomes),
	)

	input := coreagent.Input{
		Objective: sc.obj,
		// The results are in the task rather than WorldState because
		// WorldState is the observed world at the start of the iteration —
		// what was true before the actions ran, which is the wrong half of a
		// before-and-after judgement.
		WorldState: nil,
		Memory:     nil,
		Task:       task,
	}

	output, err := sc.agent.Run(ctx, input)
	if err != nil {
		return false
	}
	return verdictIsPass(output.Content)
}

// renderOutcomes turns the outcomes into the evidence the judge reads. Each
// line names the capability, because "action 3 succeeded" tells a model less
// than it needs to decide whether the thing the criterion asks about happened.
//
// Deliberately plain and bounded: a criterion is judged on what happened, and
// a payload large enough to push the criterion out of the model's attention
// would defeat the purpose of including it at all.
func renderOutcomes(outcomes []actionOutcome) string {
	const maxDeltaChars = 600

	var sb strings.Builder
	for i, o := range outcomes {
		r := o.Result
		status := "FAILED"
		if r.Success {
			status = "succeeded"
		}
		fmt.Fprintf(&sb, "%d. %s: %s", i+1, o.CapabilityID, status)
		if r.Error != "" {
			fmt.Fprintf(&sb, " — error: %s", r.Error)
		}
		if len(r.StateDelta) > 0 {
			delta, err := json.Marshal(r.StateDelta)
			if err == nil {
				text := string(delta)
				if len(text) > maxDeltaChars {
					text = text[:maxDeltaChars] + "…(truncated)"
				}
				fmt.Fprintf(&sb, "\n   produced: %s", text)
			}
		}
		if len(r.ArtifactSHAs) > 0 {
			fmt.Fprintf(&sb, "\n   artifacts: %v", r.ArtifactSHAs)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// verdictIsPass reads the judge's answer.
//
// The old version searched the whole reply for "pass", "met", "approved" or
// "yes" and returned true on any hit, so "this does not pass" scored as met and
// "the criterion is not met" scored as met — a negated verdict counted as a
// positive one, which is the worst possible direction for a scoring bug to
// fail in.
//
// It now reads the first word, which is what the prompt asks for, and falls
// back to an explicit-negation check rather than to a substring search: an
// answer nobody can parse is a FAIL, because scoring a criterion met on an
// unparseable reply is exactly the silent success this codebase keeps finding.
func verdictIsPass(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	if lower == "" {
		return false
	}

	// The requested form: one word, first.
	fields := strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r)
	})
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "pass", "passed", "met", "yes", "true", "approved":
		return true
	case "fail", "failed", "no", "false", "not", "unmet", "rejected":
		return false
	}

	// A discursive answer. Require a positive marker AND no negation of it,
	// rather than accepting any positive substring.
	negated := []string{
		"not met", "not pass", "does not", "doesn't", "did not", "didn't",
		"cannot", "can't", "no evidence", "insufficient", "unmet", "fail",
	}
	for _, n := range negated {
		if strings.Contains(lower, n) {
			return false
		}
	}
	for _, p := range []string{"pass", "met", "approved", "yes"} {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// outcomesFor returns the outcomes produced by a named capability.
//
// Matched on the whole ID rather than a substring. The substring test — which
// only ever covered "run_tests" and "lint" — would have matched a pack's
// software.verify.run_tests against another pack's identically-suffixed one,
// and matched nothing at all for every other verifier a pack might declare.
func outcomesFor(outcomes []actionOutcome, capabilityID string) []actionOutcome {
	if capabilityID == "" {
		return nil
	}
	var out []actionOutcome
	for _, o := range outcomes {
		if o.CapabilityID == capabilityID {
			out = append(out, o)
		}
	}
	return out
}
