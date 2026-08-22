// Package reconcile holds standing objectives at the state they declare.
//
// The loop converges once. This is the thing that keeps calling it: a single
// supervisor goroutine that asks which objectives are due, senses cheaply
// whether the world has moved, and spends a model call only when it has — or
// when a clock said to look anyway.
//
// It is a caller of the loop, not a change to it. The six steps are untouched;
// what the supervisor contributes is *when* to run them, *how far* the
// objective is trusted this time (written into the request's authority bounds,
// which the decide step has enforced since Phase 1), and what to do with the
// answer. That boundary is the reason this feature adds no second policy
// engine and no second execution path.
package reconcile

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
	"github.com/bsenel/karakuri/internal/core/event"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	featurecp "github.com/bsenel/karakuri/internal/feature/checkpoint"
	featureloop "github.com/bsenel/karakuri/internal/feature/loop"
	"github.com/bsenel/karakuri/internal/platform/schedule"
	"github.com/bsenel/karakuri/internal/platform/storage"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
)

// LoopRunner is the slice of the loop service this package needs.
//
// Declared here rather than imported wholesale so the supervisor's own tests
// can run a fake loop, and so the dependency reads as what it is: the
// supervisor starts loops and does not otherwise reach into them.
type LoopRunner interface {
	Run(ctx context.Context, req coreloop.Request) (coreloop.Result, error)
}

// Config is the operator's ceiling on everything the supervisor does.
type Config struct {
	Tick               time.Duration
	MaxConcurrent      int
	LeaseTTL           time.Duration
	BreakerFailures    int
	StallReconciles    int
	DefaultMinInterval time.Duration
	MaxBackoff         time.Duration

	// LoopPoll is how often a running reconcile checks whether its loop has
	// finished, and doubles as the lease heartbeat. Not exposed in the config
	// file: it trades a little polling against how long a finished loop goes
	// unnoticed, and neither side of that is an operator's decision. Tests
	// set it so they do not pay two seconds per loop.
	LoopPoll time.Duration

	// MaxLoopWait bounds how long one pass will watch a single loop before
	// letting go of it. The lease heartbeat rides on the same ticker, so a
	// loop whose goroutine has died — a panic, a wedged adapter, a provider
	// socket that never answers — leaves a row that is neither completed nor
	// paused, and a watcher that renews its own claim forever. Without a
	// ceiling that pass holds one of MaxConcurrent slots for the life of the
	// process, and four of them stop the supervisor dead.
	MaxLoopWait time.Duration
}

func (c Config) withDefaults() Config {
	if c.Tick <= 0 {
		c.Tick = 30 * time.Second
	}
	if c.MaxConcurrent <= 0 {
		c.MaxConcurrent = 4
	}
	if c.LeaseTTL <= 0 {
		c.LeaseTTL = 5 * time.Minute
	}
	if c.BreakerFailures <= 0 {
		c.BreakerFailures = 3
	}
	if c.StallReconciles <= 0 {
		c.StallReconciles = 3
	}
	if c.DefaultMinInterval <= 0 {
		c.DefaultMinInterval = 10 * time.Minute
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = time.Hour
	}
	if c.MaxLoopWait <= 0 {
		// Generous on purpose: a real reconcile can run for hours, and
		// cutting a working loop loose is worse than watching a dead one
		// for a while. This is a stuck-detector, not a timeout.
		c.MaxLoopWait = 6 * time.Hour
	}
	if c.LoopPoll <= 0 {
		c.LoopPoll = 2 * time.Second
	}
	return c
}

type Service struct {
	store  storage.StorageAdapter
	loops  LoopRunner
	envReg *environment.Registry
	domReg *domain.Registry
	cpSvc  *featurecp.Service
	hub    *event.Hub
	cfg    Config

	// quota answers what an objective has spent today. Zero value is a
	// deployment with no ledger, in which case a declared budget cannot be
	// enforced and says so rather than silently permitting everything.
	quota karakuriquota.Deps

	// holder identifies this replica in the lease. Hostname and PID rather
	// than a random value alone, so an operator looking at a stuck lease can
	// tell which process is holding it.
	holder string

	// inflight bounds concurrent reconciles. Loops today are unbounded
	// detached goroutines; a hundred standing objectives coming due together
	// would start a hundred concurrent model-calling loops, and the failure
	// mode arrives as a bill and a rate-limit wall at the same moment.
	inflight chan struct{}

	// running guards against a second pass over an objective this replica is
	// already working on. The lease covers other replicas; this covers this
	// one, where a slow reconcile would otherwise be picked up again by the
	// next tick before its lease was even close to expiring.
	mu      sync.Mutex
	running map[objective.ObjectiveID]bool

	// now is injectable so tests can drive the schedule without sleeping.
	now func() time.Time
}

