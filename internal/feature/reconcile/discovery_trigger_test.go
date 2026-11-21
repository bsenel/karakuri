package reconcile

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bsenel/karakuri/domains/software"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 40 step 13, acceptance for the discovery trigger: with a fake clock and
// a fake forge, no model, the supervisor starts a discovery cycle when the
// pipeline is empty, without waiting for the cadence. The cadence here is a
// weekly cron (Sunday 09:00) plus a 15 minute sense; the test clock starts on a
// Wednesday.

const discoveryBranch = "karakuri/discovery-2026-03-04"

// discoveryWorld is everything the trigger reads that is not the forge: when the
// last cycle of either trigger ended, and the owner's closing comment.
type discoveryWorld struct {
	forge   *fakeForge
	lastEnd time.Time
	comment string
}

func (w *discoveryWorld) openProposal() {
	w.forge.mu.Lock()
	defer w.forge.mu.Unlock()
	w.forge.prs = append(w.forge.prs, versioncontrol.PRSummary{ID: "5", Number: 5, Title: "docs(research): market report", HeadBranch: discoveryBranch, OwnAuthor: true})
}

func (w *discoveryWorld) closeProposal(comment string) {
	w.forge.mu.Lock()
	defer w.forge.mu.Unlock()
	w.forge.prs = nil
	w.comment = comment
}

// newDiscoveryFixture declares a standing objective whose only environment is
// the discovery trigger. A pass of the fake loop ends with a proposal open and
// records its end, which is what a real cycle does to the world.
func newDiscoveryFixture(t *testing.T, mainText string, start time.Time) (*fixture, *discoveryWorld, objective.Objective) {
	t.Helper()
	w := &discoveryWorld{forge: &fakeForge{NoOp: versioncontrol.NewNoOp(), main: mainText}}
	f := newFixture(t, Config{})
	f.clock = start
	f.use(t, map[string]string{})
	if err := f.svc.envReg.Register(environment.Factory{
		EnvID: "discovery_trigger", Domain: "test",
		Build: func(environment.BuildContext) (environment.Environment, error) {
			return software.NewDiscoveryTriggerEnvironment("discovery_trigger", w.forge, software.DiscoveryTriggerSources{
				ReadMain:       w.forge.readMain,
				LastCycleEnd:   func(context.Context) (time.Time, error) { return w.lastEnd, nil },
				ClosingComment: func(context.Context) (string, error) { return w.comment, nil },
				MinGap:         software.DiscoveryMinGap,
				Now:            func() time.Time { return f.clock },
			}), nil
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	f.loops.criteriaMet = 1.0
	f.loops.onRun = func() {
		w.openProposal()
		w.lastEnd = f.clock
	}
	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Sense: "15m", Cron: "0 9 * * 0"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})
	// A stream that has been running: the last scheduled cycle was last Sunday
	// 09:00 and the next one is due on Sunday 2026-03-08 09:00, so what starts
	// a cycle before then is not the cadence. (A never-reconciled objective is
	// due at once, which would hide what these tests look at.)
	st := f.state(t, obj.ID)
	lastSunday := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	nextSunday := time.Date(2026, 3, 8, 9, 0, 0, 0, time.UTC)
	st.LastReconciledAt = &lastSunday
	st.NextReconcileAt = &nextSunday
	if err := f.store.SaveReconcileState(context.Background(), st); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	return f, w, obj
}

func tickFor(t *testing.T, f *fixture, id objective.ObjectiveID, d time.Duration) {
	t.Helper()
	for end := f.clock.Add(d); f.clock.Before(end); {
		f.clock = f.clock.Add(time.Hour)
		f.pass(t, id, "")
	}
}

// Nothing Planned and no open discovery pull request: a cycle starts without a
// person and without waiting for the cadence (Sunday is four days away).
func TestEmptyPipelineStartsADiscoveryCycleWithoutTheCadence(t *testing.T) {
	f, _, obj := newDiscoveryFixture(t, mainNothingPlanned, time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC))

	f.pass(t, obj.ID, "")
	tickFor(t, f, obj.ID, time.Hour)
	if got := f.loops.count(); got != 1 {
		t.Fatalf("%d cycles started with an empty pipeline, want exactly 1 (and the open proposal it left must keep the next ones away)", got)
	}
	if got := f.state(t, obj.ID).LastTrigger; got == reconcile.TriggerSchedule || got == reconcile.TriggerManual {
		t.Errorf("trigger = %q, want the sense-driven one: nobody asked and the cadence was not due", got)
	}
}

// With one open discovery proposal, none starts, however many ticks pass.
func TestOpenDiscoveryProposalKeepsTheEmptyTriggerSilent(t *testing.T) {
	f, w, obj := newDiscoveryFixture(t, mainNothingPlanned, time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC))
	w.openProposal()

	f.pass(t, obj.ID, "")
	tickFor(t, f, obj.ID, 40*time.Hour)
	if got := f.loops.count(); got != 0 {
		t.Fatalf("%d model calls with one open proposal and an empty pipeline, want 0: the proposals would pile up", got)
	}
}

