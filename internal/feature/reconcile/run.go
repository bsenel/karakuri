package reconcile

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/event"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	featurecp "github.com/bsenel/karakuri/internal/feature/checkpoint"
	featureloop "github.com/bsenel/karakuri/internal/feature/loop"
	"github.com/bsenel/karakuri/internal/platform/schedule"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// pass is one turn of the outer loop over a single objective.
//
// The shape is: claim it, look cheaply, decide whether looking cheaply was
// enough, and only then spend. Every early return leaves the objective
// rescheduled and the lease released, because the failure that matters here is
// not a crash — it is an objective that quietly stops being watched.
func (s *Service) pass(ctx context.Context, id objective.ObjectiveID, forced reconcile.Trigger) error {
	obj, err := s.store.GetObjective(ctx, id)
	if err != nil {
		return fmt.Errorf("load objective: %w", err)
	}
	if !obj.IsStanding() {
		// The declaration changed under us. Adoption would get here
		// eventually; doing it now stops the supervisor working on
		// something nobody asked it to.
		return s.Forget(ctx, id)
	}

	st, err := s.store.GetReconcileState(ctx, id)
	if err != nil {
		return fmt.Errorf("load reconcile state: %w", err)
	}
	if st.Paused {
		return nil
	}

	now := s.now()
	claimed, err := s.store.ClaimReconcileState(ctx, id, s.holder, now, now.Add(s.cfg.LeaseTTL))
	if err != nil {
		return fmt.Errorf("claim: %w", err)
	}
	if !claimed {
		// Another replica has it. Not an error and not worth logging: on a
		// three-replica deployment two of every three ticks land here.
		return nil
	}
	defer func() {
		_ = s.store.ReleaseReconcileLease(context.WithoutCancel(ctx), id, s.holder)
	}()

	// Re-read under the claim. Between the due query and the claim another
	// replica may have finished a pass and moved everything on.
	if fresh, err := s.store.GetReconcileState(ctx, id); err == nil {
		st = fresh
	}
	if st.Paused {
		return nil
	}

	cadence := s.effectiveCadence(obj.CadenceDeclaration())
	decl := obj.AutonomyDeclaration()
	level := st.EffectiveAutonomy(decl)

	// A loop this objective left running, because a previous pass escalated
	// and let go rather than sit on a human for a day.
	var settledConverged bool
	if st.ActiveLoopID != "" {
		done, converged, err := s.settleActiveLoop(ctx, &st, obj, decl)
		if err != nil {
			return err
		}
		settledConverged = converged
		if !done {
			// Still with the human, or still running. Come back later; do
			// not start a second loop on the same objective.
			//
			// The re-arm has to be persisted, not just held in memory: an
			// unsaved next_due_at leaves the row due on every tick, so a
			// checkpoint nobody answers for a week is a claim, a lease and
			// a concurrency slot burned every thirty seconds for a week.
			s.reschedule(&st, cadence, now.Add(s.cfg.LeaseTTL))
			return s.store.SaveReconcileState(ctx, st)
		}
	}

	started := s.now()
	envs := s.envsFor(ctx, obj)

	// The cheap tier. Always, on every pass, whatever the trigger — a
	// scheduled reconcile still wants to know what moved so its outcome can
	// say so, and it costs the same handful of adapter calls either way.
	fp := fingerprint(ctx, envs)

	// A loop that settled just now converged against the world as it stands
	// at this moment, so the baseline moves here — before the comparison, not
	// after it. Re-baselining afterwards would let the very change that loop
	// was approved to handle read as fresh drift, and a propose-level
	// objective would escalate the same change on every pass forever.
	if settledConverged && fp.SHA != "" {
		st.Converged = fp
		st.Converged.TakenAt = started
		s.publish(ctx, event.TypeConverged, obj, map[string]any{
			"criteria_met": st.CriteriaMet,
			"fingerprint":  fp.SHA,
		})
	}

	drift := compare(st.Converged, fp)
	st.LastRunAt = &started

	// First sight of a hashable world with no baseline to compare against.
	// Record it, and call it no drift — which it is: nothing has changed
	// since the only observation there has ever been.
	//
	// Without this an objective that never reconciles (one held at sense
	// level, watching and reporting) would never acquire a baseline and so
	// could never report drift, which is the one job it has.
	if st.Converged.SHA == "" && fp.SHA != "" {
		st.Converged = fp
	}

	s.publish(ctx, event.TypeReconcileSensed, obj, map[string]any{
		"drift":        drift.Changed,
		"blind":        drift.Blind,
		"environments": len(fp.Environments),
	})

	trigger := forced
	if trigger == "" {
		switch {
		case st.NextReconcileAt != nil && !started.Before(*st.NextReconcileAt):
			trigger = reconcile.TriggerSchedule
		case drift.Changed:
			trigger = reconcile.TriggerDrift
		}
	}
	if drift.Changed {
		s.publish(ctx, event.TypeDriftDetected, obj, map[string]any{
			"environments": drift.Environments,
			"from":         drift.From,
			"to":           drift.To,
		})
	}

	// Sense-only: nothing is due and nothing moved, or the objective is not
	// trusted to do anything about it yet. This is the common case and the
	// whole reason the split exists — it costs a few adapter calls and no
	// tokens at all.
	if trigger == "" || level == objective.AutonomySense {
		if drift.Changed && level == objective.AutonomySense {
			// Observed and reported, deliberately not acted on. An objective
			// that may not act can still do the one useful thing available
			// to it, which is tell somebody — so drift at this level raises
			// a checkpoint, the way watch mode did before the supervisor
			// subsumed it.
			trigger = reconcile.TriggerDrift
			s.raiseDriftCheckpoint(ctx, obj, drift)
			// Re-baseline, so the same change is reported once rather than
			// on every tick until somebody deals with it.
			st.Converged = fp
		}
		return s.finish(ctx, &st, obj, cadence, reconcile.Outcome{
			ID:          newOutcomeID(),
			ObjectiveID: obj.ID,
			TwinID:      obj.TwinID,
			Trigger:     trigger,
			Drift:       drift,
			Autonomy:    level,
			CriteriaMet: st.CriteriaMet,
			Converged:   st.LastConvergedAt != nil && !drift.Changed,
			StartedAt:   started,
			EndedAt:     s.now(),
		}, nil)
	}

	// The floor and the quiet hours apply to the expensive tier only. Sensing
	// through the night is how the morning reconcile knows what happened.
	allowed, deferUntil, err := schedule.Allowed(cadence, s.reference(st, started), started)
	if err != nil {
		return fmt.Errorf("cadence: %w", err)
	}
	if !allowed {
		st.NextDueAt = nilOrTime(deferUntil)
		st.NextReconcileAt = nilOrTime(deferUntil)
		st.Phase = reconcile.PhaseIdle
		return s.store.SaveReconcileState(ctx, st)
	}

	return s.reconcileNow(ctx, &st, obj, decl, level, cadence, trigger, drift, fp, started)
}

