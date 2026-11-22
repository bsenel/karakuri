package reconcile

// Phase 40 step 14b, the remedy's memory: a remedy a diagnose-and-fix pass
// found, and the next reading confirmed (the blocker is gone), is stored as one
// semantic memory entry of the stream's agent with the blocker's kind as its
// domain. A later pass on a blocker of that kind is handed it; a blocker of
// another kind is not. A recalled remedy is a hint: it rides on the request and
// adds no authority.

import (
	"context"
	"testing"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/memory"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	featureloop "github.com/bsenel/karakuri/internal/feature/loop"
)

const (
	verifyRemedy = "pin the clock the fixture's tests read, then push the branch"
	planRemedy   = "shorten the observation the planner is given, then rerun"
)

func streamAgent(f *fixture, obj objective.Objective) coreagent.AgentID {
	return featureloop.SelectAgent(f.svc.domReg, obj, coreagent.Definition{}).ID
}

// remediesOf reads back the stream agent's semantic entries whose domain is
// kind. The domain is compared here, not left to the store's filter, so a
// store that ignores it cannot hide a wrong write.
func remediesOf(t *testing.T, f *fixture, agent coreagent.AgentID, kind reconcile.BlockerKind) []memory.Entry {
	t.Helper()
	all, err := f.store.QuerySemantic(context.Background(), memory.Query{AgentID: agent})
	if err != nil {
		t.Fatalf("query semantic: %v", err)
	}
	var out []memory.Entry
	for _, e := range all {
		if e.Domain == string(kind) {
			out = append(out, e)
		}
	}
	return out
}

func seedRemedy(t *testing.T, f *fixture, agent coreagent.AgentID, kind reconcile.BlockerKind, text string) {
	t.Helper()
	err := f.store.SaveMemorySemantic(context.Background(), memory.Entry{
		ID: "seed-" + string(kind) + "-" + string(agent), AgentID: agent, Tier: string(memory.TierSemantic),
		Domain: string(kind), Content: text, CreatedAt: f.clock,
	})
	if err != nil {
		t.Fatalf("seed remedy: %v", err)
	}
}

// holdBlocker puts a blocker on the stream's state before its next pass.
func holdBlocker(t *testing.T, f *fixture, obj objective.Objective, b reconcile.Blocker) {
	t.Helper()
	st := f.state(t, obj.ID)
	st.Blockers = []reconcile.Blocker{b}
	if err := f.store.SaveReconcileState(context.Background(), st); err != nil {
		t.Fatalf("save state: %v", err)
	}
}

// A pass that applied a remedy and left the blocker clear stores it once.
func TestAConfirmedRemedyIsStoredAsOneSemanticMemoryWithTheBlockersKind(t *testing.T) {
	f, obj := plainStream(t)
	f.loops.remedy = verifyRemedy
	f.loops.criteria = []objective.Criterion{{ID: "tests-pass", State: objective.CriterionMet}}
	holdBlocker(t, f, obj, blk(reconcile.BlockerVerifyFailed, "tests-pass", 2, t0))

	f.pass(t, obj.ID, reconcile.TriggerManual)

	wantBlockers(t, f.state(t, obj.ID))
	got := remediesOf(t, f, streamAgent(f, obj), reconcile.BlockerVerifyFailed)
	if len(got) != 1 {
		t.Fatalf("stored %d remedies of kind verify_failed, want 1: %+v", len(got), got)
	}
	if got[0].Content != verifyRemedy {
		t.Errorf("stored remedy = %q, want %q", got[0].Content, verifyRemedy)
	}
	if got[0].Tier != string(memory.TierSemantic) {
		t.Errorf("tier = %q, want semantic", got[0].Tier)
	}
}

// A remedy the next reading did not confirm is not kept: the blocker is still
// there, or no blocker was being fixed.
func TestAnUnconfirmedRemedyIsNotStored(t *testing.T) {
	t.Run("blocker still raised", func(t *testing.T) {
		f, obj := plainStream(t)
		f.failVerify("tests-pass")
		f.loops.remedy = verifyRemedy
		f.pass(t, obj.ID, reconcile.TriggerManual)
		f.again(t, obj.ID)
		wantBlockers(t, f.state(t, obj.ID), reconcile.Blocker{Kind: reconcile.BlockerVerifyFailed, Subject: "tests-pass", Count: 2})
		if got := remediesOf(t, f, streamAgent(f, obj), reconcile.BlockerVerifyFailed); len(got) != 0 {
			t.Fatalf("stored %+v for a blocker that did not clear", got)
		}
	})
	t.Run("no blocker was open", func(t *testing.T) {
		f, obj := plainStream(t)
		f.loops.remedy = verifyRemedy
		f.pass(t, obj.ID, reconcile.TriggerManual)
		all, err := f.store.QuerySemantic(context.Background(), memory.Query{AgentID: streamAgent(f, obj)})
		if err != nil || len(all) != 0 {
			t.Fatalf("semantic entries = %+v (err %v), want none", all, err)
		}
	})
	t.Run("cleared with no remedy named", func(t *testing.T) {
		f, obj := plainStream(t)
		f.loops.criteria = []objective.Criterion{{ID: "tests-pass", State: objective.CriterionMet}}
		holdBlocker(t, f, obj, blk(reconcile.BlockerVerifyFailed, "tests-pass", 2, t0))
		f.pass(t, obj.ID, reconcile.TriggerManual)
		wantBlockers(t, f.state(t, obj.ID))
		if got := remediesOf(t, f, streamAgent(f, obj), reconcile.BlockerVerifyFailed); len(got) != 0 {
			t.Fatalf("stored %+v though the pass named no remedy", got)
		}
	})
}