// A delivered item waiting for merge (Planned on main, open pull request) and
// nothing else Planned: the pipeline is empty, so a cycle starts.
func TestDeliveredItemWaitingForMergeDoesNotHoldDiscoveryBack(t *testing.T) {
	f, w, obj := newDiscoveryFixture(t, mainWithPlanned, time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC))
	w.forge.openPR("feat(x): the planned thing (phase 40 step 1)")

	f.pass(t, obj.ID, "")
	tickFor(t, f, obj.ID, time.Hour)
	if got := f.loops.count(); got != 1 {
		t.Fatalf("%d cycles with the only Planned item delivered and waiting for the owner, want 1", got)
	}
}

// After a proposal is closed: none inside the minimum time, counted from the
// end of the last cycle; one after it.
func TestClosedProposalWaitsOutTheMinimumTimeBeforeTheNextCycle(t *testing.T) {
	// Friday, so Sunday's scheduled cycle is outside the window under test.
	f, w, obj := newDiscoveryFixture(t, mainNothingPlanned, time.Date(2026, 3, 6, 6, 0, 0, 0, time.UTC))

	f.pass(t, obj.ID, "")
	tickFor(t, f, obj.ID, time.Hour)
	if got := f.loops.count(); got != 1 {
		t.Fatalf("%d cycles to begin with, want 1", got)
	}
	w.closeProposal("Closing: too broad, narrow it to the CLI")

	tickFor(t, f, obj.ID, software.DiscoveryMinGap-2*time.Hour)
	if got := f.loops.count(); got != 1 {
		t.Fatalf("%d cycles inside the minimum time after the proposal was closed, want still 1: a closed proposal would start a loop of cycles", got)
	}

	tickFor(t, f, obj.ID, 3*time.Hour)
	if got := f.loops.count(); got != 2 {
		t.Fatalf("%d cycles after the minimum time, want 2", got)
	}
}

// The scheduled cycle runs on its cadence while Planned work remains, in a
// period in which an empty-pipeline cycle already ran, and the minimum time
// never delays it: the empty cycle ended 13 hours before Sunday 09:00.
func TestScheduledCycleStillRunsAfterAnEmptyPipelineCycleInTheSamePeriod(t *testing.T) {
	f, w, obj := newDiscoveryFixture(t, mainNothingPlanned, time.Date(2026, 3, 7, 20, 0, 0, 0, time.UTC))

	f.pass(t, obj.ID, "")
	tickFor(t, f, obj.ID, time.Hour)
	if got := f.loops.count(); got != 1 {
		t.Fatalf("%d empty-pipeline cycles on Saturday evening, want 1", got)
	}
	// Overnight the proposal is merged and adds Planned work: delivery's, so the
	// empty trigger is silent; the cadence is what is left.
	w.forge.mu.Lock()
	w.forge.prs = nil
	w.forge.mu.Unlock()
	w.forge.setMain(mainWithPlanned)

	tickFor(t, f, obj.ID, 12*time.Hour) // to Sunday 08:00
	if got := f.loops.count(); got != 1 {
		t.Fatalf("%d cycles before the cadence is due with Planned work on main, want still 1", got)
	}
	tickFor(t, f, obj.ID, 2*time.Hour) // past Sunday 09:00
	if got := f.loops.count(); got != 2 {
		t.Fatalf("%d cycles after Sunday 09:00, want 2: the empty-pipeline cycle must not count as the period's scheduled one, and the minimum time must not delay the scheduled one", got)
	}
	if got := f.state(t, obj.ID).LastTrigger; got != reconcile.TriggerSchedule {
		t.Errorf("trigger = %q, want the schedule", got)
	}
}

// After a merge that adds Planned work, the empty trigger is silent.
func TestMergeThatAddsPlannedWorkLeavesTheEmptyTriggerSilent(t *testing.T) {
	f, w, obj := newDiscoveryFixture(t, mainNothingPlanned, time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC))
	w.openProposal()
	f.pass(t, obj.ID, "") // baseline: a proposal is open

	w.forge.mu.Lock()
	w.forge.prs = nil // merged
	w.forge.mu.Unlock()
	w.forge.setMain(mainWithPlanned)
	tickFor(t, f, obj.ID, 40*time.Hour)
	if got := f.loops.count(); got != 0 {
		t.Fatalf("%d discovery cycles after a merge added Planned work, want 0: delivery's to run, not discovery's", got)
	}
}

// Main or the pull requests unreadable: no cycle starts, and it is not read as
// an empty pipeline.
func TestUnreadableMainStartsNoDiscoveryCycle(t *testing.T) {
	f, w, obj := newDiscoveryFixture(t, mainNothingPlanned, time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC))
	w.forge.mu.Lock()
	w.forge.mainErr = errors.New("401 bad credentials")
	w.forge.mu.Unlock()

	f.pass(t, obj.ID, "")
	tickFor(t, f, obj.ID, 10*time.Hour)
	if got := f.loops.count(); got != 0 {
		t.Fatalf("%d cycles with main unreadable, want 0: blind must not read as an empty pipeline", got)
	}
}
