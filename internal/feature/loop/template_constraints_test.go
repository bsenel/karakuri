package loop

import (
	"context"
	"reflect"
	"testing"

	"github.com/bsenel/karakuri/domains/software"
	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Phase 40 step 2: a template's action limits are read at run time, and its
// constraints were copied onto an objective only when it was created. An
// objective created before the template gained the action-limits constraint is
// held to limits its planner is never told about, and a plan that leaves
// params.branch out is refused with no checkpoint. The planner is therefore
// shown the template's constraints the objective does not hold, read through
// the same lookup the limits use, and never written into the stored objective.

const deliveryTemplateID = "software.objective.roadmap_delivery"

// inputRecorder records every input it is given and answers from the script,
// repeating the last answer once the script runs out.
type inputRecorder struct {
	inputs  []coreagent.Input
	replies []string
}

func (a *inputRecorder) Run(_ context.Context, in coreagent.Input) (coreagent.Output, error) {
	a.inputs = append(a.inputs, in)
	reply := ""
	if n := len(a.inputs) - 1; n < len(a.replies) {
		reply = a.replies[n]
	} else if len(a.replies) > 0 {
		reply = a.replies[len(a.replies)-1]
	}
	return coreagent.Output{Content: reply, Confidence: 0.9}, nil
}

func (a *inputRecorder) Stream(context.Context, coreagent.Input) (<-chan coreagent.OutputChunk, error) {
	ch := make(chan coreagent.OutputChunk)
	close(ch)
	return ch, nil
}

const recordedPlan = `{"actions":[{"capability":"software.act.write_code","params":{"prompt":"p"},"reason":"r","env_id":"software.env.codebase"}],"confidence":0.9,"reasoning":"draft"}`

func softwareRegistry(t *testing.T) *domain.Registry {
	t.Helper()
	reg := domain.NewRegistry()
	if err := reg.Register(context.Background(), software.New(), domain.Config{}); err != nil {
		t.Fatalf("register software: %v", err)
	}
	return reg
}

func softwareTemplate(t *testing.T, id string) objective.Template {
	t.Helper()
	for _, tmpl := range software.New().ObjectiveTemplates() {
		if tmpl.ID == id {
			return tmpl
		}
	}
	t.Fatalf("the software pack declares no %s", id)
	return objective.Template{}
}

func constraintByID(cs []objective.Constraint, id string) (objective.Constraint, int) {
	var found objective.Constraint
	n := 0
	for _, c := range cs {
		if c.ID == id {
			found = c
			n++
		}
	}
	return found, n
}

func constraintIDs(cs []objective.Constraint) []string {
	ids := make([]string, len(cs))
	for i, c := range cs {
		ids[i] = c.ID
	}
	return ids
}

// noMergeOnly is the delivery objective as it was created before #252: the
// template's no-merge constraint and nothing else.
func noMergeOnly(t *testing.T) objective.Objective {
	t.Helper()
	tmpl := softwareTemplate(t, deliveryTemplateID)
	noMerge, n := constraintByID(tmpl.Constraints, "no-merge")
	if n != 1 {
		t.Fatalf("the delivery template holds %d no-merge constraints, want 1", n)
	}
	if _, n := constraintByID(tmpl.Constraints, "action-limits"); n != 1 {
		t.Fatalf("the delivery template holds %d action-limits constraints, want 1", n)
	}
	return objective.Objective{
		ID:          "obj-delivery-before-252",
		Title:       "deliver the roadmap",
		Domain:      "software",
		TwinID:      "twin-1",
		Mode:        objective.ModeStanding,
		TemplateID:  deliveryTemplateID,
		Constraints: []objective.Constraint{noMerge},
	}
}

// constraintContext is a reason-step context over a real store holding obj,
// with the software pack registered and a Reflexion agent, so one stepReason
// reaches the plan, critique and revise calls.
func constraintContext(t *testing.T, obj objective.Objective, agent coreagent.Agent) (*stepContext, storage.StorageAdapter) {
	t.Helper()
	svc, _, store := newResumeFixture(t)
	svc.domReg = softwareRegistry(t)
	if err := store.SaveObjective(context.Background(), obj); err != nil {
		t.Fatalf("save objective: %v", err)
	}
	return &stepContext{
		svc:   svc,
		agent: agent,
		agentDef: coreagent.Definition{
			ID:                "test-agent",
			ReasoningStrategy: coreagent.ReasoningReflexion,
		},
		obj:       obj,
		loopID:    "loop-constraints-0001",
		iteration: 1,
	}, store
}

// everyAgentInput runs each agent call that carries the objective, the plan,
// critique, revise, modify-revise and judge, and returns the objective each was
// given.
func everyAgentInput(t *testing.T, sc *stepContext, agent *inputRecorder) map[string]objective.Objective {
	t.Helper()
	ctx := context.Background()
	stepReason(ctx, sc, loop.WorldState{})
	if len(agent.inputs) != 3 {
		t.Fatalf("stepReason made %d agent calls, want 3 (plan, critique, revise)", len(agent.inputs))
	}
	got := map[string]coreagent.Input{
		"plan":     agent.inputs[0],
		"critique": agent.inputs[1],
		"revise":   agent.inputs[2],
	}

	draft := plan{Actions: []plannedAction{{CapabilityID: "software.act.write_code"}}, Confidence: 0.6}
	if _, applied := stepReasonRevise(ctx, sc, draft, corecheckpoint.Decision{Choice: "modify", Note: "set the branch"}); !applied {
		t.Fatal("the operator's modify was not applied")
	}
	if len(agent.inputs) != 4 {
		t.Fatalf("stepReasonRevise made %d agent calls, want 1", len(agent.inputs)-3)
	}
	got["modify revise"] = agent.inputs[3]

	evaluateWithAgent(ctx, sc, objective.Criterion{Description: "a pull request is open"}, []actionOutcome{{
		CapabilityID: "software.act.create_pr",
	}})
	if len(agent.inputs) != 5 {
		t.Fatalf("evaluateWithAgent made %d agent calls, want 1", len(agent.inputs)-4)
	}
	got["judge"] = agent.inputs[4]

	shown := make(map[string]objective.Objective, len(got))
	for call, in := range got {
		o, ok := in.Objective.(objective.Objective)
		if !ok {
			t.Fatalf("%s: the agent was given %T, not an objective.Objective", call, in.Objective)
		}
		shown[call] = o
	}
	return shown
}

func reflexionReplies() []string {
	return []string{recordedPlan, "The draft leaves params.branch out.", recordedPlan, recordedPlan, "PASS"}
}

// (1) The delivery objective created before #252 holds only no-merge. Every
// agent input that carries it carries the template's action-limits constraint
// too, and the objective in the store and on the step context still holds only
// no-merge.
func TestAnObjectiveCreatedBeforeItsTemplatesConstraintIsToldIt(t *testing.T) {
	obj := noMergeOnly(t)
	tmpl := softwareTemplate(t, deliveryTemplateID)
	limits, _ := constraintByID(tmpl.Constraints, "action-limits")
	// Phase 40 step 6 added branches-without-pr to every stream template, so
	// an objective created before it is told that rule the same way.
	withoutPR, n := constraintByID(tmpl.Constraints, "branches-without-pr")
	if n != 1 {
		t.Fatalf("the delivery template holds %d branches-without-pr constraints, want 1", n)
	}
	// Phase 40 step 6a added unmergeable-prs to every stream template, ahead
	// of branches-without-pr, so it is told the same way.
	unmergeable, n := constraintByID(tmpl.Constraints, "unmergeable-prs")
	if n != 1 {
		t.Fatalf("the delivery template holds %d unmergeable-prs constraints, want 1", n)
	}

	agent := &inputRecorder{replies: reflexionReplies()}
	sc, store := constraintContext(t, obj, agent)

	for call, in := range everyAgentInput(t, sc, agent) {
		got, n := constraintByID(in.Constraints, "action-limits")
		if n != 1 {
			t.Errorf("%s: the planner was given %d action-limits constraints, want 1: %v",
				call, n, constraintIDs(in.Constraints))
			continue
		}
		if !reflect.DeepEqual(got, limits) {
			t.Errorf("%s: action-limits is %+v, the template's is %+v", call, got, limits)
		}
		if got, n := constraintByID(in.Constraints, "branches-without-pr"); n != 1 || !reflect.DeepEqual(got, withoutPR) {
			t.Errorf("%s: branches-without-pr is %+v (%d held), the template's is %+v", call, got, n, withoutPR)
		}
		if got, n := constraintByID(in.Constraints, "unmergeable-prs"); n != 1 || !reflect.DeepEqual(got, unmergeable) {
			t.Errorf("%s: unmergeable-prs is %+v (%d held), the template's is %+v", call, got, n, unmergeable)
		}
		if ids := constraintIDs(in.Constraints); !reflect.DeepEqual(ids, []string{"no-merge", "action-limits", "unmergeable-prs", "branches-without-pr"}) {
			t.Errorf("%s: constraints %v, want the objective's own first, then the template's", call, ids)
		}
		if in.ID != obj.ID || in.Title != obj.Title || in.TemplateID != obj.TemplateID {
			t.Errorf("%s: the planner was given another objective: %+v", call, in)
		}
	}

	if ids := constraintIDs(sc.obj.Constraints); !reflect.DeepEqual(ids, []string{"no-merge"}) {
		t.Errorf("the step context's objective now holds %v, want only no-merge", ids)
	}
	stored, err := store.GetObjective(context.Background(), obj.ID)
	if err != nil {
		t.Fatalf("read the objective back: %v", err)
	}
	if ids := constraintIDs(stored.Constraints); !reflect.DeepEqual(ids, []string{"no-merge"}) {
		t.Errorf("the stored objective now holds %v; the template's constraints are read, never written", ids)
	}
}

// (1) Every stream objective created before #252 holds only no-merge, and each
// is told its own template's action-limits constraint, not another stream's.
func TestEveryStreamIsToldItsOwnTemplatesLimits(t *testing.T) {
	reg := softwareRegistry(t)
	for _, id := range []string{
		"software.objective.market_discovery",
		"software.objective.engineering_backlog",
		"software.objective.ux_improvement",
		deliveryTemplateID,
	} {
		t.Run(id, func(t *testing.T) {
			tmpl := softwareTemplate(t, id)
			noMerge, _ := constraintByID(tmpl.Constraints, "no-merge")
			want, n := constraintByID(tmpl.Constraints, "action-limits")
			if n != 1 {
				t.Fatalf("%s holds %d action-limits constraints, want 1", id, n)
			}
			obj := objective.Objective{ID: "obj-stream", TemplateID: id, Constraints: []objective.Constraint{noMerge}}

			shown := withTemplateConstraints(reg, obj)

			got, n := constraintByID(shown.Constraints, "action-limits")
			if n != 1 || !reflect.DeepEqual(got, want) {
				t.Errorf("shown %v, want the template's own action-limits %q", constraintIDs(shown.Constraints), want.Description)
			}
			if ids := constraintIDs(obj.Constraints); !reflect.DeepEqual(ids, []string{"no-merge"}) {
				t.Errorf("the objective passed in now holds %v", ids)
			}
		})
	}
}

// (2) An objective that already holds a constraint with a template
// constraint's ID keeps its own: one constraint with that ID, the objective's
// text, in the objective's position.
func TestTheObjectivesOwnConstraintWinsOverTheTemplates(t *testing.T) {
	tmpl := softwareTemplate(t, deliveryTemplateID)
	noMerge, _ := constraintByID(tmpl.Constraints, "no-merge")
	unmergeable, _ := constraintByID(tmpl.Constraints, "unmergeable-prs")
	withoutPR, _ := constraintByID(tmpl.Constraints, "branches-without-pr")
	mine := objective.Constraint{
		ID:          "action-limits",
		Description: "Every coding action sets params.branch to karakuri/phase-40-action-limits.",
		Hard:        true,
	}
	obj := objective.Objective{
		ID:          "obj-own-limits",
		Title:       "deliver the roadmap",
		Domain:      "software",
		TemplateID:  deliveryTemplateID,
		Constraints: []objective.Constraint{mine, noMerge},
	}

	agent := &inputRecorder{replies: reflexionReplies()}
	sc, store := constraintContext(t, obj, agent)

	for call, in := range everyAgentInput(t, sc, agent) {
		got, n := constraintByID(in.Constraints, "action-limits")
		if n != 1 {
			t.Errorf("%s: %d action-limits constraints, want exactly 1", call, n)
		}
		if !reflect.DeepEqual(got, mine) {
			t.Errorf("%s: action-limits is %q, want the objective's own %q", call, got.Description, mine.Description)
		}
		if _, n := constraintByID(in.Constraints, "no-merge"); n != 1 {
			t.Errorf("%s: %d no-merge constraints, want exactly 1", call, n)
		}
		// The objective's own, unchanged and in place, then the template's
		// ones it does not hold.
		want := []objective.Constraint{mine, noMerge, unmergeable, withoutPR}
		if !reflect.DeepEqual(in.Constraints, want) {
			t.Errorf("%s: constraints %+v, want the objective's own unchanged, then the template's unmergeable-prs and branches-without-pr: %+v", call, in.Constraints, want)
		}
	}

	stored, err := store.GetObjective(context.Background(), obj.ID)
	if err != nil {
		t.Fatalf("read the objective back: %v", err)
	}
	if !reflect.DeepEqual(stored.Constraints, obj.Constraints) {
		t.Errorf("the stored constraints changed: %+v", stored.Constraints)
	}
}

// (2) The same rule on the helper alone, for a template constraint the
// objective holds under different text and a second it does not hold.
func TestTemplateConstraintsAreMatchedByID(t *testing.T) {
	reg := softwareRegistry(t)
	tmpl := softwareTemplate(t, deliveryTemplateID)
	limits, _ := constraintByID(tmpl.Constraints, "action-limits")
	unmergeable, _ := constraintByID(tmpl.Constraints, "unmergeable-prs")
	withoutPR, _ := constraintByID(tmpl.Constraints, "branches-without-pr")
	mine := objective.Constraint{ID: "no-merge", Description: "never merge, said differently", Hard: false}
	obj := objective.Objective{ID: "obj-id-match", TemplateID: deliveryTemplateID, Constraints: []objective.Constraint{mine}}

	shown := withTemplateConstraints(reg, obj)

	want := []objective.Constraint{mine, limits, unmergeable, withoutPR}
	if !reflect.DeepEqual(shown.Constraints, want) {
		t.Errorf("shown %+v\nwant %+v", shown.Constraints, want)
	}
}

// The copy's constraints are a new slice: appending the template's must not
// write into spare capacity behind the objective's own, which would change
// what the caller's objective, and anything saved from it, holds.
func TestTheShownObjectiveSharesNoConstraintStorage(t *testing.T) {
	reg := softwareRegistry(t)
	tmpl := softwareTemplate(t, deliveryTemplateID)
	noMerge, _ := constraintByID(tmpl.Constraints, "no-merge")
	backing := make([]objective.Constraint, 1, 4)
	backing[0] = noMerge
	obj := objective.Objective{ID: "obj-capacity", TemplateID: deliveryTemplateID, Constraints: backing}

	shown := withTemplateConstraints(reg, obj)
	if len(shown.Constraints) < 2 {
		t.Fatalf("shown %v, want the template's action-limits added", constraintIDs(shown.Constraints))
	}
	if spare := backing[:2][1]; spare != (objective.Constraint{}) {
		t.Errorf("the template's constraint was written into the objective's backing array: %+v", spare)
	}
	shown.Constraints[0].Description = "edited"
	if obj.Constraints[0].Description != noMerge.Description {
		t.Error("editing the shown objective's constraint edited the objective's own")
	}

	// The template's own slice is not handed out either.
	again := softwareTemplate(t, deliveryTemplateID)
	if !reflect.DeepEqual(again.Constraints, tmpl.Constraints) {
		t.Errorf("the template's constraints changed: %+v", again.Constraints)
	}
}

// (3) An objective with no template, and one whose template no pack declares,
// is shown exactly its own constraints, as before.
func TestAnObjectiveWithoutAKnownTemplateIsShownOnlyItsOwnConstraints(t *testing.T) {
	own := []objective.Constraint{
		{ID: "no-merge", Description: "never merge", Hard: true},
		{ID: "budget", Description: "stay under budget"},
	}
	for name, templateID := range map[string]string{
		"no template":      "",
		"unknown template": "software.objective.retired_long_ago",
	} {
		t.Run(name, func(t *testing.T) {
			obj := objective.Objective{
				ID:          "obj-no-template",
				Title:       "hand-written objective",
				Domain:      "software",
				TemplateID:  templateID,
				Constraints: append([]objective.Constraint(nil), own...),
			}
			agent := &inputRecorder{replies: reflexionReplies()}
			sc, _ := constraintContext(t, obj, agent)

			for call, in := range everyAgentInput(t, sc, agent) {
				if !reflect.DeepEqual(in.Constraints, own) {
					t.Errorf("%s: shown %+v, want exactly the objective's own %+v", call, in.Constraints, own)
				}
			}
		})
	}

	t.Run("nil registry", func(t *testing.T) {
		obj := objective.Objective{ID: "obj-nil-reg", TemplateID: deliveryTemplateID, Constraints: own}
		if got := withTemplateConstraints(nil, obj); !reflect.DeepEqual(got.Constraints, own) {
			t.Errorf("shown %+v with no registry, want the objective's own", got.Constraints)
		}
	})
}
