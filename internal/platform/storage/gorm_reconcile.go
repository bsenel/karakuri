package storage

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	"github.com/bsenel/karakuri/internal/platform/db/schema"
)

// ── Reconcile state (Phase 20) ────────────────────────────────────────────

func (s *GORMStorage) SaveReconcileState(ctx context.Context, st reconcile.State) error {
	convJ, _ := json.Marshal(st.Converged)
	return s.db.WithContext(ctx).Save(&schema.ReconcileStateModel{
		ObjectiveID:  string(st.ObjectiveID),
		TwinID:       st.TwinID,
		Phase:        string(st.Phase),
		Paused:       st.Paused,
		PausedReason: st.PausedReason,

		NextDueAt:       st.NextDueAt,
		NextSenseAt:     st.NextSenseAt,
		NextReconcileAt: st.NextReconcileAt,

		ConvergedJSON:   string(convJ),
		LastConvergedAt: st.LastConvergedAt,

		LastRunAt:        st.LastRunAt,
		LastReconciledAt: st.LastReconciledAt,
		LastTrigger:      string(st.LastTrigger),
		LastOutcomeID:    st.LastOutcomeID,
		LastError:        st.LastError,
		ActiveLoopID:     st.ActiveLoopID,

		CriteriaMet:         st.CriteriaMet,
		ScoreStreak:         st.ScoreStreak,
		ConsecutiveFailures: st.ConsecutiveFailures,

		Autonomy:  string(st.Autonomy),
		CleanRuns: st.CleanRuns,

		Holder:     st.Holder,
		LeaseUntil: st.LeaseUntil,
	}).Error
}

func (s *GORMStorage) GetReconcileState(ctx context.Context, objectiveID objective.ObjectiveID) (reconcile.State, error) {
	var m schema.ReconcileStateModel
	if err := s.db.WithContext(ctx).First(&m, "objective_id = ?", string(objectiveID)).Error; err != nil {
		// "No such row" and "the database is unreachable" are opposite
		// answers, and callers decide whether to create a fresh state from
		// this error. Translating only the first keeps a transient outage
		// from reading as an objective that has never been seen before.
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return reconcile.State{}, coreerrors.ErrNotFound
		}
		return reconcile.State{}, err
	}
	return reconcileStateFromModel(m), nil
}

func (s *GORMStorage) DeleteReconcileState(ctx context.Context, objectiveID objective.ObjectiveID) error {
	return s.db.WithContext(ctx).
		Delete(&schema.ReconcileStateModel{}, "objective_id = ?", string(objectiveID)).Error
}

func (s *GORMStorage) ListReconcileStateIDs(ctx context.Context) ([]objective.ObjectiveID, error) {
	var ids []string
	if err := s.db.WithContext(ctx).
		Model(&schema.ReconcileStateModel{}).
		Pluck("objective_id", &ids).Error; err != nil {
		return nil, err
	}
	out := make([]objective.ObjectiveID, 0, len(ids))
	for _, id := range ids {
		out = append(out, objective.ObjectiveID(id))
	}
	return out, nil
}

// ListDueReconcileStates is the supervisor's tick query.
//
// The lease predicate is part of the WHERE rather than a filter applied after
// reading, so a replica does not spend its tick budget pulling back rows it
// cannot have. Rows this replica already holds are included: a claim it took
// and did not finish is exactly the work it should pick up again.
//
// Ordered by due time so that when the limit bites, the most overdue objectives
// are the ones that run. Without the ordering a backlog would be worked in
// primary-key order, and one objective could sit unattended indefinitely while
// alphabetically earlier neighbours were serviced every tick.
func (s *GORMStorage) ListDueReconcileStates(ctx context.Context, holder string, now time.Time, limit int) ([]reconcile.State, error) {
	if limit <= 0 {
		limit = 100
	}
	var models []schema.ReconcileStateModel
	err := s.db.WithContext(ctx).
		Where("paused = ?", false).
		Where("next_due_at IS NOT NULL AND next_due_at <= ?", now.UTC()).
		Where("lease_until IS NULL OR lease_until <= ? OR holder = ?", now.UTC(), holder).
		Order("next_due_at ASC").
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	out := make([]reconcile.State, len(models))
	for i, m := range models {
		out[i] = reconcileStateFromModel(m)
	}
	return out, nil
}

