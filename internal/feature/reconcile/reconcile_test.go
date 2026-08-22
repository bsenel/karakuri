package reconcile

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/event"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	featurecp "github.com/bsenel/karakuri/internal/feature/checkpoint"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	"github.com/bsenel/karakuri/quota/cost"
)

// ── fakes ─────────────────────────────────────────────────────────────────

// fakeLoop stands in for the loop service. It writes the loop_states row the
// supervisor watches, which is the real contract between the two: the
// supervisor never holds a channel into a loop, it reads the row that survives
// the process.
type fakeLoop struct {
	mu    sync.Mutex
	store storage.StorageAdapter
	runs  int

	// outcome describes what the loop should do. Written before Run.
	err         error
	criteriaMet float64
	status      objective.ObjectiveStatus
	paused      bool
	checkpoint  string
	authority   coreagent.AuthorityBounds
}

func (f *fakeLoop) Run(ctx context.Context, req coreloop.Request) (coreloop.Result, error) {
	f.mu.Lock()
	f.runs++
	f.authority = req.Agent.Authority
	err := f.err
	f.mu.Unlock()
	if err != nil {
		return coreloop.Result{}, err
	}

	id := newOutcomeID()
	st := coreloop.State{
		LoopID:       id,
		ObjectiveID:  req.Objective.ID,
		Completed:    !f.paused,
		Paused:       f.paused,
		Status:       f.status,
		CriteriaMet:  f.criteriaMet,
		CheckpointID: f.checkpoint,
	}
	if err := f.store.SaveLoopState(ctx, st); err != nil {
		return coreloop.Result{}, err
	}
	return coreloop.Result{LoopID: id, ObjectiveID: req.Objective.ID}, nil
}

func (f *fakeLoop) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.runs
}

func (f *fakeLoop) bounds() coreagent.AuthorityBounds {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.authority
}

// fakeEnv is an environment whose snapshot hash the test controls.
type fakeEnv struct {
	id      string
	sha     string
	present bool
}

func (e *fakeEnv) ID() environment.EnvironmentID { return environment.EnvironmentID(e.id) }
func (e *fakeEnv) Domain() string                { return "test" }
func (e *fakeEnv) Observe(context.Context, environment.ObservationQuery) (environment.Observation, error) {
	return environment.Observation{}, nil
}
func (e *fakeEnv) Act(context.Context, environment.Action) (environment.ActionResult, error) {
	return environment.ActionResult{}, nil
}
func (e *fakeEnv) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return nil, nil
}
func (e *fakeEnv) Snapshot(context.Context) (environment.EnvironmentSnapshot, error) {
	if e.sha == "" {
		return environment.EnvironmentSnapshot{EnvID: e.ID()}, nil
	}
	return environment.EnvironmentSnapshot{EnvID: e.ID(), SHA: e.sha}, nil
}

// ── fixture ───────────────────────────────────────────────────────────────

type fixture struct {
	svc   *Service
	store storage.StorageAdapter
	loops *fakeLoop
	envs  []*fakeEnv
	clock time.Time
}

func newFixture(t *testing.T, cfg Config) *fixture {
	t.Helper()
	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "reconcile.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := storage.NewGORMStorage(db)
	hub := event.NewHub()
	loops := &fakeLoop{store: store, status: objective.StatusCompleted}
	f := &fixture{
		store: store,
		loops: loops,
		clock: time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC),
	}
	// A real registry rather than a hook into the service: the supervisor has
	// to hash exactly the environments the loop would observe, and a test that
	// bypassed the build path would not catch it drifting from them.
	envReg := environment.NewRegistry()
	for _, id := range []string{"git", "ticket", "calendar", "inbox"} {
		env := &fakeEnv{id: id}
		f.envs = append(f.envs, env)
		if err := envReg.Register(environment.Factory{
			EnvID:  environment.EnvironmentID(id),
			Domain: "test",
			Build: func(environment.BuildContext) (environment.Environment, error) {
				if !env.present {
					return nil, errors.New("not part of this fixture")
				}
				return env, nil
			},
		}); err != nil {
			t.Fatalf("register env: %v", err)
		}
	}
	// A short poll so the suite is not paced by the production heartbeat.
	if cfg.LoopPoll == 0 {
		cfg.LoopPoll = time.Millisecond
	}
	f.svc = NewService(store, loops, envReg, nil, featurecp.NewService(store, hub), hub, karakuriquota.Deps{}, cfg)
	f.svc.now = func() time.Time { return f.clock }
	return f
}

