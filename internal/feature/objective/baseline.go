package objective

import (
	"context"
	"errors"

	"github.com/bsenel/karakuri/internal/core/objective"
)

// ErrCriterionNotFound is a criterion ID the objective does not have.
var ErrCriterionNotFound = errors.New("criterion not found")

// errSetBaselineNotImplemented is the scaffolding Phase 41 step 7 part D2's
// tests are written against; the next action replaces it.
var errSetBaselineNotImplemented = errors.New("set baseline: not implemented")

// SetBaseline is a person setting a measured criterion's baseline through the
// API (Phase 41 decision 3 'Baselines' and decision 6 'Targets belong to their
// author'). It writes value as the criterion's BaselineValue, replacing a
// frozen one, persists the objective, and audits the change in one baseline
// row naming setBy, and returns the updated objective.
func (s *Service) SetBaseline(ctx context.Context, id objective.ObjectiveID, criterionID string, value float64, setBy string) (objective.Objective, error) {
	return objective.Objective{}, errSetBaselineNotImplemented
}