func NewService(
	store storage.StorageAdapter,
	loops LoopRunner,
	envReg *environment.Registry,
	domReg *domain.Registry,
	cpSvc *featurecp.Service,
	hub *event.Hub,
	quotaDeps karakuriquota.Deps,
	cfg Config,
) *Service {
	cfg = cfg.withDefaults()
	return &Service{
		store:    store,
		loops:    loops,
		envReg:   envReg,
		domReg:   domReg,
		cpSvc:    cpSvc,
		hub:      hub,
		quota:    quotaDeps,
		cfg:      cfg,
		holder:   newHolderID(),
		inflight: make(chan struct{}, cfg.MaxConcurrent),
		running:  map[objective.ObjectiveID]bool{},
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// Holder is this replica's identity in the lease, exposed for diagnostics.
func (s *Service) Holder() string { return s.holder }

// Start launches the supervisor.
//
// One goroutine and one ticker for the whole deployment, not one per
// objective: the due-wheel is a single indexed query, so a thousand standing
// objectives cost one statement every tick rather than a thousand sleeping
// goroutines. It follows the shape the retention sweeps in bootstrap.go
// already use, including their reason for an early first pass — a server that
// restarts more often than its interval would otherwise never get round to
// any work at all.
func (s *Service) Start(ctx context.Context) {
	slog.Info("reconcile supervisor started",
		"holder", s.holder,
		"tick", s.cfg.Tick.String(),
		"max_concurrent", s.cfg.MaxConcurrent,
		"lease_ttl", s.cfg.LeaseTTL.String())

	go func() {
		// Adopt before the first tick so standing objectives declared by a
		// previous process — or by another replica — have state rows to be
		// due on.
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
			if err := s.Adopt(ctx); err != nil {
				slog.Warn("reconcile adoption failed", "err", err)
			}
			s.Tick(ctx)
		}

		ticker := time.NewTicker(s.cfg.Tick)
		defer ticker.Stop()
		// Adoption is far cheaper than a reconcile but not free, so it runs
		// on its own slower beat rather than on every tick.
		adopt := time.NewTicker(maxDuration(s.cfg.Tick*10, time.Minute))
		defer adopt.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-adopt.C:
				if err := s.Adopt(ctx); err != nil {
					slog.Warn("reconcile adoption failed", "err", err)
				}
			case <-ticker.C:
				s.Tick(ctx)
			}
		}
	}()
}

// Tick runs one due-wheel pass. Exported so tests can drive the supervisor
// deterministically instead of waiting on wall time.
func (s *Service) Tick(ctx context.Context) {
	now := s.now()
	due, err := s.store.ListDueReconcileStates(ctx, s.holder, now, s.cfg.MaxConcurrent*4)
	if err != nil {
		slog.Warn("reconcile due query failed", "err", err)
		return
	}
	for _, st := range due {
		s.dispatch(ctx, st.ObjectiveID, "")
	}
}

// dispatch runs one objective's pass in the background, under both the
// concurrency bound and the this-replica guard.
//
// A slot that is not free means the objective is left due; the next tick picks
// it up, and the ordering in ListDueReconcileStates means the most overdue
// objectives are the ones that get the slots.
func (s *Service) dispatch(ctx context.Context, id objective.ObjectiveID, forced reconcile.Trigger) {
	s.mu.Lock()
	if s.running[id] {
		s.mu.Unlock()
		return
	}
	s.running[id] = true
	s.mu.Unlock()

	select {
	case s.inflight <- struct{}{}:
	default:
		s.mu.Lock()
		delete(s.running, id)
		s.mu.Unlock()
		return
	}

	go func() {
		defer func() {
			<-s.inflight
			s.mu.Lock()
			delete(s.running, id)
			s.mu.Unlock()
		}()
		if err := s.pass(ctx, id, forced); err != nil {
			slog.Warn("reconcile pass failed", "objective", string(id), "err", err)
		}
	}()
}