// use marks which of the fixture's environments this test's objective
// observes, and gives them their starting hashes.
func (f *fixture) use(t *testing.T, shas map[string]string) {
	t.Helper()
	for _, e := range f.envs {
		sha, ok := shas[e.id]
		e.present = ok
		e.sha = sha
	}
}

// move changes one environment's hash, which is what drift is.
func (f *fixture) move(id, sha string) {
	for _, e := range f.envs {
		if e.id == id {
			e.sha = sha
		}
	}
}

func (f *fixture) declare(t *testing.T, obj objective.Objective) objective.Objective {
	t.Helper()
	if obj.ID == "" {
		obj.ID = "obj-1"
	}
	obj.Mode = objective.ModeStanding
	if obj.Domain == "" {
		obj.Domain = "test"
	}
	if err := f.store.SaveObjective(context.Background(), obj); err != nil {
		t.Fatalf("save objective: %v", err)
	}
	if err := f.svc.Declare(context.Background(), obj); err != nil {
		t.Fatalf("declare: %v", err)
	}
	return obj
}

func (f *fixture) pass(t *testing.T, id objective.ObjectiveID, forced reconcile.Trigger) {
	t.Helper()
	if err := f.svc.pass(context.Background(), id, forced); err != nil {
		t.Fatalf("pass: %v", err)
	}
}

func (f *fixture) state(t *testing.T, id objective.ObjectiveID) reconcile.State {
	t.Helper()
	st, err := f.store.GetReconcileState(context.Background(), id)
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	return st
}

// ── the economics ─────────────────────────────────────────────────────────

// The claim the whole design rests on: an objective that is checked and found
// unchanged costs adapter calls and no model call at all.
func TestQuietWorldCostsNoLoopRun(t *testing.T) {
	f := newFixture(t, Config{})
	f.use(t, map[string]string{"git": "aaa"})
	obj := f.declare(t, objective.Objective{Cadence: &objective.Cadence{Sense: "15m"}})

	// First pass establishes the baseline.
	f.pass(t, obj.ID, "")
	// Several more with nothing moving.
	for i := 0; i < 5; i++ {
		f.clock = f.clock.Add(15 * time.Minute)
		f.pass(t, obj.ID, "")
	}

	if got := f.loops.count(); got != 0 {
		t.Fatalf("the loop ran %d times over a still world, want 0", got)
	}
	history, err := f.svc.History(context.Background(), obj.ID, 50)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 6 {
		t.Errorf("recorded %d passes, want 6 — the cheap ones are the evidence the split works", len(history))
	}
	for _, o := range history {
		if o.LoopID != "" {
			t.Errorf("a sense-only pass recorded loop %q", o.LoopID)
		}
	}
}

// An environment that moves is what makes it worth spending.
func TestDriftTriggersAReconcile(t *testing.T) {
	f := newFixture(t, Config{})
	f.use(t, map[string]string{"git": "aaa"})
	f.loops.criteriaMet = 1.0
	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Sense: "15m"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})

	f.pass(t, obj.ID, "")
	if f.loops.count() != 0 {
		t.Fatalf("the baseline pass ran a loop")
	}

	f.move("git", "bbb")
	f.clock = f.clock.Add(15 * time.Minute)
	f.pass(t, obj.ID, "")

	if got := f.loops.count(); got != 1 {
		t.Fatalf("the loop ran %d times after drift, want 1", got)
	}
	st := f.state(t, obj.ID)
	if st.LastTrigger != reconcile.TriggerDrift {
		t.Errorf("trigger = %q, want drift", st.LastTrigger)
	}
	if st.Converged.SHA == "" || st.LastConvergedAt == nil {
		t.Error("a converged reconcile did not record a new baseline")
	}

	// And the objective is converged, not completed. "Completed" would say
	// the work is over on a thing whose point is that it never is.
	got, err := f.store.GetObjective(context.Background(), obj.ID)
	if err != nil {
		t.Fatalf("get objective: %v", err)
	}
	if got.Status != objective.StatusConverged {
		t.Errorf("objective status = %q, want converged", got.Status)
	}
}