// reconcileNow is the expensive half: start a loop under the objective's
// earned authority, wait for it to finish or escalate, and fold the answer
// back into the state.
func (s *Service) reconcileNow(
	ctx context.Context,
	st *reconcile.State,
	obj objective.Objective,
	decl objective.Autonomy,
	level objective.AutonomyLevel,
	cadence objective.Cadence,
	trigger reconcile.Trigger,
	drift reconcile.Drift,
	fp reconcile.Fingerprint,
	started time.Time,
) error {
	twin, _ := s.store.GetTwin(ctx, obj.TwinID)

	// The one place autonomy becomes enforcement. The definition the loop
	// would have chosen for itself, with its bounds rewritten to what this
	// objective has earned — and then the loop enforces those bounds through
	// the decide step it has always used.
	def := featureloop.SelectAgent(s.domReg, obj, coreagent.Definition{})
	def.Authority = effectiveAuthority(def, level)

	st.Phase = reconcile.PhaseReconciling
	st.LastTrigger = trigger
	_ = s.store.SaveReconcileState(ctx, *st)

	s.publish(ctx, event.TypeReconcileStarted, obj, map[string]any{
		"trigger":  string(trigger),
		"autonomy": string(level),
		"drift":    drift.Changed,
	})

	outcome := reconcile.Outcome{
		ID:          newOutcomeID(),
		ObjectiveID: obj.ID,
		TwinID:      obj.TwinID,
		Trigger:     trigger,
		Drift:       drift,
		Autonomy:    level,
		CriteriaMet: st.CriteriaMet,
		StartedAt:   started,
	}

	res, err := s.loops.Run(ctx, coreloop.Request{
		Objective: obj,
		Twin:      twin,
		Agent:     def,
		MaxIter:   obj.MaxIterations,
	})
	if err != nil {
		outcome.Error = err.Error()
		outcome.EndedAt = s.now()
		return s.finish(ctx, st, obj, cadence, outcome, &fp)
	}
	outcome.LoopID = res.LoopID

	final, escalated := s.awaitLoop(ctx, res.LoopID, obj.ID)
	outcome.EndedAt = s.now()
	outcome.CriteriaMet = final.CriteriaMet
	outcome.CheckpointID = final.CheckpointID
	outcome.Escalated = escalated

	switch {
	case escalated:
		// Let go. A reconcile that stopped to ask a human could be waiting
		// for days, and a supervisor sitting on it would hold a lease and a
		// concurrency slot for all of them — starving every other standing
		// objective on the deployment to babysit one question.
		st.ActiveLoopID = res.LoopID
		st.Phase = reconcile.PhaseWaiting
	case final.Status == objective.StatusFailed:
		outcome.Error = failureReason(final)
	default:
		outcome.Converged = final.CriteriaMet >= 1.0
	}
	return s.finish(ctx, st, obj, cadence, outcome, &fp)
}

