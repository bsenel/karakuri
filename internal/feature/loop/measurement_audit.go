package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// auditMeasurements writes one measurement row for each measurement an action
// in this pass returned, whichever capability returned it and whether or not
// it settles a criterion (Phase 41, decision 6, 'Measurements are audited').
//
// The row names the capability, the parameters the action ran with, the
// metric, value, unit, source and trust, and the commit the verify step
// judges: the snapshot of the environment that ran the action, taken now,
// after the work. A blind environment has no SHA, and the row says so with an
// empty commit rather than a guessed one (AGENTS.md rule 10). Only names and
// numbers are written, never a result's output.
func (s *serviceImpl) auditMeasurements(ctx context.Context, sc *stepContext, outcomes []actionOutcome) {
	if s.store == nil {
		return
	}
	commits := make(map[string]string)
	n := 0
	for _, o := range outcomes {
		if len(o.Result.Measurements) == 0 {
			continue
		}
		commit, ok := commits[o.EnvID]
		if !ok {
			commit = snapshotSHA(ctx, sc.envs, o.EnvID)
			commits[o.EnvID] = commit
		}
		for _, m := range o.Result.Measurements {
			s.recordMeasurement(ctx, sc, o, m, commit, n)
			n++
		}
	}
}

// snapshotSHA is the current commit of the environment envID names among the
// ones this loop built, or "" when none matches or it cannot see.
func snapshotSHA(ctx context.Context, envs []environment.Environment, envID string) string {
	for _, env := range envs {
		if string(env.ID()) != envID {
			continue
		}
		snap, err := env.Snapshot(ctx)
		if err != nil {
			return ""
		}
		return snap.SHA
	}
	return ""
}

// recordMeasurement writes the audit row for one measurement. n keeps the IDs
// of rows written in the same instant apart.
func (s *serviceImpl) recordMeasurement(ctx context.Context, sc *stepContext, o actionOutcome, m environment.Measurement, commit string, n int) {
	payload := map[string]any{
		"loop_id":  sc.loopID,
		"verifier": o.CapabilityID,
		"params":   o.Params,
		"metric":   m.Name,
		"value":    m.Value,
		"unit":     m.Unit,
		"source":   m.Source,
		"trust":    string(m.Trust),
		"commit":   commit,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		// A NaN or infinite value has no JSON form. The row is still written,
		// with no value, so the measurement is not lost from the audit.
		delete(payload, "value")
		payload["value_error"] = "not a finite number"
		payloadJSON, _ = json.Marshal(payload)
	}
	now := time.Now().UTC()
	if err := s.store.SaveToolEvent(ctx, storage.ToolEvent{
		ID:          fmt.Sprintf("measurement-%d-%d", now.UnixNano(), n),
		ObjectiveID: string(sc.obj.ID),
		AgentID:     string(sc.agentDef.ID),
		Kind:        storage.ToolEventMeasurement,
		Capability:  o.CapabilityID,
		Adapter:     o.EnvID,
		TemplateID:  sc.obj.TemplateID,
		Success:     !o.Errored,
		PayloadJSON: string(payloadJSON),
		CreatedAt:   now,
	}); err != nil {
		slog.Warn("could not write the measurement audit row", "objective", string(sc.obj.ID),
			"capability", o.CapabilityID, "metric", m.Name, "error", err)
	}
}