// Environments that cannot hash themselves — a calendar, an inbox — must not
// read as "unchanged". Such objectives are driven by their schedule.
func TestBlindEnvironmentsAreNotSilentlyStill(t *testing.T) {
	f := newFixture(t, Config{})
	f.use(t, map[string]string{"calendar": "", "inbox": ""})
	f.loops.criteriaMet = 1.0
	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Sense: "15m", Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})

	f.pass(t, obj.ID, "")

	history, _ := f.svc.History(context.Background(), obj.ID, 5)
	if len(history) == 0 {
		t.Fatal("no outcome recorded")
	}
	if !history[0].Drift.Blind {
		t.Error("a fingerprint over unhashable environments did not report itself blind")
	}
	if history[0].Drift.Changed {
		t.Error("a blind sense reported drift")
	}
	// The schedule still moved it: a never-run objective is due immediately.
	if f.loops.count() != 1 {
		t.Errorf("the loop ran %d times, want 1 — the schedule is what drives a blind objective", f.loops.count())
	}
}

// ── autonomy ──────────────────────────────────────────────────────────────

// The ceiling is what makes earned autonomy safe. It is enforced when the
// state is read, so no history and no hand-edited row can widen it.
func TestAutonomyStopsAtTheCeiling(t *testing.T) {
	f := newFixture(t, Config{})
	f.use(t, map[string]string{"git": "aaa"})
	f.loops.criteriaMet = 1.0
	obj := f.declare(t, objective.Objective{
		Cadence: &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{
			Level:        objective.AutonomyPropose,
			Ceiling:      objective.AutonomyActWithNotice,
			PromoteAfter: 1,
		},
	})

	// Enough clean reconciles to climb well past the ceiling if nothing
	// stopped it.
	for i := 0; i < 6; i++ {
		f.pass(t, obj.ID, reconcile.TriggerManual)
		f.clock = f.clock.Add(time.Hour)
	}

	st := f.state(t, obj.ID)
	if st.Autonomy != objective.AutonomyActWithNotice {
		t.Errorf("autonomy = %q after six clean runs, want to stop at the declared ceiling", st.Autonomy)
	}

	// The audit log carries the movement, because a change in what Karakuri
	// may do without asking is worth its own row.
	events, err := f.store.ListToolEvents(context.Background(), storage.ToolEventFilter{
		ObjectiveID: string(obj.ID),
		Kind:        storage.ToolEventPromotion,
	})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(events) == 0 {
		t.Error("promotions left no audit trail")
	}
}

// The level becomes enforcement by rewriting the bounds the decide step
// already reads. There is no second gate.
func TestProposeLevelPinsTheAuthorityBoundsShut(t *testing.T) {
	f := newFixture(t, Config{})
	f.use(t, map[string]string{"git": "aaa"})
	f.loops.criteriaMet = 1.0
	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyPropose, Ceiling: objective.AutonomyPropose},
	})

	f.pass(t, obj.ID, reconcile.TriggerManual)

	bounds := f.loops.bounds()
	if bounds.MaxAutonomousActions != 0 {
		t.Errorf("MaxAutonomousActions = %d at propose, want 0", bounds.MaxAutonomousActions)
	}
	if bounds.ConfidenceThreshold <= 1.0 {
		t.Errorf("ConfidenceThreshold = %v at propose, want above any attainable confidence", bounds.ConfidenceThreshold)
	}
}

// An objective held at sense level never spends, and tells a human instead.
func TestSenseLevelReportsDriftAndNeverActs(t *testing.T) {
	f := newFixture(t, Config{})
	f.use(t, map[string]string{"git": "aaa"})
	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Sense: "30s"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomySense, Ceiling: objective.AutonomySense},
	})

	f.pass(t, obj.ID, "") // baseline
	f.move("git", "bbb")
	f.clock = f.clock.Add(time.Minute)
	f.pass(t, obj.ID, "")

	if f.loops.count() != 0 {
		t.Fatalf("a sense-level objective ran %d loops, want 0", f.loops.count())
	}
	pending, err := f.store.ListPendingCheckpoints(context.Background(), "")
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("got %d checkpoints, want 1 — an objective that may not act must still say something", len(pending))
	}

	// Re-baselined, so the same change is reported once rather than on every
	// tick until somebody deals with it.
	f.clock = f.clock.Add(time.Minute)
	f.pass(t, obj.ID, "")
	if pending, _ := f.store.ListPendingCheckpoints(context.Background(), ""); len(pending) != 1 {
		t.Errorf("got %d checkpoints after an unchanged tick, want the original 1", len(pending))
	}
}

