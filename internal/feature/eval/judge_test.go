package eval

import (
	"context"
	"errors"
	"testing"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/feature/loop"
	"github.com/bsenel/karakuri/internal/platform/storage"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	"github.com/bsenel/karakuri/quota/cost"
)

// fixedJudge answers every objective with the same agent, for the tests that
// are not about which agent judges.
func fixedJudge(a coreagent.Agent) JudgeFor {
	return func(context.Context, objective.Objective) (coreagent.Agent, error) { return a, nil }
}

// meteredJudge replies PASS and reports what the call spent.
type meteredJudge struct {
	name  string
	calls int
}

func (j *meteredJudge) Run(context.Context, coreagent.Input) (coreagent.Output, error) {
	j.calls++
	return coreagent.Output{Content: "PASS", TokensUsed: 120, Provider: "fakeprov", Model: j.name}, nil
}

func (j *meteredJudge) Stream(context.Context, coreagent.Input) (<-chan coreagent.OutputChunk, error) {
	return nil, errors.New("meteredJudge does not stream")
}

func twoObjectiveStore() *fakeStore {
	store := &fakeStore{objectives: map[objective.ObjectiveID]objective.Objective{
		"obj-impl":  {ID: "obj-impl", Title: "implement", AgentID: "software.agent.implementer"},
		"obj-maint": {ID: "obj-maint", Title: "maintain", AgentID: "software.agent.maintainer"},
	}}
	for _, id := range []objective.ObjectiveID{"obj-impl", "obj-maint"} {
		store.checkpoints = append(store.checkpoints, checkpoint.Checkpoint{
			ID: "cp-" + string(id), ObjectiveID: id, TwinID: "twin-a",
			Actions:  []checkpoint.Action{{CapabilityID: "vcs.open_pr"}},
			Status:   checkpoint.StatusResolved,
			Decision: &checkpoint.Decision{Choice: decisionApprove},
		})
	}
	return store
}

// The loop judges a criterion with the objective's own agent, which chooses
// its provider and temperature. A calibration that asked one fixed agent about
// every objective would measure a judge the loop does not run for most of them.
func TestCalibrate_JudgesEachObjectiveWithItsOwnAgent(t *testing.T) {
	judges := map[string]*meteredJudge{
		"software.agent.implementer": {name: "implementer-model"},
		"software.agent.maintainer":  {name: "maintainer-model"},
	}
	var asked []string
	judgeFor := func(_ context.Context, obj objective.Objective) (coreagent.Agent, error) {
		asked = append(asked, obj.AgentID)
		return judges[obj.AgentID], nil
	}

	rep, err := NewService(twoObjectiveStore(), judgeFor, nil).Calibrate(context.Background(), storage.ResolvedCheckpointFilter{})
	if err != nil {
		t.Fatalf("Calibrate: %v", err)
	}
	if rep.N != 2 {
		t.Fatalf("N = %d, want 2", rep.N)
	}
	for id, j := range judges {
		if j.calls != 1 {
			t.Errorf("%s judged %d checkpoints, want exactly its own 1 (asked for: %v)", id, j.calls, asked)
		}
	}
}

// An objective whose agent cannot be built has no judge. That is a FAIL with
// the reason recorded, as when the judge errors, and not a skipped label.
func TestCalibrate_UnavailableJudgeCountsAsFail(t *testing.T) {
	judgeFor := func(context.Context, objective.Objective) (coreagent.Agent, error) {
		return nil, errors.New("no provider for this agent")
	}
	rep, err := NewService(twoObjectiveStore(), judgeFor, nil).Calibrate(context.Background(), storage.ResolvedCheckpointFilter{})
	if err != nil {
		t.Fatalf("Calibrate: %v", err)
	}
	if rep.N != 2 || rep.Skipped != 0 {
		t.Fatalf("N = %d, Skipped = %d, want 2 and 0", rep.N, rep.Skipped)
	}
	for _, it := range rep.Items {
		if it.JudgePass || it.Error == "" {
			t.Errorf("%s: JudgePass = %v, Error = %q; want a recorded failure", it.CheckpointID, it.JudgePass, it.Error)
		}
	}
}

