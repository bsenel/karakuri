package loop

import (
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
)

// Phase 41 step 8 part C: only a person's change through the API restarts a
// criterion's sustain window (docs/roadmap.md, Phase 41 section 6 'Targets
// belong to their author'). The loop's own at_start freeze is the baseline the
// criterion was created to be measured against, not a change to it, so it
// leaves sustain_since unset.
func TestAnAtStartFreezeDoesNotRestartTheSustainWindow(t *testing.T) {
	env := &atStartEnvFake{
		commit:   "c0ffee1",
		verifier: []verifierReply{latencyMeasured(200), latencyMeasured(215)},
		work:     environment.ActionResult{Success: true},
	}
	svc, store, obj := atStartFixture(t, env, atStartPlan)

	runAtStart(t, svc, obj, "loop-at-start-sustain-1", 1)

	c := storedLatency(t, store, obj.ID)
	if c.BaselineValue == nil || *c.BaselineValue != 200 {
		t.Fatalf("stored baseline_value = %s, want 200 frozen at start", frozenAt(c))
	}
	if c.SustainSince != nil {
		t.Errorf("sustain_since = %v after the loop's at_start freeze, want none: only a person's change restarts the window", *c.SustainSince)
	}
}