// awaitLoop waits for a loop to finish or to stop at a checkpoint, renewing
// the lease as it goes.
//
// It watches the persisted loop state rather than holding a channel, because
// the row is the thing that survives this process. Polling is honest here: a
// reconcile lasts minutes to hours, so a two-second poll is a rounding error
// against it, and it buys a wait that a restart can simply redo. The same
// ticker renews the lease, so a reconcile that is making progress keeps its
// claim alive by the very act of watching itself.
func (s *Service) awaitLoop(ctx context.Context, loopID string, objID objective.ObjectiveID) (coreloop.State, bool) {
	ticker := time.NewTicker(s.cfg.LoopPoll)
	defer ticker.Stop()

	// The displacement check below cannot rescue a wedged watch on its own:
	// this pass renews its own lease on every tick, so it can never be
	// displaced by anybody. The deadline is what stops a loop whose goroutine
	// died from holding a concurrency slot until the process restarts.
	deadline := s.now().Add(s.cfg.MaxLoopWait)

	var last coreloop.State
	for {
		select {
		case <-ctx.Done():
			return last, false
		case <-ticker.C:
			if s.now().After(deadline) {
				slog.Warn("gave up watching a reconcile loop",
					"objective", string(objID), "loop", loopID,
					"waited", s.cfg.MaxLoopWait.String())
				// Let go the same way an escalation does: the objective
				// keeps ActiveLoopID, so a later pass settles it if it ever
				// finishes, and the slot is freed meanwhile.
				return last, true
			}
			ls, err := s.store.GetLoopState(ctx, loopID)
			if err != nil {
				continue
			}
			last = ls
			if ls.Completed {
				return ls, false
			}
			if ls.Paused {
				return ls, true
			}
			now := s.now()
			if ok, err := s.store.RenewReconcileLease(ctx, objID, s.holder, now, now.Add(s.cfg.LeaseTTL)); err == nil && !ok {
				// Displaced. Somebody else owns this objective now, and
				// continuing to write its state would be two supervisors
				// disagreeing about one row.
				return last, false
			}
		}
	}
}

