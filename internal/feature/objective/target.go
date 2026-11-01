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

// SetTarget is a person changing a measured criterion's target through the API
// (Phase 41 decision 6 'Targets belong to their author' and step 8's
// acceptance: a person's target change restarts the sustain window). It writes
// value as the criterion's Target, sets SustainSince to the time of the change
// (which reconcile reads), persists the objective, audits the change in one
// target row naming setBy, and returns the updated objective. Sustain and
// baseline are left as they were.
func (s *Service) SetTarget(ctx context.Context, id objective.ObjectiveID, criterionID string, value float64, setBy string) (objective.Objective, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return objective.Objective{}, &ValidationError{Field: "value", Err: errors.New("a target is a finite number")}
	}
	o, i, err := s.criterionOf(ctx, id, criterionID)
	if err != nil {
		return objective.Objective{}, err
	}
	c := o.SuccessCriteria[i]
	if c.Kind != objective.KindMeasured {
		return objective.Objective{}, &ValidationError{Field: "kind", Err: fmt.Errorf("only a measured criterion has a target, and %s is %s", c.ID, c.Kind)}
	}

	previous := c.Target
	now := time.Now().UTC()
	o.SuccessCriteria[i].Target = &value
	// A person's change restarts the sustain window, the same way SetBaseline
	// restarts it.
	o.SuccessCriteria[i].SustainSince = &now
	o.UpdatedAt = now
	if err := s.store.SaveObjective(ctx, o); err != nil {
		return objective.Objective{}, fmt.Errorf("save objective: %w", err)
	}
	if err := s.auditTarget(ctx, o, c, previous, value, setBy, now); err != nil {
		// The target is saved; the caller is told the change went unaudited
		// rather than told it succeeded.
		return objective.Objective{}, fmt.Errorf("target saved but not audited: %w", err)
	}
	return o, nil
}

// auditTarget writes the one target row a person's change leaves: the
// criterion, the target it replaced (null for none), the new one, the unit,
// the person, and when the change restarted the sustain window.
func (s *Service) auditTarget(ctx context.Context, o objective.Objective, c objective.Criterion, previous *float64, value float64, setBy string, sustainSince time.Time) error {
	payloadJSON, err := json.Marshal(map[string]any{
		"criterion":     c.ID,
		"verifier":      string(c.Verifier),
		"metric":        c.Metric,
		"unit":          c.Unit,
		"previous":      previous,
		"value":         value,
		"set_by":        setBy,
		"sustain_since": sustainSince,
	})
	if err != nil {
		return err
	}
	suffix, _ := newID()
	now := time.Now().UTC()
	return s.store.SaveToolEvent(ctx, storage.ToolEvent{
		ID:          fmt.Sprintf("audit-%d-%s", now.UnixNano(), suffix),
		ObjectiveID: string(o.ID),
		Kind:        storage.ToolEventTarget,
		Capability:  string(c.Verifier),
		TemplateID:  o.TemplateID,
		Success:     true,
		PayloadJSON: string(payloadJSON),
		CreatedAt:   now,
	})
}

// criterionOf loads the objective and finds the index of criterionID in its
// success criteria, or ErrCriterionNotFound.
func (s *Service) criterionOf(ctx context.Context, id objective.ObjectiveID, criterionID string) (objective.Objective, int, error) {
	o, err := s.store.GetObjective(ctx, id)
	if err != nil {
		return objective.Objective{}, -1, err
	}
	for i := range o.SuccessCriteria {
		if o.SuccessCriteria[i].ID == criterionID {
			return o, i, nil
		}
	}
	return objective.Objective{}, -1, fmt.Errorf("%w: %s", ErrCriterionNotFound, criterionID)
}
