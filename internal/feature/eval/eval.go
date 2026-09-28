// Package eval calibrates the judge against human checkpoint verdicts.
//
// It is read-only: it lists checkpoints a human already resolved, asks the
// judge the same PASS/FAIL question about each drafted plan, and reports how
// often the two agree. Nothing is written back. A human "approve" is the
// positive label; "reject" and "modify" are both negative, because a plan the
// reviewer had to change is not one they would have let run as drafted.
package eval

import (
	"context"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Store is the slice of storage calibration reads.
type Store interface {
	ListResolvedCheckpoints(ctx context.Context, f storage.ResolvedCheckpointFilter) ([]checkpoint.Checkpoint, error)
	GetObjective(ctx context.Context, id objective.ObjectiveID) (objective.Objective, error)
}

// Service runs judge calibration.
type Service struct {
	store Store
	judge coreagent.Agent
}

func NewService(store Store, judge coreagent.Agent) *Service {
	return &Service{store: store, judge: judge}
}

// CalibrationReport says how often the judge agreed with the humans who
// resolved the checkpoints in the window.
type CalibrationReport struct {
	TwinID       string
	Since, Until time.Time

	N, Agreed, Skipped int
	Agreement          float64

	Confusion  Confusion
	ByDecision map[string]DecisionStats
	Items      []Item
}

// Confusion crosses the judge's verdict with the human's.
type Confusion struct {
	JudgePassHumanApprove, JudgePassHumanReject int
	JudgeFailHumanApprove, JudgeFailHumanReject int
}

// DecisionStats breaks agreement down by the human's choice.
type DecisionStats struct {
	N, Agreed, JudgePass int
}

// Item is one scored checkpoint.
type Item struct {
	CheckpointID, ObjectiveID, Choice string
	HumanApprove, JudgePass, Agreed   bool
	Reply, Error                      string
}

// Calibrate scores the judge against every labelled checkpoint f selects.
func (s *Service) Calibrate(ctx context.Context, f storage.ResolvedCheckpointFilter) (CalibrationReport, error) {
	return CalibrationReport{}, nil
}

// renderPlanTask builds the judge's prompt for one drafted plan.
func renderPlanTask(obj objective.Objective, actions []checkpoint.Action) string {
	return ""
}