// ClaimReconcileState is the whole of Karakuri's distributed coordination.
//
// One conditional UPDATE, one RowsAffected check. The database already
// serializes writes to a row, so whichever replica's statement lands first
// moves lease_until into the future and every other replica's WHERE clause
// stops matching. There is no lock to release and nothing to clean up after a
// crash: an expired lease is indistinguishable from an absent one.
//
// Re-claiming a lease this holder already has succeeds and extends it, which
// is what makes the resume-after-restart path work without a special case.
func (s *GORMStorage) ClaimReconcileState(ctx context.Context, objectiveID objective.ObjectiveID, holder string, now, until time.Time) (bool, error) {
	res := s.db.WithContext(ctx).
		Model(&schema.ReconcileStateModel{}).
		Where("objective_id = ?", string(objectiveID)).
		Where("lease_until IS NULL OR lease_until <= ? OR holder = ?", now.UTC(), holder).
		Updates(map[string]any{
			"holder":      holder,
			"lease_until": until.UTC(),
			"updated_at":  now.UTC(),
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// RenewReconcileLease extends a claim mid-run. It requires the caller to still
// be the holder, and deliberately does not check expiry: a holder whose lease
// lapsed for a moment but whom nobody displaced should keep going rather than
// abandon a reconcile half-done. What it will not do is take a lease back from
// a replica that has already claimed it.
func (s *GORMStorage) RenewReconcileLease(ctx context.Context, objectiveID objective.ObjectiveID, holder string, now, until time.Time) (bool, error) {
	res := s.db.WithContext(ctx).
		Model(&schema.ReconcileStateModel{}).
		Where("objective_id = ? AND holder = ?", string(objectiveID), holder).
		Updates(map[string]any{
			"lease_until": until.UTC(),
			"updated_at":  now.UTC(),
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// ReleaseReconcileLease drops a claim on the ordinary path. Scoped to the
// holder so a late release from a replica that was already displaced cannot
// unlock work somebody else is now doing.
func (s *GORMStorage) ReleaseReconcileLease(ctx context.Context, objectiveID objective.ObjectiveID, holder string) error {
	return s.db.WithContext(ctx).
		Model(&schema.ReconcileStateModel{}).
		Where("objective_id = ? AND holder = ?", string(objectiveID), holder).
		Updates(map[string]any{"holder": "", "lease_until": nil}).Error
}

func (s *GORMStorage) SaveReconcileOutcome(ctx context.Context, o reconcile.Outcome) error {
	driftJ, _ := json.Marshal(o.Drift)
	return s.db.WithContext(ctx).Save(&schema.ReconcileOutcomeModel{
		ID:           o.ID,
		ObjectiveID:  string(o.ObjectiveID),
		TwinID:       o.TwinID,
		Trigger:      string(o.Trigger),
		LoopID:       o.LoopID,
		DriftJSON:    string(driftJ),
		Autonomy:     string(o.Autonomy),
		CriteriaMet:  o.CriteriaMet,
		Converged:    o.Converged,
		Escalated:    o.Escalated,
		CheckpointID: o.CheckpointID,
		Error:        o.Error,
		StartedAt:    o.StartedAt,
		EndedAt:      o.EndedAt,
	}).Error
}

func (s *GORMStorage) ListReconcileOutcomes(ctx context.Context, objectiveID objective.ObjectiveID, limit int) ([]reconcile.Outcome, error) {
	if limit <= 0 {
		limit = 50
	}
	var models []schema.ReconcileOutcomeModel
	err := s.db.WithContext(ctx).
		Where("objective_id = ?", string(objectiveID)).
		Order("started_at DESC").
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	out := make([]reconcile.Outcome, len(models))
	for i, m := range models {
		var drift reconcile.Drift
		_ = json.Unmarshal([]byte(m.DriftJSON), &drift)
		out[i] = reconcile.Outcome{
			ID:           m.ID,
			ObjectiveID:  objective.ObjectiveID(m.ObjectiveID),
			TwinID:       m.TwinID,
			Trigger:      reconcile.Trigger(m.Trigger),
			LoopID:       m.LoopID,
			Drift:        drift,
			Autonomy:     objective.AutonomyLevel(m.Autonomy),
			CriteriaMet:  m.CriteriaMet,
			Converged:    m.Converged,
			Escalated:    m.Escalated,
			CheckpointID: m.CheckpointID,
			Error:        m.Error,
			StartedAt:    m.StartedAt,
			EndedAt:      m.EndedAt,
		}
	}
	return out, nil
}

func reconcileStateFromModel(m schema.ReconcileStateModel) reconcile.State {
	var converged reconcile.Fingerprint
	if m.ConvergedJSON != "" {
		_ = json.Unmarshal([]byte(m.ConvergedJSON), &converged)
	}
	return reconcile.State{
		ObjectiveID:  objective.ObjectiveID(m.ObjectiveID),
		TwinID:       m.TwinID,
		Phase:        reconcile.Phase(m.Phase),
		Paused:       m.Paused,
		PausedReason: m.PausedReason,

		NextDueAt:       m.NextDueAt,
		NextSenseAt:     m.NextSenseAt,
		NextReconcileAt: m.NextReconcileAt,

		Converged:       converged,
		LastConvergedAt: m.LastConvergedAt,

		LastRunAt:        m.LastRunAt,
		LastReconciledAt: m.LastReconciledAt,
		LastTrigger:      reconcile.Trigger(m.LastTrigger),
		LastOutcomeID:    m.LastOutcomeID,
		LastError:        m.LastError,
		ActiveLoopID:     m.ActiveLoopID,

		CriteriaMet:         m.CriteriaMet,
		ScoreStreak:         m.ScoreStreak,
		ConsecutiveFailures: m.ConsecutiveFailures,

		Autonomy:  objective.AutonomyLevel(m.Autonomy),
		CleanRuns: m.CleanRuns,

		Holder:     m.Holder,
		LeaseUntil: m.LeaseUntil,

		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}
