package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// measureAtStartBaselines measures, once for the life of the objective, each
// measured criterion whose baseline is at_start and has no value yet (Phase 41
// step 7, decision 3 'Baselines').
//
// It runs before the first plan's actions, so the value is where the world
// stood before this objective's work touched it. The verifier is dispatched by
// stepAct, the path every planned action takes, so the quota, the execute row
// and its provenance are the ones any action gets; nothing here refuses
// anything (AGENTS.md rule 8).
//
// Once is persisted two ways. A frozen value is on the criterion in the
// objective store, and the next run reads it and measures nothing. An
// inconclusive measurement leaves the value nil, exactly as before any
// measurement, so the one baseline audit row is what says it was attempted: a
// second attempt would take its baseline from a world the work has already
// changed, and the work would be choosing what it is judged against.
func (s *serviceImpl) measureAtStartBaselines(ctx context.Context, sc *stepContext) {
	var pending []int
	for i, c := range sc.obj.SuccessCriteria {
		if c.Kind == objective.KindMeasured && c.Baseline != nil && c.Baseline.AtStart && c.BaselineValue == nil {
			pending = append(pending, i)
		}
	}
	if len(pending) == 0 {
		return
	}

	attempted, err := s.baselinesAttempted(ctx, sc.obj.ID)
	if err != nil {
		// Without the audit log the loop cannot tell an inconclusive first
		// measurement from none, and measuring again could rewrite the first.
		// Leaving the baseline unmeasured is the safe reading: the criterion is
		// inconclusive: no_baseline, and the next run asks again.
		slog.Warn("could not read the baseline audit rows; leaving at_start baselines unmeasured this run",
			"objective", string(sc.obj.ID), "error", err)
		return
	}
	for _, i := range pending {
		if attempted[sc.obj.SuccessCriteria[i].ID] {
			continue
		}
		s.measureBaseline(ctx, sc, i)
	}
}

// baselinesAttempted is the set of criterion IDs that already have a baseline
// audit row on the objective.
func (s *serviceImpl) baselinesAttempted(ctx context.Context, id objective.ObjectiveID) (map[string]bool, error) {
	rows, err := s.store.ListToolEvents(ctx, storage.ToolEventFilter{
		ObjectiveID: string(id),
		Kind:        storage.ToolEventBaseline,
	})
	if err != nil {
		return nil, err
	}
	attempted := make(map[string]bool, len(rows))
	for _, r := range rows {
		var p struct {
			Criterion string `json:"criterion"`
		}
		if json.Unmarshal([]byte(r.PayloadJSON), &p) == nil && p.Criterion != "" {
			attempted[p.Criterion] = true
		}
	}
	return attempted, nil
}

// measureBaseline runs criterion i's named verifier once, freezes the value
// its qualifying samples reduce to with the commit it was taken at, and audits
// the attempt in one row whether or not it produced a value.
func (s *serviceImpl) measureBaseline(ctx context.Context, sc *stepContext, i int) {
	c := sc.obj.SuccessCriteria[i]
	action := plannedAction{
		CapabilityID: string(c.Verifier),
		Params:       c.Params,
		Reason:       "measure the at_start baseline of criterion " + c.ID,
	}

	// The commit is the snapshot of the environment that runs the verifier,
	// taken before it runs. A blind environment has no SHA, and the row says
	// so with an empty commit rather than a guessed one (AGENTS.md rule 10).
	commit := ""
	if env, _ := s.resolveEnv(sc.envs, action); env != nil {
		if snap, err := env.Snapshot(ctx); err == nil {
			commit = snap.SHA
		}
	}

	outcomes := stepAct(ctx, sc, plan{Actions: []plannedAction{action}})
	samples, reason := qualifyingSamples(c, outcomesFor(outcomes, string(c.Verifier)))
	var value *float64
	switch {
	case len(samples) == 0:
	case len(samples) < c.MinSamples:
		reason = objective.ReasonTooFewSamples
	default:
		if v, ok := c.Reduce(samples); ok {
			value = &v
		} else {
			reason = objective.ReasonNoData
		}
	}

	if value != nil {
		if err := s.freezeBaseline(ctx, sc.obj.ID, c.ID, *value); err != nil {
			// Not frozen, so not a baseline: the run settles the criterion
			// inconclusive: no_baseline, and the row records why.
			slog.Warn("could not persist the at_start baseline", "objective", string(sc.obj.ID),
				"criterion", c.ID, "error", err)
			value = nil
			reason = objective.ReasonVerifierError
		} else {
			sc.obj.SuccessCriteria[i].BaselineValue = value
		}
	}
	s.recordBaseline(ctx, sc, c, commit, value, reason)
}

// freezeBaseline writes the value onto the stored criterion, only while it
// still has none, so nothing in the loop can rewrite a frozen baseline.
func (s *serviceImpl) freezeBaseline(ctx context.Context, id objective.ObjectiveID, criterionID string, value float64) error {
	stored, err := s.store.GetObjective(ctx, id)
	if err != nil {
		return fmt.Errorf("get objective: %w", err)
	}
	for j := range stored.SuccessCriteria {
		c := &stored.SuccessCriteria[j]
		if c.ID != criterionID {
			continue
		}
		if c.BaselineValue != nil {
			return fmt.Errorf("criterion %s already has a baseline", criterionID)
		}
		c.BaselineValue = &value
		return s.store.SaveObjective(ctx, stored)
	}
	return fmt.Errorf("criterion %s is not on the stored objective", criterionID)
}

// recordBaseline writes the one baseline audit row: the criterion, verifier,
// metric, unit and commit, with the value when one was frozen and the reason
// when none was.
func (s *serviceImpl) recordBaseline(ctx context.Context, sc *stepContext, c objective.Criterion, commit string, value *float64, reason objective.InconclusiveReason) {
	payload := map[string]any{
		"loop_id":   sc.loopID,
		"criterion": c.ID,
		"verifier":  string(c.Verifier),
		"metric":    c.Metric,
		"unit":      c.Unit,
		"commit":    commit,
	}
	if value != nil {
		payload["value"] = *value
	} else {
		payload["reason"] = string(reason)
	}
	payloadJSON, _ := json.Marshal(payload)
	now := time.Now().UTC()
	if err := s.store.SaveToolEvent(ctx, storage.ToolEvent{
		ID:          fmt.Sprintf("audit-%d", now.UnixNano()),
		ObjectiveID: string(sc.obj.ID),
		AgentID:     string(sc.agentDef.ID),
		Kind:        storage.ToolEventBaseline,
		Capability:  string(c.Verifier),
		TemplateID:  sc.obj.TemplateID,
		Success:     value != nil,
		PayloadJSON: string(payloadJSON),
		CreatedAt:   now,
	}); err != nil {
		slog.Warn("could not write the baseline audit row", "objective", string(sc.obj.ID),
			"criterion", c.ID, "error", err)
	}
}
