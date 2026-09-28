package eval

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/feature/loop"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// The choices a reviewer resolves a checkpoint with. They are string
// literals in internal/feature/checkpoint/service.go and the loop runner, not
// exported constants, so they are spelled the same way here.
const (
	choiceApprove = "approve"
	choiceReject  = "reject"
	choiceModify  = "modify"
)

type fakeStore struct {
	checkpoints []checkpoint.Checkpoint
	objectives  map[objective.ObjectiveID]objective.Objective
	gotFilter   *storage.ResolvedCheckpointFilter
}

func (f *fakeStore) ListResolvedCheckpoints(_ context.Context, filter storage.ResolvedCheckpointFilter) ([]checkpoint.Checkpoint, error) {
	f.gotFilter = &filter
	return f.checkpoints, nil
}

func (f *fakeStore) GetObjective(_ context.Context, id objective.ObjectiveID) (objective.Objective, error) {
	obj, ok := f.objectives[id]
	if !ok {
		return objective.Objective{}, errors.New("objective not found")
	}
	return obj, nil
}

// fakeJudge answers by objective title: the first scripted title found in the
// task wins, and input.Objective is the fallback.
type fakeJudge struct {
	replies map[string]string
	errs    map[string]error

	mu    sync.Mutex
	calls int
	tasks []string
}

func (j *fakeJudge) Run(_ context.Context, input coreagent.Input) (coreagent.Output, error) {
	j.mu.Lock()
	j.calls++
	j.tasks = append(j.tasks, input.Task)
	j.mu.Unlock()

	title := ""
	for t := range j.replies {
		if strings.Contains(input.Task, t) {
			title = t
			break
		}
	}
	if title == "" {
		for t := range j.errs {
			if strings.Contains(input.Task, t) {
				title = t
				break
			}
		}
	}
	if title == "" {
		if obj, ok := input.Objective.(objective.Objective); ok {
			title = obj.Title
		}
	}
	if err, ok := j.errs[title]; ok {
		return coreagent.Output{}, err
	}
	return coreagent.Output{Content: j.replies[title]}, nil
}

func (j *fakeJudge) Stream(context.Context, coreagent.Input) (<-chan coreagent.OutputChunk, error) {
	return nil, errors.New("fakeJudge does not stream")
}

func (j *fakeJudge) callCount() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.calls
}

// fixture builds a store and a judge from cases. Each case gets its own
// objective, whose title is how the judge picks the scripted reply.
type fixtureCase struct {
	id      string
	choice  string
	reply   string
	err     error
	actions []checkpoint.Action // nil means one default action
}

func fixture(cases ...fixtureCase) (*fakeStore, *fakeJudge) {
	store := &fakeStore{objectives: map[objective.ObjectiveID]objective.Objective{}}
	judge := &fakeJudge{replies: map[string]string{}, errs: map[string]error{}}
	for _, c := range cases {
		objID := objective.ObjectiveID("obj-" + c.id)
		title := "Objective title <" + c.id + ">"
		store.objectives[objID] = objective.Objective{
			ID:    objID,
			Title: title,
			SuccessCriteria: []objective.Criterion{
				{ID: "c1", Description: "criterion for " + c.id},
			},
		}
		actions := c.actions
		if actions == nil {
			actions = []checkpoint.Action{{CapabilityID: "vcs.open_pr", Reason: "draft for " + c.id}}
		}
		store.checkpoints = append(store.checkpoints, checkpoint.Checkpoint{
			ID: c.id, ObjectiveID: objID, TwinID: "twin-a",
			Actions:  actions,
			Status:   checkpoint.StatusResolved,
			Decision: &checkpoint.Decision{Choice: c.choice},
		})
		if c.err != nil {
			judge.errs[title] = c.err
		} else {
			judge.replies[title] = c.reply
		}
	}
	return store, judge
}

func calibrate(t *testing.T, store *fakeStore, judge *fakeJudge, f storage.ResolvedCheckpointFilter) CalibrationReport {
	t.Helper()
	rep, err := NewService(store, judge).Calibrate(context.Background(), f)
	if err != nil {
		t.Fatalf("Calibrate: %v", err)
	}
	return rep
}

func itemFor(t *testing.T, rep CalibrationReport, id string) Item {
	t.Helper()
	for _, it := range rep.Items {
		if it.CheckpointID == id {
			return it
		}
	}
	t.Fatalf("no item for checkpoint %q in %+v", id, rep.Items)
	return Item{}
}

