package reconcile

// Phase 40 step 14c: a blocker the stream cannot remove, or that comes back,
// becomes a backlog entry. With a fake provider whose fix never clears the
// signal: after the template's declared number of failed attempts (3 when it
// declares none) no further pass starts for that blocker and the objective is
// blocked; those failed diagnose-and-fix passes never move
// ConsecutiveFailures, so no breaker checkpoint is raised; and the blocker is
// handed over with its diagnosis and each attempt with its audit rows. A
// blocker of one kind cleared for the second time inside the template's window
// is handed over the same way while the stream keeps running, and a third
// occurrence adds evidence to that one handover: one per kind.

import (
	"context"
	"fmt"
	"testing"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

const neverClearsDiagnosis = "DIAGNOSIS: TestClock reads time.Now and the fixture pins no clock"

// limitsPack is a pack whose only contribution is one template, so a test can
// declare a stream's blocker limits.
type limitsPack struct{ templates []objective.Template }

func (p *limitsPack) ID() string                                  { return "test" }
func (p *limitsPack) Name() string                                { return "test" }
func (p *limitsPack) Version() string                             { return "0" }
func (p *limitsPack) Description() string                         { return "" }
func (p *limitsPack) Capabilities() []capability.Capability       { return nil }
func (p *limitsPack) EnvironmentFactories() []environment.Factory { return nil }
func (p *limitsPack) AgentDefinitions() []coreagent.Definition    { return nil }
func (p *limitsPack) ObjectiveTemplates() []objective.Template    { return p.templates }
func (p *limitsPack) PlannerHints() []domain.PlannerHint          { return nil }
func (p *limitsPack) Init(context.Context, domain.Config) error   { return nil }
func (p *limitsPack) Teardown(context.Context) error              { return nil }

// handoverStream declares a stream built from tmpl (none when its ID is empty)
// with the given breaker threshold.
func handoverStream(t *testing.T, tmpl objective.Template, breaker int) (*fixture, objective.Objective) {
	t.Helper()
	f := newFixture(t, Config{BreakerFailures: breaker})
	if tmpl.ID != "" {
		reg := domain.NewRegistry()
		if err := reg.Register(context.Background(), &limitsPack{templates: []objective.Template{tmpl}}, domain.Config{}); err != nil {
			t.Fatalf("register pack: %v", err)
		}
		f.svc.domReg = reg
	}
	f.use(t, map[string]string{"git": "aaa"})
	obj := f.declare(t, objective.Objective{
		TwinID:     "twin-1",
		TemplateID: tmpl.ID,
		Cadence:    &objective.Cadence{Every: "1h"},
		Autonomy:   &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})
	return f, obj
}

// auditEachRun makes every run write one audit row for the objective, as a
// pass's plan row would, and returns the rows' IDs in run order.
func auditEachRun(f *fixture, obj objective.Objective) *[]string {
	ids := &[]string{}
	f.loops.onRun = func() {
		id := fmt.Sprintf("row-%d", len(*ids)+1)
		_ = f.store.SaveToolEvent(context.Background(), storage.ToolEvent{
			ID: id, ObjectiveID: string(obj.ID), Kind: "plan", Success: true, CreatedAt: f.clock,
		})
		*ids = append(*ids, id)
	}
	return ids
}

// fixNeverClears is a provider whose diagnose-and-fix passes name a diagnosis
// and a remedy, and whose verify stays red.
func (f *fixture) fixNeverClears() {
	f.failVerify("tests-pass")
	f.loops.remedy = verifyRemedy
	f.loops.diagnosis = neverClearsDiagnosis
}

func (f *fixture) verifyGreen(ids ...string) {
	f.loops.mu.Lock()
	defer f.loops.mu.Unlock()
	f.loops.status = objective.StatusCompleted
	f.loops.criteria = nil
	for _, id := range ids {
		f.loops.criteria = append(f.loops.criteria, objective.Criterion{ID: id, State: objective.CriterionMet})
	}
}

func handoversOf(t *testing.T, f *fixture, obj objective.Objective) []reconcile.Handover {
	t.Helper()
	return f.state(t, obj.ID).Handovers
}

// (a) After the declared number of failed attempts no further pass starts for
// that blocker, and the objective's status is blocked. Before it, the stream
// is not blocked.
func TestAfterTheDeclaredAttemptsNoFurtherPassStartsAndTheStreamIsBlocked(t *testing.T) {
	for _, tc := range []struct {
		name  string
		tmpl  objective.Template
		limit int
	}{
		{"no template declares one: three", objective.Template{}, objective.DefaultBlockerAttempts},
		{"the template declares two", objective.Template{ID: "test.objective.two_tries", Domain: "test", BlockerAttempts: 2}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, obj := handoverStream(t, tc.tmpl, 10)
			f.fixNeverClears()
			f.pass(t, obj.ID, reconcile.TriggerManual) // raises the blocker: not an attempt

			for i := 1; i <= tc.limit; i++ {
				if got := f.objectiveStatus(t, obj.ID); got == objective.StatusBlocked {
					t.Fatalf("status is blocked after %d of %d attempts", i-1, tc.limit)
				}
				f.again(t, obj.ID)
				if b := blockerOn(t, f.state(t, obj.ID), reconcile.BlockerVerifyFailed, "tests-pass"); b.Attempts != i {
					t.Fatalf("attempts = %d after %d diagnose-and-fix passes", b.Attempts, i)
				}
			}
			if got := f.objectiveStatus(t, obj.ID); got != objective.StatusBlocked {
				t.Errorf("status = %q after %d failed attempts, want blocked", got, tc.limit)
			}

			runs := f.loops.count()
			f.again(t, obj.ID)
			f.again(t, obj.ID)
			if got := f.loops.count(); got != runs {
				t.Errorf("%d more passes started for a blocker whose %d attempts are spent, want none", got-runs, tc.limit)
			}
			if b := blockerOn(t, f.state(t, obj.ID), reconcile.BlockerVerifyFailed, "tests-pass"); b.Attempts != tc.limit {
				t.Errorf("attempts = %d after the stream stopped trying, want %d", b.Attempts, tc.limit)
			}
		})
	}
}

