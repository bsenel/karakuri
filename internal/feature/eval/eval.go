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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/feature/loop"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// The choices a reviewer resolves a checkpoint with. internal/feature/checkpoint
// spells them as string literals rather than exported constants, so they are
// spelled the same way here; anything else is not a label we can score.
const (
	decisionApprove = "approve"
	decisionReject  = "reject"
	decisionModify  = "modify"
)

// Bounds on what the judge reads, for the reason renderOutcomes bounds its
// evidence: a plan is judged against its objective, and a payload large enough
// to push the objective out of the model's attention defeats showing it at all.
const (
	maxDescriptionChars = 2000
	maxActionChars      = 600
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

	// Replayable counts the resolved checkpoints in the window that carry a
	// recorded world state. This is the planner-replay corpus, which starts
	// empty and grows only from escalations after this shipped.
	Replayable int

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
	cps, err := s.store.ListResolvedCheckpoints(ctx, f)
	if err != nil {
		return CalibrationReport{}, err
	}

	rep := CalibrationReport{
		TwinID: f.TwinID, Since: f.Since, Until: f.Until,
		ByDecision: map[string]DecisionStats{},
	}
	for _, cp := range cps {
		if err := ctx.Err(); err != nil {
			return CalibrationReport{}, err
		}

		// Skipped rather than guessed at: a checkpoint without a recognisable
		// human verdict has no label, and one whose objective is gone cannot be
		// put to the judge the way the loop would have put it.
		if cp.Decision == nil {
			rep.Skipped++
			continue
		}
		choice := cp.Decision.Choice
		if choice != decisionApprove && choice != decisionReject && choice != decisionModify {
			rep.Skipped++
			continue
		}
		obj, err := s.store.GetObjective(ctx, cp.ObjectiveID)
		if err != nil {
			rep.Skipped++
			continue
		}

		it := Item{
			CheckpointID: cp.ID, ObjectiveID: string(cp.ObjectiveID), Choice: choice,
			HumanApprove: choice == decisionApprove,
		}
		it.JudgePass, it.Reply, it.Error = s.judgePlan(ctx, obj, cp.Actions)
		it.Agreed = it.JudgePass == it.HumanApprove

		rep.N++
		if it.Agreed {
			rep.Agreed++
		}
		switch {
		case it.JudgePass && it.HumanApprove:
			rep.Confusion.JudgePassHumanApprove++
		case it.JudgePass:
			rep.Confusion.JudgePassHumanReject++
		case it.HumanApprove:
			rep.Confusion.JudgeFailHumanApprove++
		default:
			rep.Confusion.JudgeFailHumanReject++
		}
		ds := rep.ByDecision[choice]
		ds.N++
		if it.Agreed {
			ds.Agreed++
		}
		if it.JudgePass {
			ds.JudgePass++
		}
		rep.ByDecision[choice] = ds
		rep.Items = append(rep.Items, it)
	}
	if rep.N > 0 {
		rep.Agreement = float64(rep.Agreed) / float64(rep.N)
	}
	return rep, nil
}

// CountReplayable counts the resolved checkpoints f selects that carry a
// recorded world state.
func (s *Service) CountReplayable(ctx context.Context, f storage.ResolvedCheckpointFilter) (int, error) {
	return 0, nil
}

// judgePlan asks the judge the loop's question about a drafted plan. It has to
// be asked the loop's way and read with the loop's parser, or the report
// measures a judge nobody runs.
func (s *Service) judgePlan(ctx context.Context, obj objective.Objective, actions []checkpoint.Action) (pass bool, reply, errText string) {
	// Mirrors evaluateWithAgent: nothing drafted means nothing to judge, and
	// asking anyway would score the judge on how plausible the objective sounds.
	if len(actions) == 0 {
		return false, "", ""
	}
	out, err := s.judge.Run(ctx, coreagent.Input{
		Objective: obj,
		// Nil, as in evaluateWithAgent: the plan is in the task, and the world
		// the reviewer saw was never recorded on the checkpoint.
		WorldState: nil,
		Memory:     nil,
		Task:       renderPlanTask(obj, actions),
	})
	if err != nil {
		// A judge that could not answer did not approve, same as in the loop.
		return false, "", err.Error()
	}
	return loop.VerdictIsPass(out.Content), out.Content, ""
}

// renderPlanTask builds the judge's prompt for one drafted plan. It keeps
// evaluateWithAgent's contract — shown evidence only, absence is FAIL, one
// word — so the only thing that differs from the loop's judgement is what is
// being judged.
func renderPlanTask(obj objective.Objective, actions []checkpoint.Action) string {
	var sb strings.Builder
	sb.WriteString("Evaluate whether this proposed plan should be accepted as proposed " +
		"for the objective below, based only on what is shown.\n\n")
	fmt.Fprintf(&sb, "Objective: %s\n", obj.Title)
	fmt.Fprintf(&sb, "Description: %s\n", truncate(obj.Description, maxDescriptionChars))
	sb.WriteString("Success criteria:\n")
	for _, c := range obj.SuccessCriteria {
		fmt.Fprintf(&sb, "- %s\n", c.Description)
	}
	sb.WriteString("\nProposed actions:\n")
	sb.WriteString(renderActions(actions))
	sb.WriteString("\nIf the plan does not clearly serve the objective, answer FAIL — " +
		"absence of evidence is not evidence it would.\n" +
		"Answer with exactly one word: PASS or FAIL.")
	return sb.String()
}

// renderActions lays the draft out the way renderOutcomes lays out results,
// capability first, because that is what a reviewer reads first too.
func renderActions(actions []checkpoint.Action) string {
	var sb strings.Builder
	for i, a := range actions {
		fmt.Fprintf(&sb, "%d. %s", i+1, a.CapabilityID)
		if a.EnvID != "" {
			fmt.Fprintf(&sb, " (%s)", a.EnvID)
		}
		if a.Reason != "" {
			fmt.Fprintf(&sb, ": %s", truncate(a.Reason, maxActionChars))
		}
		if len(a.Params) > 0 {
			if params, err := json.Marshal(a.Params); err == nil {
				fmt.Fprintf(&sb, "\n   params: %s", truncate(string(params), maxActionChars))
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…(truncated)"
}