// ── guardrails ────────────────────────────────────────────────────────────

func TestCircuitBreakerPausesAndAsks(t *testing.T) {
	f := newFixture(t, Config{BreakerFailures: 3})
	f.use(t, map[string]string{"git": "aaa"})
	f.loops.err = errors.New("adapter unreachable")
	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})

	for i := 0; i < 3; i++ {
		f.pass(t, obj.ID, reconcile.TriggerManual)
		f.clock = f.clock.Add(time.Hour)
	}

	st := f.state(t, obj.ID)
	if !st.Paused {
		t.Fatalf("still running after 3 consecutive failures (count=%d)", st.ConsecutiveFailures)
	}
	if st.NextDueAt != nil {
		t.Error("a paused objective is still scheduled")
	}
	pending, _ := f.store.ListPendingCheckpoints(context.Background(), "")
	if len(pending) == 0 {
		t.Error("the breaker tripped silently; an objective that went quiet with no explanation looks exactly like one that is content")
	}

	// A resume clears what stopped it, or the next stumble trips it again
	// immediately.
	if err := f.svc.Resume(context.Background(), obj.ID); err != nil {
		t.Fatalf("resume: %v", err)
	}
	st = f.state(t, obj.ID)
	if st.Paused || st.ConsecutiveFailures != 0 || st.ScoreStreak != 0 {
		t.Errorf("resume left paused=%v failures=%d streak=%d, want all cleared",
			st.Paused, st.ConsecutiveFailures, st.ScoreStreak)
	}
}

func TestStallDetectorStopsBurningTokens(t *testing.T) {
	f := newFixture(t, Config{StallReconciles: 3})
	f.use(t, map[string]string{"git": "aaa"})
	// Runs fine, never gets anywhere.
	f.loops.criteriaMet = 0.4
	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})

	for i := 0; i < 4; i++ {
		f.pass(t, obj.ID, reconcile.TriggerManual)
		f.clock = f.clock.Add(time.Hour)
	}

	st := f.state(t, obj.ID)
	if !st.Paused {
		t.Fatalf("still running after %d reconciles without progress", st.ScoreStreak)
	}
	if st.ConsecutiveFailures != 0 {
		t.Error("a stalled objective was counted as failing; the runs succeeded, they just achieved nothing")
	}
}

// The other half of the stall detector, and the half that was wrong: an
// objective climbing 0.2 → 0.5 → 0.8 is working, and pausing it for "no
// improvement" takes a converging objective out of rotation three passes
// before it would have arrived.
//
// The bug was invisible to the test above because flat scores stall either
// way. finish() assigned st.CriteriaMet from the outcome and then compared the
// outcome against st.CriteriaMet — a value against itself, so every pass
// counted as no improvement.
func TestStallDetectorLeavesImprovingObjectivesAlone(t *testing.T) {
	f := newFixture(t, Config{StallReconciles: 3})
	f.use(t, map[string]string{"git": "aaa"})
	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})

	for _, score := range []float64{0.2, 0.5, 0.8} {
		f.loops.criteriaMet = score
		f.pass(t, obj.ID, reconcile.TriggerManual)
		f.clock = f.clock.Add(time.Hour)
	}

	st := f.state(t, obj.ID)
	if st.Paused {
		t.Fatalf("an objective improving 0.2 -> 0.5 -> 0.8 was paused as stalled: %s", st.PausedReason)
	}
	if st.ScoreStreak != 0 {
		t.Errorf("score_streak = %d after three straight improvements, want 0", st.ScoreStreak)
	}
}

// An escalation is not a failure. A loop that stopped to ask a question did
// the right thing, and a breaker counting questions would trip precisely on
// the objectives being most careful.
func TestEscalationIsNotCountedAsFailure(t *testing.T) {
	f := newFixture(t, Config{BreakerFailures: 2})
	f.use(t, map[string]string{"git": "aaa"})
	f.loops.paused = true
	f.loops.checkpoint = "cp-1"
	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyPropose, Ceiling: objective.AutonomyPropose},
	})

	f.pass(t, obj.ID, reconcile.TriggerManual)

	st := f.state(t, obj.ID)
	if st.ConsecutiveFailures != 0 {
		t.Errorf("consecutive_failures = %d after an escalation, want 0", st.ConsecutiveFailures)
	}
	if st.Phase != reconcile.PhaseWaiting {
		t.Errorf("phase = %q, want waiting", st.Phase)
	}
	if st.ActiveLoopID == "" {
		t.Error("the supervisor let go of the loop without remembering what it let go of")
	}
	if st.Paused {
		t.Error("an escalation paused the objective")
	}
}

