package objective

import (
	"context"
	"errors"

	"github.com/bsenel/karakuri/internal/core/objective"
)

// SetTarget is a person changing a measured criterion's target through the API
// (Phase 41 decision 6 'Targets belong to their author' and step 8's
// acceptance: a person's target change restarts the sustain window). Not
// implemented yet.
func (s *Service) SetTarget(_ context.Context, _ objective.ObjectiveID, _ string, _ float64, _ string) (objective.Objective, error) {
	return objective.Objective{}, errors.New("set target: not implemented")
}