// Adopt gives every standing objective a control-loop state row, and takes it
// away from every objective that is no longer standing.
//
// It runs at boot and on a slow beat afterwards rather than only when an
// objective is declared, because a declaration can be written by any replica
// and by a migration and by a hand-edited row. Making adoption idempotent and
// periodic means the supervisor converges on its own inputs the same way it
// converges on everything else, instead of depending on having been told.
func (s *Service) Adopt(ctx context.Context) error {
	standing, err := s.store.ListObjectives(ctx, storage.ObjectiveFilter{Mode: string(objective.ModeStanding)})
	if err != nil {
		return fmt.Errorf("list standing objectives: %w", err)
	}
	for _, obj := range standing {
		_, err := s.store.GetReconcileState(ctx, obj.ID)
		if err == nil {
			continue
		}
		if !errors.Is(err, coreerrors.ErrNotFound) {
			// A database that answered "I cannot tell you" is not a
			// database that said "there is no such row". Adopting on this
			// error would rebuild the state from scratch and throw away
			// earned autonomy, the failure counters and the converged
			// fingerprint. Skip; adoption is periodic and will retry.
			slog.Warn("could not read reconcile state; leaving it alone",
				"objective", string(obj.ID), "err", err)
			continue
		}
		if err := s.Declare(ctx, obj); err != nil {
			slog.Warn("could not adopt standing objective", "objective", string(obj.ID), "err", err)
		}
	}

	// And the other half of the promise: take the control loop away from
	// anything that has stopped being standing. Undeclaring through the API
	// already does this, but a mode changed by a migration or a hand-edited
	// row leaves a state row nothing else will ever collect — the due query
	// only returns rows that are due, so an orphan parked far in the future
	// is invisible to every other path.
	ids, err := s.store.ListReconcileStateIDs(ctx)
	if err != nil {
		// Adoption already did its useful half; sweeping is best effort.
		return nil
	}
	keep := make(map[objective.ObjectiveID]struct{}, len(standing))
	for _, obj := range standing {
		keep[obj.ID] = struct{}{}
	}
	for _, id := range ids {
		if _, ok := keep[id]; ok {
			continue
		}
		if err := s.store.DeleteReconcileState(ctx, id); err != nil {
			slog.Warn("could not drop orphaned reconcile state", "objective", string(id), "err", err)
		}
	}
	return nil
}

// Declare creates or refreshes the control-loop state for a standing
// objective, and removes it for one that has stopped being standing.
//
// A freshly declared objective is due immediately. Somebody has just written
// down a state they want held, and the first useful thing to tell them is
// whether the world already matches it.
func (s *Service) Declare(ctx context.Context, obj objective.Objective) error {
	if !obj.IsStanding() {
		return s.Forget(ctx, obj.ID)
	}
	cadence := obj.CadenceDeclaration()
	if err := schedule.Validate(cadence); err != nil {
		return err
	}

	now := s.now()
	existing, err := s.store.GetReconcileState(ctx, obj.ID)
	if err != nil && !errors.Is(err, coreerrors.ErrNotFound) {
		// Creating a fresh state here would silently discard whatever the
		// existing row holds — earned autonomy, the breaker's pause, the
		// converged fingerprint. A read that failed for any reason other
		// than absence is a reason to stop, not to start over.
		return fmt.Errorf("read reconcile state: %w", err)
	}
	if err != nil {
		st := reconcile.State{
			ObjectiveID: obj.ID,
			TwinID:      obj.TwinID,
			Phase:       reconcile.PhaseIdle,
			NextDueAt:   &now,
			Autonomy:    obj.AutonomyDeclaration().EffectiveLevel(),
			CreatedAt:   now,
		}
		plan, perr := schedule.Next(cadence, schedule.Reference{Now: now})
		if perr == nil {
			applyPlan(&st, plan)
		}
		return s.store.SaveReconcileState(ctx, st)
	}

	// An edited declaration re-plans from the same history. The earned
	// autonomy is re-clamped rather than reset: lowering a ceiling should cut
	// an objective back immediately, and raising one should not hand it the
	// new headroom it has not earned.
	existing.TwinID = obj.TwinID
	existing.Autonomy = existing.EffectiveAutonomy(obj.AutonomyDeclaration())
	if plan, perr := schedule.Next(cadence, s.reference(existing, now)); perr == nil {
		applyPlan(&existing, plan)
	}
	return s.store.SaveReconcileState(ctx, existing)
}

// Forget drops the control loop for an objective that is no longer standing.
// Missing rows are not an error: this is called on every update of every
// oneshot objective too.
func (s *Service) Forget(ctx context.Context, id objective.ObjectiveID) error {
	if _, err := s.store.GetReconcileState(ctx, id); err != nil {
		return nil
	}
	return s.store.DeleteReconcileState(ctx, id)
}

