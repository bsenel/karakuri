package loop

import (
	"context"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
)

// Phase 44 step 3, "plan from what is delivered", through the steps rather
// than the policy: the observation says phase 44 step 5 is ticked on a pull
// request the deployment opened itself, so provenance has nothing to say and
// the only thing that can stop the plan is the redo.

// deliveredEnv is an environment of the operator's own whose observation says
// one step is delivered.
type deliveredEnv struct {
	provEnv
	delivered []environment.DeliveredWork
}

func (e *deliveredEnv) Observe(ctx context.Context, q environment.ObservationQuery) (environment.Observation, error) {
	obs, err := e.provEnv.Observe(ctx, q)
	obs.Delivered = e.delivered
	return obs, err
}

// deliveredFixture observes an environment whose checklist ticks phase 44
// step 5 on pull request #12, with write_code declared as writing files and
// review_pr as not.
func deliveredFixture(t *testing.T) *stepContext {
	t.Helper()
	sc := observeFixture(t, &deliveredEnv{
		provEnv: provEnv{id: "test.env.git", trust: environment.TrustOperator},
		delivered: []environment.DeliveredWork{
			{Ref: "phase 44 step 5", Where: "pull request #12", SHAs: []string{"abc1234"}},
		},
	})
	reg := capability.NewRegistry()
	for _, c := range []capability.Capability{
		{ID: "software.act.write_code", Domain: "software", NeedsWorkspace: true},
		{ID: "software.act.review_pr", Domain: "software"},
	} {
		if err := reg.Register(c); err != nil {
			t.Fatal(err)
		}
	}
	sc.svc.capReg = reg
	stepObserve(context.Background(), sc)
	if sc.evidence.HasThirdParty() {
		t.Fatalf("the fixture is own-authored, yet evidence names %v", sc.evidence.ThirdParty)
	}
	return sc
}

// onePlan is a confident plan of one action whose prompt is prompt.
func onePlan(capID, prompt string) plan {
	return plan{Confidence: 0.99, Actions: []plannedAction{{
		CapabilityID: capID, EnvID: "test.env.git",
		Params: map[string]any{"prompt": prompt},
	}}}
}

func TestPlanThatWritesATickedStepRaisesACheckpointWithTheRedoReason(t *testing.T) {
	sc := deliveredFixture(t)

	got, paused := stepDecide(context.Background(), sc,
		onePlan("software.act.write_code", "Deliver phase 44 step 5: mark the group ready."), nil)
	if !paused {
		t.Fatal("a plan writing a ticked step ran without asking")
	}
	if len(got.Actions) != 1 {
		t.Errorf("escalated plan carries %d actions, want the one for the reviewer", len(got.Actions))
	}
	cps := pendingFor(t, sc.svc.store, sc.obj)
	if len(cps) != 1 {
		t.Fatalf("pending checkpoints = %d, want 1", len(cps))
	}
	for _, want := range []string{"redoes delivered work", "phase 44 step 5", "pull request #12", "abc1234"} {
		if !strings.Contains(cps[0].Reason, want) {
			t.Errorf("checkpoint reason %q does not name %q", cps[0].Reason, want)
		}
	}
}

func TestPlanThatWritesAnUntickedStepDoesNotEscalate(t *testing.T) {
	sc := deliveredFixture(t)

	if _, paused := stepDecide(context.Background(), sc,
		onePlan("software.act.write_code", "Deliver phase 44 step 6: plan from what is delivered."), nil); paused {
		t.Fatalf("a plan writing an unticked step escalated: %v", pendingFor(t, sc.svc.store, sc.obj))
	}
}

// Only a write redoes work: reviewing the delivered step is what a human
// would want done with it, so an action that writes no files is not checked.
func TestPlanThatOnlyReviewsATickedStepDoesNotEscalate(t *testing.T) {
	sc := deliveredFixture(t)

	if _, paused := stepDecide(context.Background(), sc,
		onePlan("software.act.review_pr", "Review phase 44 step 5 on pull request #12."), nil); paused {
		t.Fatalf("a plan reviewing a ticked step escalated: %v", pendingFor(t, sc.svc.store, sc.obj))
	}
}
