package loop

import (
	"context"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	featureobj "github.com/bsenel/karakuri/internal/feature/objective"
)

// Phase 41 step 7 part D2: an at_start measurement that was inconclusive
// leaves the criterion inconclusive: no_baseline until a person sets the
// baseline through the API. Once they have, the next verify settles it from
// that value by Compare, met or not_met, and at_start neither measures it
// again nor overwrites it (docs/roadmap.md, Phase 41 decision 3 'Baselines'
// and step 7's acceptance: "A baseline a person sets through the API is
// audited and then settles it").
func TestABaselineAPersonSetsSettlesANoBaselineCriterion(t *testing.T) {
	for _, tc := range []struct {
		name  string
		later float64
		want  objective.CriterionState
	}{
		// 215 is 7.5% above the person's 200, within the 10% target.
		{name: "within the target", later: 215, want: objective.CriterionMet},
		// 230 is 15% above it.
		{name: "past the target", later: 230, want: objective.CriterionNotMet},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := &atStartEnvFake{
				commit: "c0ffee1",
				// The at_start measurement reports nothing; the first plan's
				// reading is 190, and every later one is tc.later.
				verifier: []verifierReply{
					{result: environment.ActionResult{Success: true}},
					latencyMeasured(190),
					latencyMeasured(tc.later),
				},
				work: environment.ActionResult{Success: true},
			}
			svc, store, obj := atStartFixture(t, env, atStartPlan)

			runAtStart(t, svc, obj, "loop-at-start-person-1", 1)
			st := finalLoopState(t, store, "loop-at-start-person-1")
			if got := criterionByID(t, st.Criteria, "latency"); got.State != objective.CriterionInconclusive || got.Reason != objective.ReasonNoBaseline {
				t.Fatalf("before the person sets it: state = %q, reason = %q; want inconclusive: no_baseline", got.State, got.Reason)
			}

			if _, err := featureobj.NewService(store).SetBaseline(context.Background(), obj.ID, "latency", 200, "alice@example.com"); err != nil {
				t.Fatalf("set baseline: %v", err)
			}

			env.setCommit("beefcafe")
			runAtStart(t, svc, obj, "loop-at-start-person-2", 1)

			st = finalLoopState(t, store, "loop-at-start-person-2")
			if got := criterionByID(t, st.Criteria, "latency"); got.State != tc.want {
				t.Errorf("%v against the person's 200 +10%%: state = %q (reason %q), want %q",
					tc.later, got.State, got.Reason, tc.want)
			}
			if c := storedLatency(t, store, obj.ID); c.BaselineValue == nil || *c.BaselineValue != 200 {
				t.Errorf("stored baseline_value = %s after the next run, want the person's 200: at_start never overwrites it", frozenAt(c))
			}
			if got := env.verifierCalls(); got != 3 {
				t.Errorf("verifier ran %d times over two one-iteration runs, want 3: "+
					"one at_start measurement and one per plan, never a second at_start measurement (calls %v)", got, env.actCalls())
			}

			// The loop's inconclusive at_start row and the person's row.
			records := baselineRecords(t, store, obj.ID)
			if len(records) != 2 {
				t.Fatalf("%d baseline audit rows, want 2, the at_start attempt and the person's set: %v", len(records), records)
			}
			var person map[string]any
			for _, r := range records {
				if _, ok := r["set_by"]; ok {
					person = r
				}
			}
			if person == nil {
				t.Fatalf("no baseline row names the person who set it: %v", records)
			}
			if person["set_by"] != "alice@example.com" || person["value"] != 200.0 || person["criterion"] != "latency" {
				t.Errorf("person's baseline row = %v, want criterion latency, value 200, set_by alice@example.com", person)
			}
		})
	}
}