// The kind filter is applied end to end: two kinds are stored, and each blocker
// is handed its own kind's remedy and never the other's.
func TestAPassRecallsTheStoredRemedyOfItsBlockersKindOnly(t *testing.T) {
	for _, tc := range []struct {
		kind    reconcile.BlockerKind
		want    string
		notWant string
	}{
		{reconcile.BlockerVerifyFailed, verifyRemedy, planRemedy},
		{reconcile.BlockerPlanError, planRemedy, verifyRemedy},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			f, obj := plainStream(t)
			agent := streamAgent(f, obj)
			seedRemedy(t, f, agent, reconcile.BlockerVerifyFailed, verifyRemedy)
			seedRemedy(t, f, agent, reconcile.BlockerPlanError, planRemedy)

			got := unblockRequest(t, f, obj, blk(tc.kind, "obj-1", 1, t0)).Unblock
			if got == nil {
				t.Fatal("request.Unblock is nil")
			}
			if len(got.Recalled) != 1 || got.Recalled[0] != tc.want {
				t.Errorf("Recalled = %q, want exactly [%q]", got.Recalled, tc.want)
			}
			for _, r := range got.Recalled {
				if r == tc.notWant {
					t.Errorf("recalled %q, a remedy of another kind", r)
				}
			}
		})
	}
}

// A kind nothing was stored for recalls nothing, though other kinds have
// remedies.
func TestABlockerOfAKindWithNoStoredRemedyRecallsNothing(t *testing.T) {
	f, obj := plainStream(t)
	seedRemedy(t, f, streamAgent(f, obj), reconcile.BlockerVerifyFailed, verifyRemedy)
	seedRemedy(t, f, streamAgent(f, obj), reconcile.BlockerPlanError, planRemedy)
	got := unblockRequest(t, f, obj, blk(reconcile.BlockerUnmergeablePR, "karakuri/x", 1, t0)).Unblock
	if got == nil {
		t.Fatal("request.Unblock is nil")
	}
	if len(got.Recalled) != 0 {
		t.Fatalf("Recalled = %q, want none for a kind nothing was stored for", got.Recalled)
	}
}

// Another agent keeps its own remedies.
func TestARemedyIsRecalledOnlyByTheAgentThatStoredIt(t *testing.T) {
	f, obj := plainStream(t)
	seedRemedy(t, f, streamAgent(f, obj), reconcile.BlockerVerifyFailed, verifyRemedy)
	seedRemedy(t, f, "some-other-agent", reconcile.BlockerVerifyFailed, planRemedy)
	got := unblockRequest(t, f, obj, blk(reconcile.BlockerVerifyFailed, "tests-pass", 1, t0)).Unblock
	if got == nil || len(got.Recalled) != 1 || got.Recalled[0] != verifyRemedy {
		t.Fatalf("Unblock = %+v, want only the stream agent's own remedy", got)
	}
}

// The whole path with nothing seeded by hand: a pass confirms a remedy, a later
// blocker of the same kind recalls it, one of another kind does not.
func TestAConfirmedRemedyIsRecalledByALaterBlockerOfTheSameKind(t *testing.T) {
	f, obj := plainStream(t)
	f.loops.remedy = verifyRemedy
	f.loops.criteria = []objective.Criterion{{ID: "tests-pass", State: objective.CriterionMet}}
	holdBlocker(t, f, obj, blk(reconcile.BlockerVerifyFailed, "tests-pass", 2, t0))
	f.pass(t, obj.ID, reconcile.TriggerManual)
	f.loops.remedy = ""

	f.clock = f.clock.Add(2 * time.Hour)
	same := unblockRequest(t, f, obj, blk(reconcile.BlockerVerifyFailed, "lint-clean", 1, t0)).Unblock
	if same == nil || len(same.Recalled) != 1 || same.Recalled[0] != verifyRemedy {
		t.Fatalf("same kind: Unblock = %+v, want the stored remedy recalled", same)
	}
	f.clock = f.clock.Add(2 * time.Hour)
	other := unblockRequest(t, f, obj, blk(reconcile.BlockerPlanError, "obj-1", 1, t0)).Unblock
	if other == nil {
		t.Fatal("other kind: request.Unblock is nil")
	}
	if len(other.Recalled) != 0 {
		t.Fatalf("other kind: Recalled = %q, want nothing", other.Recalled)
	}
}