// (b) A failed diagnose-and-fix pass counts toward the attempts and not toward
// the circuit breaker: ConsecutiveFailures is what the raising pass left, and
// no breaker checkpoint is raised, even with a breaker the attempts would trip.
func TestFailedDiagnoseAndFixPassesLeaveConsecutiveFailuresUnchanged(t *testing.T) {
	f, obj := handoverStream(t, objective.Template{}, 2)
	f.fixNeverClears()
	f.pass(t, obj.ID, reconcile.TriggerManual)
	before := f.state(t, obj.ID).ConsecutiveFailures

	for i := 1; i <= objective.DefaultBlockerAttempts; i++ {
		f.again(t, obj.ID)
		if got := f.state(t, obj.ID).ConsecutiveFailures; got != before {
			t.Fatalf("ConsecutiveFailures = %d after diagnose-and-fix pass %d, want %d: an attempt is not a breaker failure", got, i, before)
		}
	}
	pending, err := f.store.ListPendingCheckpoints(context.Background(), "twin-1")
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	for _, cp := range pending {
		if cp.ObjectiveID == obj.ID && cp.Reason == "reconcile_circuit_breaker" {
			t.Errorf("a breaker checkpoint was raised: %q", cp.Summary)
		}
	}
	if b := blockerOn(t, f.state(t, obj.ID), reconcile.BlockerVerifyFailed, "tests-pass"); b.Attempts != objective.DefaultBlockerAttempts {
		t.Errorf("attempts = %d, want %d: the passes counted toward the attempts", b.Attempts, objective.DefaultBlockerAttempts)
	}
}

// (a, d) The blocker the stream stopped on is handed over with its diagnosis
// and each attempt: the loop it ran, what it found, what it tried and the
// audit rows it wrote.
func TestASpentBlockerIsHandedOverWithItsDiagnosisAndEachAttempt(t *testing.T) {
	f, obj := handoverStream(t, objective.Template{}, 10)
	rows := auditEachRun(f, obj)
	f.fixNeverClears()
	f.pass(t, obj.ID, reconcile.TriggerManual)
	for i := 0; i < objective.DefaultBlockerAttempts; i++ {
		f.again(t, obj.ID)
	}

	hs := handoversOf(t, f, obj)
	if len(hs) != 1 {
		t.Fatalf("handovers = %+v, want one", hs)
	}
	h := hs[0]
	if h.Kind != reconcile.BlockerVerifyFailed || h.Subject != "tests-pass" || h.Reason != reconcile.HandoverAttemptsSpent {
		t.Errorf("handover = %s on %q (%s), want verify_failed on tests-pass (attempts_spent)", h.Kind, h.Subject, h.Reason)
	}
	if h.Diagnosis != neverClearsDiagnosis {
		t.Errorf("handover diagnosis = %q, want %q", h.Diagnosis, neverClearsDiagnosis)
	}
	if len(h.Attempts) != objective.DefaultBlockerAttempts {
		t.Fatalf("handover attempts = %+v, want %d", h.Attempts, objective.DefaultBlockerAttempts)
	}
	loops := map[string]bool{}
	for i, a := range h.Attempts {
		if a.LoopID == "" || loops[a.LoopID] {
			t.Errorf("attempt %d loop = %q, want its own loop", i, a.LoopID)
		}
		loops[a.LoopID] = true
		if a.Diagnosis != neverClearsDiagnosis || a.Remedy != verifyRemedy {
			t.Errorf("attempt %d = %q / %q, want the diagnosis and the remedy tried", i, a.Diagnosis, a.Remedy)
		}
		if a.At.IsZero() {
			t.Errorf("attempt %d has no time", i)
		}
		// Run 0 raised the blocker; attempt i is run i+1.
		if want := (*rows)[i+1]; len(a.AuditRows) != 1 || a.AuditRows[0] != want {
			t.Errorf("attempt %d audit rows = %v, want [%s]", i, a.AuditRows, want)
		}
	}
}