// settleActiveLoop folds in a loop that a previous pass left running.
//
// Returns whether the objective is free to be worked on again, and whether the
// loop that just settled reached its criteria. A loop still waiting on a human
// is not free, and neither is one still executing.
//
// The caller needs the second answer because a loop that converged while the
// supervisor was away converged against the world as it stands now. Its
// baseline has to move with it, and only the caller holds the fresh
// fingerprint to move it to.
func (s *Service) settleActiveLoop(ctx context.Context, st *reconcile.State, obj objective.Objective, decl objective.Autonomy) (done, converged bool, err error) {
	ls, err := s.store.GetLoopState(ctx, st.ActiveLoopID)
	if err != nil {
		// The row is gone — a pruned database, or a loop that never
		// persisted. Nothing to wait for.
		st.ActiveLoopID = ""
		return true, false, nil
	}
	if !ls.Completed {
		if ls.Paused {
			st.Phase = reconcile.PhaseWaiting
			return false, false, nil
		}
		st.Phase = reconcile.PhaseReconciling
		return false, false, nil
	}

	// The human answered and the loop ran on to its end. What they answered
	// is the part that matters for trust: a rejection is the strongest signal
	// the system gets that it was about to do the wrong thing.
	rejected := false
	if ls.CheckpointID != "" {
		if cp, cerr := s.store.GetCheckpoint(ctx, ls.CheckpointID); cerr == nil && cp.Decision != nil {
			rejected = cp.Decision.Choice == "reject"
		}
	}

	now := s.now()
	st.ActiveLoopID = ""
	st.Phase = reconcile.PhaseIdle
	st.LastReconciledAt = &now
	st.CriteriaMet = ls.CriteriaMet

	if rejected {
		s.applyDemotion(ctx, st, obj, decl, "checkpoint_rejected")
		st.CleanRuns = 0
		return true, false, s.store.SaveReconcileState(ctx, *st)
	}

	if ls.CriteriaMet >= 1.0 {
		st.LastConvergedAt = &now
		st.ScoreStreak = 0
		s.applyCleanRun(ctx, st, obj, decl)
		_ = s.store.UpdateObjectiveStatus(ctx, obj.ID, objective.StatusConverged)
		return true, true, s.store.SaveReconcileState(ctx, *st)
	}
	return true, false, s.store.SaveReconcileState(ctx, *st)
}

// finish records the pass, moves the counters, and re-arms the schedule. It is
// the single exit from a pass, so every path that can leave an objective
// unscheduled has to go through the place that schedules it.
func (s *Service) finish(
	ctx context.Context,
	st *reconcile.State,
	obj objective.Objective,
	cadence objective.Cadence,
	outcome reconcile.Outcome,
	fp *reconcile.Fingerprint,
) error {
	now := s.now()
	decl := obj.AutonomyDeclaration()

	// The score this pass has to beat, captured before st.CriteriaMet is
	// overwritten below. Comparing the outcome against the field it was just
	// written into compares a value with itself, which is always "no
	// improvement" — and stalls objectives that are converging nicely.
	previousScore := st.CriteriaMet

	st.LastOutcomeID = outcome.ID
	st.CriteriaMet = outcome.CriteriaMet
	if outcome.Trigger != "" {
		st.LastTrigger = outcome.Trigger
	}

	switch {
	// Failure is tested before the sense-only case, and the order matters: a
	// loop that could not be *started* never gets a loop ID, so asking "was
	// there a loop" first would file a broken objective as a quiet one and
	// leave the circuit breaker at zero forever.
	case outcome.Failed():
		st.ConsecutiveFailures++
		st.LastError = outcome.Error
		st.CleanRuns = 0
		st.Phase = reconcile.PhaseIdle
		st.LastReconciledAt = &now
		if decl.DemoteOnFailure {
			s.applyDemotion(ctx, st, obj, decl, "reconcile_failed")
		}
		s.publish(ctx, event.TypeReconcileFailed, obj, map[string]any{
			"error":                outcome.Error,
			"consecutive_failures": st.ConsecutiveFailures,
		})

	case outcome.LoopID == "":
		// A sense-only pass changes no counters. It is a look, not an
		// attempt, and counting looks as attempts would trip the stall
		// detector on an objective that is simply quiet.
		st.Phase = reconcile.PhaseIdle

	case outcome.Escalated:
		// Neither a success nor a failure: the loop did exactly what a
		// bounded agent should do. The counters wait for the human.
		st.LastError = ""
		st.LastReconciledAt = &now

	default:
		st.ConsecutiveFailures = 0
		st.LastError = ""
		st.LastReconciledAt = &now
		st.Phase = reconcile.PhaseIdle

		if outcome.Converged {
			// The fingerprint is recorded at the moment of convergence,
			// not at the moment of observation. Drift then means "moved
			// since it was last right", which is the question worth
			// asking.
			if fp != nil {
				st.Converged = *fp
				st.Converged.TakenAt = now
			}
			st.LastConvergedAt = &now
			st.ScoreStreak = 0
			s.applyCleanRun(ctx, st, obj, decl)
			_ = s.store.UpdateObjectiveStatus(ctx, obj.ID, objective.StatusConverged)
			s.publish(ctx, event.TypeConverged, obj, map[string]any{
				"criteria_met": outcome.CriteriaMet,
				"fingerprint":  st.Converged.SHA,
			})
		} else if outcome.CriteriaMet <= previousScore {
			st.ScoreStreak++
		} else {
			st.ScoreStreak = 0
		}
	}

	if err := s.store.SaveReconcileOutcome(ctx, outcome); err != nil {
		return fmt.Errorf("record outcome: %w", err)
	}

	if s.trip(ctx, st, obj) {
		return s.store.SaveReconcileState(ctx, *st)
	}

	var retryAt time.Time
	if st.ConsecutiveFailures > 0 {
		retryAt = now.Add(backoff(st.ConsecutiveFailures, s.cfg.MaxBackoff))
	}
	s.reschedule(st, cadence, retryAt)
	return s.store.SaveReconcileState(ctx, *st)
}