// A calibration spends a model call per checkpoint, and the ledger is how an
// operator sees what anything cost. It is charged to the twin and not to the
// objective: an objective's ledger entries count toward its own daily budget,
// and measuring the judge must not push a standing objective over its ceiling.
func TestCalibrate_RecordsSpendAgainstTheTwin(t *testing.T) {
	ledger := &capturingLedger{}
	costs := &karakuriquota.Recorder{Ledger: ledger}
	judge := &meteredJudge{name: "judge-model"}

	if _, err := NewService(twoObjectiveStore(), fixedJudge(judge), costs).Calibrate(context.Background(), storage.ResolvedCheckpointFilter{}); err != nil {
		t.Fatalf("Calibrate: %v", err)
	}

	if len(ledger.events) != 2 {
		t.Fatalf("recorded %d ledger events, want 2 (one per judge call)", len(ledger.events))
	}
	for _, e := range ledger.events {
		if e.ResourceType != "twin" || e.ResourceID != "twin-a" {
			t.Errorf("spend recorded against %s %q, want twin \"twin-a\"", e.ResourceType, e.ResourceID)
		}
		if e.Units != 120 || e.UnitKind != cost.UnitTokens || e.Provider != "fakeprov" || e.Model != "judge-model" {
			t.Errorf("event = %+v, want 120 tokens from fakeprov/judge-model", e)
		}
	}
}

// capturingLedger keeps the events a Recorder writes.
type capturingLedger struct{ events []cost.Event }

func (l *capturingLedger) Record(_ context.Context, e cost.Event) error {
	l.events = append(l.events, e)
	return nil
}

func (l *capturingLedger) Aggregate(context.Context, cost.Query) ([]cost.Bucket, error) {
	return nil, nil
}

// countingBuilder builds a distinct agent per definition and counts the builds.
type countingBuilder struct {
	defs []coreagent.Definition
	err  error
}

func (b *countingBuilder) New(_ context.Context, def coreagent.Definition) (coreagent.Agent, error) {
	if b.err != nil {
		return nil, b.err
	}
	b.defs = append(b.defs, def)
	return &meteredJudge{name: string(def.ID)}, nil
}

// LoopJudge must pick the agent the loop picks, and build it once.
func TestLoopJudge_SelectsTheLoopsAgentAndBuildsItOnce(t *testing.T) {
	b := &countingBuilder{}
	judgeFor := LoopJudge(b, nil)
	software := objective.Objective{ID: "o1", Domain: "software"}
	farm := objective.Objective{ID: "o2", Domain: "agriculture"}

	first, err := judgeFor(context.Background(), software)
	if err != nil {
		t.Fatalf("judgeFor: %v", err)
	}
	again, _ := judgeFor(context.Background(), software)
	other, _ := judgeFor(context.Background(), farm)

	if first != again {
		t.Error("the same objective's judge was built twice")
	}
	if first == other {
		t.Error("objectives in different domains were given the same judge")
	}
	if len(b.defs) != 2 {
		t.Fatalf("built %d agents, want 2", len(b.defs))
	}
	if want := loop.SelectAgent(nil, software, coreagent.Definition{}); b.defs[0].ID != want.ID {
		t.Errorf("built agent %q, want the loop's choice %q", b.defs[0].ID, want.ID)
	}
}

func TestLoopJudge_ReportsABuildFailure(t *testing.T) {
	judgeFor := LoopJudge(&countingBuilder{err: errors.New("no provider")}, nil)
	if _, err := judgeFor(context.Background(), objective.Objective{ID: "o1", Domain: "software"}); err == nil {
		t.Fatal("expected the factory's error")
	}
}