// Pause takes an objective out of rotation. Used by an operator, and by the
// circuit breaker and the stall detector.
func (s *Service) Pause(ctx context.Context, id objective.ObjectiveID, reason string) error {
	st, err := s.store.GetReconcileState(ctx, id)
	if err != nil {
		return err
	}
	st.Paused = true
	st.PausedReason = reason
	st.Phase = reconcile.PhasePaused
	return s.store.SaveReconcileState(ctx, st)
}

// Resume puts a paused objective back into rotation and clears the counters
// that stopped it.
//
// Clearing them is the point. An operator resuming an objective is saying they
// have looked at why it broke; leaving the failure count at its ceiling would
// trip the breaker again on the next stumble, and leaving the score streak
// would escalate again before the objective had a chance to make progress.
func (s *Service) Resume(ctx context.Context, id objective.ObjectiveID) error {
	st, err := s.store.GetReconcileState(ctx, id)
	if err != nil {
		return err
	}
	now := s.now()
	st.Paused = false
	st.PausedReason = ""
	st.Phase = reconcile.PhaseIdle
	st.ConsecutiveFailures = 0
	st.ScoreStreak = 0
	st.LastError = ""
	st.NextDueAt = &now
	return s.store.SaveReconcileState(ctx, st)
}

// Trigger reconciles an objective now, outside its cadence.
//
// It still goes through the lease and the concurrency bound: "now" means as
// soon as this replica has a slot and nobody else is working on the objective,
// not "in addition to whatever is already running".
func (s *Service) Trigger(ctx context.Context, id objective.ObjectiveID) error {
	st, err := s.store.GetReconcileState(ctx, id)
	if err != nil {
		return err
	}
	if st.Paused {
		return fmt.Errorf("objective %q is paused: %s", id, st.PausedReason)
	}
	// Detached from the caller. This is reached from an HTTP handler that
	// returns 202 the moment dispatch hands off, and net/http cancels the
	// request context at that point — so a pass holding it would be killed
	// before its first query and the operator would be told the reconcile
	// had started when nothing ever ran.
	//
	// Tick's context is deliberately left alone: there, cancellation is how
	// shutdown stops the wheel.
	s.dispatch(context.WithoutCancel(ctx), id, reconcile.TriggerManual)
	return nil
}

// State returns one objective's control-loop state.
func (s *Service) State(ctx context.Context, id objective.ObjectiveID) (reconcile.State, error) {
	return s.store.GetReconcileState(ctx, id)
}

// History returns recent passes, newest first — the cheap sense-only ones
// included, because "checked forty-eight times today and spent nothing" is
// what tells an operator the two-tier split is working.
func (s *Service) History(ctx context.Context, id objective.ObjectiveID, limit int) ([]reconcile.Outcome, error) {
	return s.store.ListReconcileOutcomes(ctx, id, limit)
}

// reference assembles what the scheduler needs from what the state remembers.
func (s *Service) reference(st reconcile.State, now time.Time) schedule.Reference {
	ref := schedule.Reference{Now: now}
	if st.LastRunAt != nil {
		ref.LastSensedAt = *st.LastRunAt
	}
	if st.LastReconciledAt != nil {
		ref.LastReconciledAt = *st.LastReconciledAt
	}
	return ref
}

// applyPlan copies a resolved schedule onto the state. NextDueAt is left alone
// when it is already sooner than the plan — a manual trigger or a backoff has
// asked for something the cadence does not know about.
func applyPlan(st *reconcile.State, plan schedule.Plan) {
	st.NextSenseAt = nilOrTime(plan.Sense)
	st.NextReconcileAt = nilOrTime(plan.Reconcile)
	st.NextDueAt = nilOrTime(plan.Due)
}

func nilOrTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}

func newHolderID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%d-%s", host, os.Getpid(), hex.EncodeToString(b))
}

func newOutcomeID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

// envsFor builds the environments an objective observes. Delegated to the loop
// package so the set the supervisor hashes is exactly the set the loop
// observes — watching one world and converging another is the kind of bug that
// shows up as "it says nothing changed but it clearly did".
func (s *Service) envsFor(ctx context.Context, obj objective.Objective) []environment.Environment {
	return featureloop.BuildEnvironments(ctx, s.store, s.envReg, s.hub, obj)
}
