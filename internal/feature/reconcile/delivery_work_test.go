package reconcile

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/bsenel/karakuri/domains/software"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 40 step 13, acceptance: a delivery pass starts when there is work and
// costs nothing when there is none. The supervisor already skips a pass whose
// fingerprint did not move (TestQuietWorldCostsNoLoopRun), so this step is
// that mechanism fed the right snapshot, and the test goes through the real
// delivery-work environment to prove it is the snapshot and not the
// supervisor that has to change.

// fakeForge serves the few reads the delivery-work environment makes and
// counts nothing else; every other method answers as the no-op adapter does.
type fakeForge struct {
	*versioncontrol.NoOp
	mu   sync.Mutex
	prs  []versioncontrol.PRSummary
	main string
}

func (f *fakeForge) Active() bool { return true }
func (f *fakeForge) ListPRs(context.Context, string, time.Time) ([]versioncontrol.PRSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]versioncontrol.PRSummary(nil), f.prs...), nil
}
func (f *fakeForge) readMain(context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.main, nil
}
func (f *fakeForge) openPR(title string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prs = append(f.prs, versioncontrol.PRSummary{ID: "9", Number: 9, Title: title, HeadBranch: "karakuri/x", OwnAuthor: true})
}
func (f *fakeForge) setMain(text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.main = text
}

const (
	mainNothingPlanned = "## Phase 39 — Done\n\n## Engineering Backlog\n\n| ID | Title | Evidence | Proposed change | Size | Status |\n|----|---|---|---|---|---|\n| EB-001 | a | e | p | S | Done |\n"
	mainWithPlanned    = "## Phase 40 — Streams (Planned)\n\n## Engineering Backlog\n\n| ID | Title | Evidence | Proposed change | Size | Status |\n|----|---|---|---|---|---|\n| EB-001 | a | e | p | S | Done |\n"
)

func TestDeliveryPassStartsWhenThereIsWorkAndCostsNothingOtherwise(t *testing.T) {
	forge := &fakeForge{NoOp: versioncontrol.NewNoOp(), main: mainNothingPlanned}

	f := newFixture(t, Config{})
	f.use(t, map[string]string{}) // none of the fixture's own fakes
	if err := f.svc.envReg.Register(environment.Factory{
		EnvID: "delivery_work", Domain: "test",
		Build: func(environment.BuildContext) (environment.Environment, error) {
			return software.NewDeliveryWorkEnvironment("delivery_work", forge, forge.readMain), nil
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	f.loops.criteriaMet = 1.0
	// The pass ends with the pull request open: what the model's run does to the world.
	f.loops.onRun = func() { forge.openPR("feat(x): the planned thing (phase 40 step 1)") }
	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Sense: "15m"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})
	tick := func() {
		t.Helper()
		f.clock = f.clock.Add(15 * time.Minute)
		f.pass(t, obj.ID, "")
	}

	// Baseline, then ten ticks with nothing Planned.
	f.pass(t, obj.ID, "")
	if f.state(t, obj.ID).Converged.SHA == "" {
		t.Fatal("the delivery-work environment gave no snapshot although main was readable; the supervisor treats that as blind and would never sense it")
	}
	for i := 0; i < 10; i++ {
		tick()
	}
	if got := f.loops.count(); got != 0 {
		t.Fatalf("%d model calls over ten ticks with nothing Planned, want 0", got)
	}

	// A commit on main adds a Planned item: the next tick starts a pass.
	forge.setMain(mainWithPlanned)
	tick()
	if got := f.loops.count(); got != 1 {
		t.Fatalf("%d model calls on the tick after main gained a Planned item, want 1: the snapshot did not move, so no pass started", got)
	}

	// The pass ended with its pull request open and waiting for the owner:
	// that is not work, so no further model call until main or the PR changes.
	for i := 0; i < 10; i++ {
		tick()
	}
	if got := f.loops.count(); got != 1 {
		t.Fatalf("%d model calls in total after the pull request opened, want 1: the stream plans again against an item only a merge can finish", got)
	}

	// The owner merges: main moves (the item is Done, the PR is gone).
	forge.mu.Lock()
	forge.prs = nil
	forge.mu.Unlock()
	forge.setMain(mainNothingPlanned)
	tick()
	if got := f.loops.count(); got != 1 {
		t.Errorf("%d model calls after the merge left nothing Planned, want still 1", got)
	}
}