func TestCalibrate_AgreementArithmetic(t *testing.T) {
	store, judge := fixture(
		fixtureCase{id: "ap", choice: choiceApprove, reply: "PASS"},
		fixtureCase{id: "af", choice: choiceApprove, reply: "FAIL"},
		fixtureCase{id: "rf", choice: choiceReject, reply: "FAIL"},
		fixtureCase{id: "mp", choice: choiceModify, reply: "PASS"},
	)
	rep := calibrate(t, store, judge, storage.ResolvedCheckpointFilter{})

	if rep.N != 4 || rep.Agreed != 2 {
		t.Fatalf("N=%d Agreed=%d, want 4 and 2", rep.N, rep.Agreed)
	}
	if rep.Agreement != 0.5 {
		t.Errorf("Agreement = %v, want 0.5", rep.Agreement)
	}
	want := Confusion{
		JudgePassHumanApprove: 1, JudgePassHumanReject: 1,
		JudgeFailHumanApprove: 1, JudgeFailHumanReject: 1,
	}
	if rep.Confusion != want {
		t.Errorf("Confusion = %+v, want %+v", rep.Confusion, want)
	}
}

func TestCalibrate_Labelling(t *testing.T) {
	store, judge := fixture(
		fixtureCase{id: "ap", choice: choiceApprove, reply: "PASS"},
		fixtureCase{id: "af", choice: choiceApprove, reply: "FAIL"},
		fixtureCase{id: "rf", choice: choiceReject, reply: "FAIL"},
		fixtureCase{id: "rp", choice: choiceReject, reply: "PASS"},
		fixtureCase{id: "mp", choice: choiceModify, reply: "PASS"},
	)
	rep := calibrate(t, store, judge, storage.ResolvedCheckpointFilter{})

	wantApprove := map[string]bool{"ap": true, "af": true, "rf": false, "rp": false, "mp": false}
	for id, want := range wantApprove {
		it := itemFor(t, rep, id)
		if it.HumanApprove != want {
			t.Errorf("%s (%s): HumanApprove = %v, want %v", id, it.Choice, it.HumanApprove, want)
		}
	}

	wantStats := map[string]DecisionStats{
		choiceApprove: {N: 2, Agreed: 1, JudgePass: 1},
		choiceReject:  {N: 2, Agreed: 1, JudgePass: 1},
		choiceModify:  {N: 1, Agreed: 0, JudgePass: 1},
	}
	for choice, want := range wantStats {
		if got := rep.ByDecision[choice]; got != want {
			t.Errorf("ByDecision[%q] = %+v, want %+v", choice, got, want)
		}
	}
}

// A reply nobody can parse must not count as the judge approving.
func TestCalibrate_UnparseableReplyIsFail(t *testing.T) {
	store, judge := fixture(
		fixtureCase{id: "a", choice: choiceApprove, reply: "maybe?"},
		fixtureCase{id: "r", choice: choiceReject, reply: "maybe?"},
	)
	rep := calibrate(t, store, judge, storage.ResolvedCheckpointFilter{})

	a := itemFor(t, rep, "a")
	if a.JudgePass || a.Agreed {
		t.Errorf("approve+maybe: JudgePass=%v Agreed=%v, want false false", a.JudgePass, a.Agreed)
	}
	r := itemFor(t, rep, "r")
	if r.JudgePass || !r.Agreed {
		t.Errorf("reject+maybe: JudgePass=%v Agreed=%v, want false true", r.JudgePass, r.Agreed)
	}
}

// Calibration measures the judge the loop actually uses, so it must read
// replies with the loop's parser rather than a lookalike.
func TestCalibrate_UsesProductionParser(t *testing.T) {
	t.Run("negated pass", func(t *testing.T) {
		store, judge := fixture(fixtureCase{id: "a", choice: choiceApprove, reply: "this does not pass"})
		rep := calibrate(t, store, judge, storage.ResolvedCheckpointFilter{})
		it := itemFor(t, rep, "a")
		if it.JudgePass || it.Agreed {
			t.Errorf("JudgePass=%v Agreed=%v, want false false", it.JudgePass, it.Agreed)
		}
	})

	replies := []string{
		"PASS", "Pass.", "FAIL", "this does not pass",
		"the criterion is not met", "", "yes it passes",
	}
	for _, reply := range replies {
		t.Run("reply "+reply, func(t *testing.T) {
			store, judge := fixture(fixtureCase{id: "a", choice: choiceApprove, reply: reply})
			rep := calibrate(t, store, judge, storage.ResolvedCheckpointFilter{})
			it := itemFor(t, rep, "a")
			if want := loop.VerdictIsPass(reply); it.JudgePass != want {
				t.Errorf("reply %q: JudgePass = %v, loop.VerdictIsPass = %v", reply, it.JudgePass, want)
			}
		})
	}
}

func TestCalibrate_KeepsReplyText(t *testing.T) {
	const reply = "PASS — the draft opens the PR the objective asks for."
	store, judge := fixture(fixtureCase{id: "a", choice: choiceApprove, reply: reply})
	rep := calibrate(t, store, judge, storage.ResolvedCheckpointFilter{})
	if got := itemFor(t, rep, "a").Reply; got != reply {
		t.Errorf("Reply = %q, want %q", got, reply)
	}
}