// trip is the circuit breaker and the stall detector, which are the same
// decision reached two ways: this objective should stop costing money until a
// person looks at it.
//
// Both raise a checkpoint rather than only setting a flag. An objective that
// went quiet with no explanation is indistinguishable from one that is
// converged and content, and that is the failure mode worth spending a
// checkpoint to avoid.
func (s *Service) trip(ctx context.Context, st *reconcile.State, obj objective.Objective) bool {
	var reason, summary string
	switch {
	case st.ConsecutiveFailures >= s.cfg.BreakerFailures:
		reason = "reconcile_circuit_breaker"
		summary = fmt.Sprintf("%d consecutive failed reconciles; last error: %s",
			st.ConsecutiveFailures, st.LastError)
	case st.ScoreStreak >= s.cfg.StallReconciles:
		reason = "reconcile_stalled"
		summary = fmt.Sprintf("%d reconciles without improving the criteria score (holding at %.2f)",
			st.ScoreStreak, st.CriteriaMet)
	default:
		return false
	}

	st.Paused = true
	st.PausedReason = summary
	st.Phase = reconcile.PhasePaused
	st.NextDueAt = nil

	if s.cpSvc != nil {
		_, _ = s.cpSvc.Create(ctx, obj.ID, obj.TwinID, reason, summary,
			[]string{"resume", "pause", "investigate"}, featurecp.CreateOptions{})
	}
	s.publish(ctx, event.TypeObjectiveSuspended, obj, map[string]any{
		"reason":  reason,
		"summary": summary,
	})
	return true
}

// applyCleanRun counts a reconcile nobody objected to and promotes if the
// declaration says that is now enough.
func (s *Service) applyCleanRun(ctx context.Context, st *reconcile.State, obj objective.Objective, decl objective.Autonomy) {
	st.CleanRuns++
	current := st.EffectiveAutonomy(decl)
	next, moved := promote(decl, current, st.CleanRuns)
	if !moved {
		return
	}
	st.Autonomy = next
	st.CleanRuns = 0
	s.recordAutonomyChange(ctx, obj, current, next, "promoted_after_clean_runs")
}

func (s *Service) applyDemotion(ctx context.Context, st *reconcile.State, obj objective.Objective, decl objective.Autonomy, reason string) {
	current := st.EffectiveAutonomy(decl)
	next, moved := demote(decl, current)
	st.CleanRuns = 0
	if !moved {
		return
	}
	st.Autonomy = next
	s.recordAutonomyChange(ctx, obj, current, next, reason)
}

// recordAutonomyChange writes the movement to the audit log and the stream.
//
// A change in what Karakuri may do to the world without asking is a
// security-relevant event, so it gets its own audit row rather than being
// inferable from a reconcile outcome. Somebody reviewing why an agent acted
// unsupervised should find the moment it was allowed to, and who or what
// allowed it.
func (s *Service) recordAutonomyChange(ctx context.Context, obj objective.Objective, from, to objective.AutonomyLevel, reason string) {
	kind := storage.ToolEventPromotion
	if to.Rank() < from.Rank() {
		kind = storage.ToolEventDemotion
	}
	s.saveAudit(ctx, obj, kind, reason, map[string]any{
		"from":   string(from),
		"to":     string(to),
		"reason": reason,
	})
	s.publish(ctx, event.TypeAutonomyChanged, obj, map[string]any{
		"from":   string(from),
		"to":     string(to),
		"reason": reason,
	})
}