// The supervisor must not start a second loop while one is still with a human.
func TestNoSecondLoopWhileWaitingOnAHuman(t *testing.T) {
	f := newFixture(t, Config{})
	f.use(t, map[string]string{"git": "aaa"})
	f.loops.paused = true
	f.loops.checkpoint = "cp-1"
	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyPropose, Ceiling: objective.AutonomyPropose},
	})

	f.pass(t, obj.ID, reconcile.TriggerManual)
	if f.loops.count() != 1 {
		t.Fatalf("first pass ran %d loops, want 1", f.loops.count())
	}

	f.clock = f.clock.Add(2 * time.Hour)
	f.pass(t, obj.ID, reconcile.TriggerManual)
	if got := f.loops.count(); got != 1 {
		t.Errorf("ran %d loops while one was still with a human, want 1", got)
	}
}

// A rejection is the strongest signal the system gets that it was about to do
// the wrong thing, so it demotes at once — no counter, no grace.
func TestRejectionDemotesImmediately(t *testing.T) {
	f := newFixture(t, Config{})
	f.use(t, map[string]string{"git": "aaa"})
	obj := f.declare(t, objective.Objective{
		Cadence: &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{
			Level:   objective.AutonomyActWithNotice,
			Ceiling: objective.AutonomyAct,
		},
	})

	// A completed loop whose checkpoint the reviewer rejected.
	cp, err := featurecp.NewService(f.store, event.NewHub()).Create(
		context.Background(), obj.ID, obj.TwinID, "authority_exceeded", "wants to force-push",
		[]string{"approve", "reject"}, featurecp.CreateOptions{})
	if err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}
	if err := f.store.ResolveCheckpoint(context.Background(), cp.ID, corecheckpoint.Decision{Choice: "reject"}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if err := f.store.SaveLoopState(context.Background(), coreloop.State{
		LoopID: "loop-x", ObjectiveID: obj.ID, Completed: true,
		Status: objective.StatusFailed, CheckpointID: cp.ID,
	}); err != nil {
		t.Fatalf("save loop state: %v", err)
	}
	st := f.state(t, obj.ID)
	st.ActiveLoopID = "loop-x"
	if err := f.store.SaveReconcileState(context.Background(), st); err != nil {
		t.Fatalf("save state: %v", err)
	}

	f.pass(t, obj.ID, "")

	st = f.state(t, obj.ID)
	if st.Autonomy != objective.AutonomyPropose {
		t.Errorf("autonomy = %q after a rejection, want one rung down at propose", st.Autonomy)
	}
	events, _ := f.store.ListToolEvents(context.Background(), storage.ToolEventFilter{
		ObjectiveID: string(obj.ID),
		Kind:        storage.ToolEventDemotion,
	})
	if len(events) == 0 {
		t.Error("the demotion left no audit trail")
	}
}

// Quiet hours hold back the expensive tier and never the cheap one. Work is
// deferred to the opening, not dropped.
func TestQuietHoursDeferTheExpensiveTierOnly(t *testing.T) {
	f := newFixture(t, Config{})
	f.use(t, map[string]string{"git": "aaa"})
	f.loops.criteriaMet = 1.0
	f.clock = time.Date(2026, 3, 4, 3, 0, 0, 0, time.UTC) // inside the window
	obj := f.declare(t, objective.Objective{
		Cadence: &objective.Cadence{
			Sense: "15m",
			Every: "1h",
			Quiet: []string{"22:00-07:00"},
		},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})

	f.pass(t, obj.ID, "")

	if f.loops.count() != 0 {
		t.Fatalf("reconciled at 3am inside a quiet window")
	}
	st := f.state(t, obj.ID)
	open := time.Date(2026, 3, 4, 7, 0, 0, 0, time.UTC)
	if st.NextReconcileAt == nil || !st.NextReconcileAt.Equal(open) {
		t.Errorf("next reconcile = %v, want the window's opening %s", st.NextReconcileAt, open)
	}
	// Sensing carries on through the night. That is how the seven o'clock
	// reconcile knows what happened while nobody was watching.
	if st.NextSenseAt == nil || !st.NextSenseAt.Equal(f.clock.Add(15*time.Minute)) {
		t.Errorf("next sense = %v, want 15 minutes on — the cheap tier keeps no quiet hours", st.NextSenseAt)
	}

	// And drift found during the small hours defers rather than disappearing.
	f.move("git", "bbb")
	f.clock = f.clock.Add(15 * time.Minute)
	f.pass(t, obj.ID, "")

	if f.loops.count() != 0 {
		t.Fatalf("drift at 3:15am was acted on inside a quiet window")
	}
	st = f.state(t, obj.ID)
	if st.NextDueAt == nil {
		t.Fatal("deferred work was dropped rather than rescheduled")
	}
	if !st.NextDueAt.Equal(open) {
		t.Errorf("next due = %s after deferred drift, want the window's opening %s", st.NextDueAt, open)
	}
}

