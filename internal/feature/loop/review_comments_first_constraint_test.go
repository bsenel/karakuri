package loop

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 40 step 12, as step 6 was for branches-without-pr: an objective created
// before the template held review-comments-first is told it all the same,
// because withTemplateConstraints reads the template, never the stored copy.

var streamTemplateIDs = []string{
	"software.objective.market_discovery",
	"software.objective.engineering_backlog",
	"software.objective.ux_improvement",
	deliveryTemplateID,
}

func TestEveryStreamObjectiveCreatedBeforeIsToldReviewCommentsFirst(t *testing.T) {
	reg := softwareRegistry(t)
	for _, id := range streamTemplateIDs {
		t.Run(id, func(t *testing.T) {
			tmpl := softwareTemplate(t, id)
			want, n := constraintByID(tmpl.Constraints, "review-comments-first")
			if n != 1 || want.Description == "" {
				t.Fatalf("%s holds %d review-comments-first constraints (%q), want 1 with text", id, n, want.Description)
			}
			var own []objective.Constraint
			for _, c := range tmpl.Constraints {
				if c.ID != "review-comments-first" {
					own = append(own, c)
				}
			}
			obj := objective.Objective{ID: "obj-before-step-12", TemplateID: id, Constraints: own}

			shown := withTemplateConstraints(reg, obj)

			got, n := constraintByID(shown.Constraints, "review-comments-first")
			if n != 1 || !reflect.DeepEqual(got, want) {
				t.Errorf("shown %v, want the template's review-comments-first", constraintIDs(shown.Constraints))
			}
			if len(obj.Constraints) != len(own) {
				t.Errorf("the objective passed in now holds %v", constraintIDs(obj.Constraints))
			}
		})
	}
}

// Every agent call that carries the delivery objective carries the constraint,
// and the stored objective is not written.
func TestEveryAgentInputCarriesReviewCommentsFirst(t *testing.T) {
	tmpl := softwareTemplate(t, deliveryTemplateID)
	want, n := constraintByID(tmpl.Constraints, "review-comments-first")
	if n != 1 {
		t.Fatalf("the delivery template holds %d review-comments-first constraints, want 1", n)
	}
	obj := noMergeOnly(t)
	agent := &inputRecorder{replies: reflexionReplies()}
	sc, _ := constraintContext(t, obj, agent)

	for call, in := range everyAgentInput(t, sc, agent) {
		if got, n := constraintByID(in.Constraints, "review-comments-first"); n != 1 || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: review-comments-first is %+v (%d held), the template's is %+v", call, got, n, want)
		}
	}
	if ids := constraintIDs(sc.obj.Constraints); !reflect.DeepEqual(ids, []string{"no-merge"}) {
		t.Errorf("the step context's objective now holds %v, want only no-merge", ids)
	}
}

// (3) A delivery pass whose git observation lists an owner's unanswered comment
// puts that comment's line in the planner's input before any other work line.
// The observation is what the git environment returns for a forge holding one
// open delivery pull request with an owner's comment after its last commit
// (domains/software/review_comments_first_test.go proves that); this test
// holds the step that carries the observation's lines to the planner, which
// builds its Task in stepReason because the agent runtime renders no
// WorldState.
func TestThePlannersInputLeadsWithTheUnansweredReviewComment(t *testing.T) {
	const comment = "Pull request 7 (karakuri/phase-40-step-12-review-comments-first): owner berat commented on domains/software/x.go:12 after the last commit: rename this"
	const other = "karakuri/phase-40-step-13-next-thing is ahead of main by 2 commits with no pull request"
	obs := environment.Observation{
		EnvID: "software.env.git",
		State: map[string]any{
			"branches_without_pr_lines":        []string{other},
			"unanswered_review_comments_lines": []string{comment},
		},
	}
	ws := loop.WorldState{Observations: []environment.Observation{obs}}

	agent := &inputRecorder{replies: reflexionReplies()}
	sc, _ := constraintContext(t, noMergeOnly(t), agent)
	stepReason(context.Background(), sc, ws)
	if len(agent.inputs) == 0 {
		t.Fatal("the agent was never called")
	}
	task := agent.inputs[0].Task

	at := strings.Index(task, comment)
	if at < 0 {
		t.Fatalf("the planner's task does not carry the comment's line:\n%s", task)
	}
	if o := strings.Index(task, other); o >= 0 && o < at {
		t.Errorf("another work line comes before the comment's:\n%s", task)
	}
}