// (e) A kind cleared for the second time inside the declared window is handed
// over while the stream keeps running; a third occurrence, on another subject
// of the same kind, adds evidence to that handover and no second one.
func TestABlockerKindClearedTwiceInsideTheWindowIsHandedOverOnce(t *testing.T) {
	tmpl := objective.Template{ID: "test.objective.window", Domain: "test", BlockerRecurrenceWindow: "48h"}
	f, obj := handoverStream(t, tmpl, 10)
	f.loops.remedy = verifyRemedy

	f.failVerify("tests-pass")
	f.pass(t, obj.ID, reconcile.TriggerManual)
	f.verifyGreen("tests-pass")
	f.again(t, obj.ID) // first clear
	if hs := handoversOf(t, f, obj); len(hs) != 0 {
		t.Fatalf("handovers after one clear = %+v, want none", hs)
	}

	f.failVerify("tests-pass")
	f.again(t, obj.ID)
	f.verifyGreen("tests-pass")
	f.again(t, obj.ID) // second clear, inside 48h
	hs := handoversOf(t, f, obj)
	if len(hs) != 1 {
		t.Fatalf("handovers after a second clear inside the window = %+v, want one", hs)
	}
	if h := hs[0]; h.Kind != reconcile.BlockerVerifyFailed || h.Reason != reconcile.HandoverRecurring || len(h.Cleared) != 2 {
		t.Errorf("handover = %+v, want verify_failed, recurring, cleared twice", h)
	}
	if got := f.objectiveStatus(t, obj.ID); got == objective.StatusBlocked {
		t.Errorf("status = blocked: a recurring blocker leaves the stream running")
	}
	runs := f.loops.count()
	f.again(t, obj.ID)
	if f.loops.count() != runs+1 {
		t.Errorf("no pass started after a recurring handover: the stream must keep running")
	}

	f.failVerify("lint-clean")
	f.again(t, obj.ID)
	f.verifyGreen("lint-clean")
	f.again(t, obj.ID) // third occurrence of the kind
	hs = handoversOf(t, f, obj)
	if len(hs) != 1 {
		t.Fatalf("handovers after a third occurrence = %+v, want still one per kind", hs)
	}
	if len(hs[0].Cleared) != 3 {
		t.Errorf("handover cleared %d times, want 3: the third occurrence adds evidence", len(hs[0].Cleared))
	}
}

// Two clears further apart than the window are a blocker fixed, not one that
// keeps coming back.
func TestABlockerKindClearedTwiceOutsideTheWindowIsNotHandedOver(t *testing.T) {
	tmpl := objective.Template{ID: "test.objective.window", Domain: "test", BlockerRecurrenceWindow: "48h"}
	f, obj := handoverStream(t, tmpl, 10)
	f.loops.remedy = verifyRemedy

	f.failVerify("tests-pass")
	f.pass(t, obj.ID, reconcile.TriggerManual)
	f.verifyGreen("tests-pass")
	f.again(t, obj.ID)

	f.clock = f.clock.Add(72 * time.Hour)
	f.failVerify("tests-pass")
	f.again(t, obj.ID)
	f.verifyGreen("tests-pass")
	f.again(t, obj.ID)
	if hs := handoversOf(t, f, obj); len(hs) != 0 {
		t.Errorf("handovers = %+v for clears 72h apart with a 48h window, want none", hs)
	}
}