func TestPausedObjectivesAreNotWorkedOn(t *testing.T) {
	f := newFixture(t, Config{})
	f.use(t, map[string]string{"git": "aaa"})
	f.loops.criteriaMet = 1.0
	obj := f.declare(t, objective.Objective{
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})
	if err := f.svc.Pause(context.Background(), obj.ID, "operator stopped it"); err != nil {
		t.Fatalf("pause: %v", err)
	}

	f.pass(t, obj.ID, reconcile.TriggerManual)
	if f.loops.count() != 0 {
		t.Error("a paused objective was reconciled")
	}
	if err := f.svc.Trigger(context.Background(), obj.ID); err == nil {
		t.Error("a manual trigger on a paused objective was accepted")
	}
}

// An objective whose declaration stops being standing loses its control loop
// rather than being worked on by a supervisor nobody asked for.
func TestDemotionToOneshotDropsTheControlLoop(t *testing.T) {
	f := newFixture(t, Config{})
	f.use(t, map[string]string{"git": "aaa"})
	obj := f.declare(t, objective.Objective{Cadence: &objective.Cadence{Every: "1h"}})

	obj.Mode = objective.ModeOneshot
	if err := f.store.SaveObjective(context.Background(), obj); err != nil {
		t.Fatalf("save: %v", err)
	}
	f.pass(t, obj.ID, "")

	if _, err := f.store.GetReconcileState(context.Background(), obj.ID); err == nil {
		t.Error("the control loop outlived the declaration that asked for it")
	}
}

func TestBackoffGrowsAndStops(t *testing.T) {
	max := 30 * time.Minute
	prev := time.Duration(0)
	for failures := 1; failures <= 12; failures++ {
		got := backoff(failures, max)
		if got < prev {
			t.Errorf("backoff(%d) = %s, shorter than backoff(%d) = %s", failures, got, failures-1, prev)
		}
		if got > max {
			t.Errorf("backoff(%d) = %s, above the ceiling %s", failures, got, max)
		}
		prev = got
	}
	if backoff(0, max) != 0 {
		t.Error("backoff with no failures is not zero")
	}
	if backoff(12, max) != max {
		t.Errorf("backoff never reached its ceiling: %s", backoff(12, max))
	}
}

// ── Phase 23: per-objective spend ceilings ────────────────────────────────

// budgetFixture wires a real in-memory ledger and pre-charges it, so the
// supervisor reads spend the same way it will in production rather than
// through a stub that cannot disagree with the recorder.
func budgetFixture(t *testing.T, spent float64) (*fixture, objective.Objective) {
	t.Helper()
	f := newFixture(t, Config{})
	ledger := cost.NewMemoryLedger()
	f.svc.quota = karakuriquota.Deps{Costs: &karakuriquota.Recorder{Ledger: ledger}}

	obj := f.declare(t, objective.Objective{
		TwinID:   "twin-1",
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
		Budget:   &objective.Budget{Daily: 10},
	})
	if spent > 0 {
		// Recorded exactly as the loop records it: the twin is the subject
		// that pays, the objective is the resource that spent.
		if err := ledger.Record(context.Background(), cost.Event{
			Subject:      karakuriquota.CostSubject("twin-1"),
			ResourceType: "objective",
			ResourceID:   string(obj.ID),
			Provider:     "claude",
			Units:        1,
			UnitKind:     cost.UnitTokens,
			Cost:         spent,
			OccurredAt:   f.clock,
		}); err != nil {
			t.Fatalf("seed ledger: %v", err)
		}
	}
	f.use(t, map[string]string{"git": "aaa"})
	return f, obj
}