func TestCalibrate_AgentErrorIsFail(t *testing.T) {
	store, judge := fixture(fixtureCase{id: "a", choice: choiceApprove, err: errors.New("provider down")})
	rep := calibrate(t, store, judge, storage.ResolvedCheckpointFilter{})
	it := itemFor(t, rep, "a")
	if it.JudgePass {
		t.Error("JudgePass = true on agent error, want false")
	}
	if it.Error == "" {
		t.Error("Error is empty on agent error")
	}
}

// Mirrors evaluateWithAgent: nothing drafted means nothing to judge, and
// asking anyway invites a verdict on plausibility.
func TestCalibrate_NoActionsIsFailWithoutAsking(t *testing.T) {
	store, judge := fixture(fixtureCase{
		id: "a", choice: choiceApprove, reply: "PASS",
		actions: []checkpoint.Action{},
	})
	rep := calibrate(t, store, judge, storage.ResolvedCheckpointFilter{})
	if it := itemFor(t, rep, "a"); it.JudgePass {
		t.Error("JudgePass = true with no actions, want false")
	}
	if n := judge.callCount(); n != 0 {
		t.Errorf("judge called %d times, want 0", n)
	}
}

func TestCalibrate_SkipsUnlabelled(t *testing.T) {
	store, judge := fixture(
		fixtureCase{id: "ok", choice: choiceApprove, reply: "PASS"},
		fixtureCase{id: "weird", choice: "weird", reply: "PASS"},
		fixtureCase{id: "nodecision", choice: choiceApprove, reply: "PASS"},
		fixtureCase{id: "noobjective", choice: choiceApprove, reply: "PASS"},
	)
	for i := range store.checkpoints {
		if store.checkpoints[i].ID == "nodecision" {
			store.checkpoints[i].Decision = nil
		}
	}
	delete(store.objectives, "obj-noobjective")

	rep := calibrate(t, store, judge, storage.ResolvedCheckpointFilter{})
	if rep.N != 1 {
		t.Errorf("N = %d, want 1", rep.N)
	}
	if rep.Skipped != 3 {
		t.Errorf("Skipped = %d, want 3", rep.Skipped)
	}
}

// An item keeps what the judge was shown so it can be exported to a golden set.
func TestCalibrate_ItemKeepsWhatTheJudgeSaw(t *testing.T) {
	store, judge := fixture(fixtureCase{id: "a", choice: choiceApprove, reply: "PASS"})
	rep := calibrate(t, store, judge, storage.ResolvedCheckpointFilter{})
	it := itemFor(t, rep, "a")
	if want := "Objective title <a>"; it.Title != want {
		t.Errorf("Title = %q, want %q", it.Title, want)
	}
	if want := "criterion for a"; it.Criterion != want {
		t.Errorf("Criterion = %q, want %q", it.Criterion, want)
	}
	if want := "1. vcs.open_pr: draft for a\n"; it.Actions != want {
		t.Errorf("Actions = %q, want %q", it.Actions, want)
	}
}

func TestCalibrate_WindowPassedThrough(t *testing.T) {
	store, judge := fixture()
	f := storage.ResolvedCheckpointFilter{
		TwinID: "twin-a",
		Since:  time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		Until:  time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC),
	}
	rep := calibrate(t, store, judge, f)

	if store.gotFilter == nil {
		t.Fatal("store never saw a filter")
	}
	if got := *store.gotFilter; got.TwinID != f.TwinID || !got.Since.Equal(f.Since) || !got.Until.Equal(f.Until) {
		t.Errorf("store filter = %+v, want %+v", got, f)
	}
	if rep.TwinID != f.TwinID || !rep.Since.Equal(f.Since) || !rep.Until.Equal(f.Until) {
		t.Errorf("report window = %q %v %v, want %q %v %v",
			rep.TwinID, rep.Since, rep.Until, f.TwinID, f.Since, f.Until)
	}
	if rep.N != 0 || math.IsNaN(rep.Agreement) || rep.Agreement != 0 {
		t.Errorf("N=%d Agreement=%v, want 0 and 0", rep.N, rep.Agreement)
	}
}

func TestRenderPlanTask_Bounded(t *testing.T) {
	obj := objective.Objective{
		ID:          "obj-1",
		Title:       "Ship the calibration report",
		Description: "Measure how often the judge agrees with reviewers.",
		SuccessCriteria: []objective.Criterion{
			{ID: "c1", Description: "the report lists agreement per decision"},
			{ID: "c2", Description: "the report never writes to storage"},
		},
	}
	actions := []checkpoint.Action{{
		CapabilityID: "vcs.open_pr",
		Params:       map[string]any{"body": strings.Repeat("x", 50_000)},
		Reason:       "open the PR",
	}}

	task := renderPlanTask(obj, actions)

	for _, want := range []string{
		obj.Title,
		obj.SuccessCriteria[0].Description,
		obj.SuccessCriteria[1].Description,
		"vcs.open_pr",
		"PASS or FAIL",
		"…(truncated)",
	} {
		if !strings.Contains(task, want) {
			t.Errorf("task does not contain %q", want)
		}
	}
	if len(task) >= 8000 {
		t.Errorf("len(task) = %d, want < 8000", len(task))
	}
}
