package objective

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// ErrCriterionNotFound is a criterion ID the objective does not have.
var ErrCriterionNotFound = errors.New("criterion not found")

// SetBaseline is a person setting a measured criterion's baseline through the
// API (Phase 41 decision 3 'Baselines' and decision 6 'Targets belong to their
// author'). It writes value as the criterion's BaselineValue, replacing a
// frozen one, persists the objective, and audits the change in one baseline
// row naming setBy, and returns the updated objective.
//
// The row carries set_by and no loop_id, so it reads as a person's and not as
// the loop's at_start measurement. at_start reads any baseline row on the
// criterion as its one attempt, so it never measures over a person's value.
func (s *Service) SetBaseline(ctx context.Context, id objective.ObjectiveID, criterionID string, value float64, setBy string) (objective.Objective, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return objective.Objective{}, &ValidationError{Field: "value", Err: errors.New("a baseline is a finite number")}
	}
	o, err := s.store.GetObjective(ctx, id)
	if err != nil {
		return objective.Objective{}, err
	}
	i := -1
	for j := range o.SuccessCriteria {
		if o.SuccessCriteria[j].ID == criterionID {
			i = j
			break
		}
	}
	if i < 0 {
		return objective.Objective{}, fmt.Errorf("%w: %s", ErrCriterionNotFound, criterionID)
	}
	c := o.SuccessCriteria[i]
	if c.Kind != objective.KindMeasured {
		return objective.Objective{}, &ValidationError{Field: "kind", Err: fmt.Errorf("only a measured criterion has a baseline, and %s is %s", c.ID, c.Kind)}
	}
	if c.Change == "" {
		return objective.Objective{}, &ValidationError{Field: "change", Err: fmt.Errorf("criterion %s has no change, so no baseline is read", c.ID)}
	}

	previous := c.BaselineValue
	o.SuccessCriteria[i].BaselineValue = &value
	o.UpdatedAt = time.Now().UTC()
	if err := s.store.SaveObjective(ctx, o); err != nil {
		return objective.Objective{}, fmt.Errorf("save objective: %w", err)
	}
	if err := s.auditBaseline(ctx, o, c, previous, value, setBy); err != nil {
		// The value is saved; the caller is told the change went unaudited
		// rather than told it succeeded.
		return objective.Objective{}, fmt.Errorf("baseline saved but not audited: %w", err)
	}
	return o, nil
}

// auditBaseline writes the one baseline row a person's set leaves: the
// criterion, the value it replaced (null for none), the new value, the unit
// and the person.
func (s *Service) auditBaseline(ctx context.Context, o objective.Objective, c objective.Criterion, previous *float64, value float64, setBy string) error {
	payloadJSON, err := json.Marshal(map[string]any{
		"criterion": c.ID,
		"verifier":  string(c.Verifier),
		"metric":    c.Metric,
		"unit":      c.Unit,
		"previous":  previous,
		"value":     value,
		"set_by":    setBy,
	})
	if err != nil {
		return err
	}
	suffix, _ := newID()
	now := time.Now().UTC()
	return s.store.SaveToolEvent(ctx, storage.ToolEvent{
		ID:          fmt.Sprintf("audit-%d-%s", now.UnixNano(), suffix),
		ObjectiveID: string(o.ID),
		Kind:        storage.ToolEventBaseline,
		Capability:  string(c.Verifier),
		TemplateID:  o.TemplateID,
		Success:     true,
		PayloadJSON: string(payloadJSON),
		CreatedAt:   now,
	})
}
