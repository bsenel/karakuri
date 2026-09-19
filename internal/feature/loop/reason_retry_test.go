package loop

import (
	"context"
	"strings"
	"testing"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/loop"
)

const goodPlan = `{"actions":[{"capability":"software.act.shell_exec","params":{"cmd":"go test ./..."},` +
	`"reason":"verify","env_id":"software.env.shell"}],"confidence":0.8,"reasoning":"ok"}`

// The failure this fixes: a reply that is not JSON produced a placeholder
// carrying the model's prose under a capability nothing serves, and the loop
// then spent a whole iteration acting on it, verifying it and asking a human
// about it. Roughly half of reason steps did this in practice.
func TestStepReasonRetriesOnceWhenThePlanDoesNotParse(t *testing.T) {
	agent := &scriptedAgent{scripted: []coreagent.Output{
		{Content: "I'll start by reading the roadmap, then create a worktree.", Confidence: 0.7},
		{Content: goodPlan, Confidence: 0.8},
	}}
	sc := scriptedReasonContext(t, agent)

	p := stepReason(context.Background(), sc, loop.WorldState{})

	if len(p.Actions) != 1 {
		t.Fatalf("expected the retry's plan to land, got %d actions", len(p.Actions))
	}
	if p.Actions[0].CapabilityID != "software.act.shell_exec" {
		t.Errorf("capability = %q, want the retried plan's action", p.Actions[0].CapabilityID)
	}
	if p.Actions[0].CapabilityID == "reason.plan" {
		t.Error("fell back to the prose placeholder despite a usable retry")
	}
}

// The retry must show the model what it did wrong. Re-asking the original
// question would repeat whatever produced prose the first time.
func TestRetryQuotesTheUnusableReply(t *testing.T) {
	agent := &scriptedAgent{scripted: []coreagent.Output{
		{Content: "here is a thought, not a plan", Confidence: 0.7},
		{Content: goodPlan, Confidence: 0.8},
	}}
	sc := scriptedReasonContext(t, agent)

	stepReason(context.Background(), sc, loop.WorldState{})

	if len(agent.tasksSeen) < 2 {
		t.Fatalf("expected a second call, saw %d", len(agent.tasksSeen))
	}
	retry := agent.tasksSeen[1]
	if !strings.Contains(retry, "could not be parsed") {
		t.Error("retry prompt must say the previous reply was rejected")
	}
	if !strings.Contains(retry, "here is a thought, not a plan") {
		t.Error("retry prompt must quote the unusable reply back")
	}
}

// Two failures is the model's answer, and the placeholder is the honest record
// of it. One retry, not a loop.
func TestStepReasonFallsBackAfterOneFailedRetry(t *testing.T) {
	agent := &scriptedAgent{scripted: []coreagent.Output{
		{Content: "prose one", Confidence: 0.7},
		{Content: "prose two", Confidence: 0.7},
	}}
	sc := scriptedReasonContext(t, agent)

	p := stepReason(context.Background(), sc, loop.WorldState{})

	if len(p.Actions) != 1 || p.Actions[0].CapabilityID != "reason.plan" {
		t.Fatalf("expected the prose placeholder after two failures, got %+v", p.Actions)
	}
	if len(agent.tasksSeen) != 2 {
		t.Errorf("made %d calls; the retry must happen exactly once", len(agent.tasksSeen))
	}
}

// A first reply that parses must not cost a second call.
func TestStepReasonDoesNotRetryWhenTheFirstPlanParses(t *testing.T) {
	agent := &scriptedAgent{scripted: []coreagent.Output{{Content: goodPlan, Confidence: 0.8}}}
	sc := scriptedReasonContext(t, agent)

	stepReason(context.Background(), sc, loop.WorldState{})

	if len(agent.tasksSeen) != 1 {
		t.Errorf("made %d calls; a parseable plan must not trigger a retry", len(agent.tasksSeen))
	}
}