// effectiveCadence fills in the deployment-wide floor for an objective whose
// own declaration did not name one.
//
// Without this, reconcile.default_min_interval is a setting that reads as a
// guardrail and does nothing: schedule.Allowed only ever sees the cadence, and
// a cadence with no min_interval imposes no floor at all. An objective sensing
// every 30 seconds against a busy repository would then drive a paid loop on
// every push, which is the exact thing the setting claims to prevent.
func (s *Service) effectiveCadence(c objective.Cadence) objective.Cadence {
	if c.MinIntervalDuration() <= 0 && s.cfg.DefaultMinInterval > 0 {
		c.MinInterval = s.cfg.DefaultMinInterval.String()
	}
	return c
}

// reschedule re-arms the due-wheel. A non-zero floor (a backoff, a lease we
// want to outlive) pushes the next attempt out without disturbing the cadence
// the operator declared.
func (s *Service) reschedule(st *reconcile.State, cadence objective.Cadence, notBefore time.Time) {
	now := s.now()
	plan, err := schedule.Next(cadence, s.reference(*st, now))
	if err != nil {
		// A cadence that will not parse should not silently stop the
		// objective; retry on the floor and let Validate complain where a
		// human is watching.
		if notBefore.IsZero() {
			notBefore = now.Add(s.cfg.DefaultMinInterval)
		}
		st.NextDueAt = nilOrTime(notBefore)
		return
	}
	applyPlan(st, plan)

	if !notBefore.IsZero() && (st.NextDueAt == nil || st.NextDueAt.Before(notBefore)) {
		st.NextDueAt = nilOrTime(notBefore)
	}
}

// backoff grows the gap after repeated failures and stops growing at the
// configured ceiling, so a long-broken objective still retries occasionally
// rather than effectively never.
func backoff(failures int, max time.Duration) time.Duration {
	if failures <= 0 {
		return 0
	}
	d := time.Minute
	for i := 1; i < failures && d < max; i++ {
		d *= 2
	}
	if d > max {
		d = max
	}
	return d
}

// failureReason explains a loop that ended failed. The loop state carries the
// status but not the cause, so a rejected checkpoint is named as such and
// everything else is reported as what it is: unexplained here, and explained
// in the audit log the loop already wrote.
func failureReason(ls coreloop.State) string {
	if ls.CheckpointID != "" {
		return "loop ended failed after checkpoint " + ls.CheckpointID
	}
	return "loop ended without meeting its success criteria"
}

// raiseDriftCheckpoint tells a human about drift on an objective that is not
// trusted to do anything about it.
//
// The options are the ones watch mode offered, because this is what watch mode
// was: promote it into work, dismiss it, or go and look. What changed is that
// the watching now survives a restart — watch mode's ticker lived in the
// goroutine of a finished loop and died with the process.
func (s *Service) raiseDriftCheckpoint(ctx context.Context, obj objective.Objective, drift reconcile.Drift) {
	if s.cpSvc == nil {
		return
	}
	summary := fmt.Sprintf("drift on %v: %s → %s", drift.Environments, short(drift.From), short(drift.To))
	_, _ = s.cpSvc.Create(ctx, obj.ID, obj.TwinID, "environment_changed", summary,
		[]string{"promote", "dismiss", "investigate"}, featurecp.CreateOptions{})
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	if sha == "" {
		return "(none)"
	}
	return sha
}

// publish emits a supervisor event. ObjectiveID and TwinID are always set:
// they are what the global stream's filter classifies on, and an event naming
// neither would be unclassifiable and therefore withheld from everybody.
func (s *Service) publish(ctx context.Context, typ string, obj objective.Objective, payload map[string]any) {
	if s.hub == nil {
		return
	}
	s.hub.Publish(ctx, event.Event{
		Type:        typ,
		ObjectiveID: string(obj.ID),
		TwinID:      obj.TwinID,
		Payload:     payload,
		Timestamp:   s.now(),
	})
}

// saveAudit writes one row to the same tool_events log the loop, the
// escalations and the RBAC refusals already share. One log rather than a
// second one for supervisor events: an operator asking "what happened to this
// objective" should get one answer in one place.
func (s *Service) saveAudit(ctx context.Context, obj objective.Objective, kind, reason string, payload map[string]any) {
	pj, _ := json.Marshal(payload)
	_ = s.store.SaveToolEvent(ctx, storage.ToolEvent{
		ID:               "audit-" + newOutcomeID(),
		ObjectiveID:      string(obj.ID),
		Kind:             kind,
		EscalationReason: reason,
		PayloadJSON:      string(pj),
		Success:          true,
	})
}