// An objective that has spent its ceiling stops reconciling — and does so
// without being marked failed, blocked or paused, because none of those is
// what happened.
func TestBudgetExhaustionDefersRatherThanFailing(t *testing.T) {
	f, obj := budgetFixture(t, 12) // over the declared 10
	f.loops.criteriaMet = 1.0

	f.pass(t, obj.ID, reconcile.TriggerManual)

	if f.loops.runs != 0 {
		t.Errorf("ran %d loops after the daily ceiling was reached", f.loops.runs)
	}
	st := f.state(t, obj.ID)
	if st.Paused {
		t.Errorf("a budget pause needs an operator to clear it; reason=%q", st.PausedReason)
	}
	if st.ConsecutiveFailures != 0 {
		t.Errorf("consecutive_failures = %d; running out of money is not misbehaving", st.ConsecutiveFailures)
	}
	if st.NextDueAt == nil {
		t.Fatal("a deferred objective was left with no next due time")
	}
	// Deferred to the window boundary, not to the next cadence tick.
	if !st.NextDueAt.After(f.clock.Add(time.Hour)) {
		t.Errorf("next due %v is within the hour; the cadence scheduled over the deferral", st.NextDueAt)
	}
}

// Sensing costs adapter calls and no tokens, so an objective that cannot
// afford to act can still afford to notice — and what it noticed is recorded.
func TestSensingContinuesWhileOverBudget(t *testing.T) {
	f, obj := budgetFixture(t, 12)
	f.pass(t, obj.ID, reconcile.TriggerManual)

	history, err := f.svc.History(context.Background(), obj.ID, 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) == 0 {
		t.Fatal("a deferred pass recorded no outcome; the digest has nothing to report")
	}
	if history[0].Deferred != "budget_exhausted" {
		t.Errorf("outcome.Deferred = %q, want budget_exhausted", history[0].Deferred)
	}
	if history[0].Failed() {
		t.Error("a deferral reported itself as a failed pass")
	}
}

// Under the ceiling, nothing changes. The gate must not stop an objective
// that has room left, or the feature is just an off switch.
func TestSpendUnderTheCeilingReconcilesNormally(t *testing.T) {
	f, obj := budgetFixture(t, 3) // well under 10
	f.loops.criteriaMet = 1.0

	f.pass(t, obj.ID, reconcile.TriggerManual)

	if f.loops.runs != 1 {
		t.Errorf("ran %d loops with budget remaining, want 1", f.loops.runs)
	}
}

// A ceiling declared on a deployment with no ledger cannot be enforced. It
// must not silently stop the objective either — an unenforceable ceiling is a
// configuration problem, and refusing to run would be a worse answer than
// running and saying so.
func TestBudgetWithoutALedgerDoesNotStopTheObjective(t *testing.T) {
	f := newFixture(t, Config{})
	f.use(t, map[string]string{"git": "aaa"})
	f.loops.criteriaMet = 1.0
	obj := f.declare(t, objective.Objective{
		TwinID:   "twin-1",
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
		Budget:   &objective.Budget{Daily: 10},
	})

	f.pass(t, obj.ID, reconcile.TriggerManual)

	if f.loops.runs != 1 {
		t.Errorf("ran %d loops; an unenforceable ceiling stopped the objective", f.loops.runs)
	}
}

// An objective with no budget is exactly what it was before Phase 23.
func TestNoBudgetIsUnchanged(t *testing.T) {
	f := newFixture(t, Config{})
	f.use(t, map[string]string{"git": "aaa"})
	f.loops.criteriaMet = 1.0
	obj := f.declare(t, objective.Objective{
		TwinID:   "twin-1",
		Cadence:  &objective.Cadence{Every: "1h"},
		Autonomy: &objective.Autonomy{Level: objective.AutonomyAct, Ceiling: objective.AutonomyAct},
	})

	f.pass(t, obj.ID, reconcile.TriggerManual)

	if f.loops.runs != 1 {
		t.Errorf("ran %d loops without a declared budget, want 1", f.loops.runs)
	}
}
