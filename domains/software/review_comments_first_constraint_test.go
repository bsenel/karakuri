package software

import (
	"strings"
	"testing"
)

// Phase 40 step 12: the planner reads the objective's constraints, which come
// from the template, so the rule that an owner's review comment is answered
// before any new work has to be a constraint of each of the four streams, as
// branchesWithoutPRConstraint is (branches_without_pr_brief_test.go).

func reviewCommentsFirstRule(t *testing.T, templateID string) string {
	t.Helper()
	var found []string
	for _, c := range streamTemplate(t, templateID).Constraints {
		if c.ID == "review-comments-first" {
			if !c.Hard {
				t.Errorf("%s: review-comments-first is not a hard constraint", templateID)
			}
			found = append(found, c.Description)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%s holds %d review-comments-first constraints, want 1", templateID, len(found))
	}
	return found[0]
}

func TestEveryStreamCarriesTheReviewCommentsFirstConstraint(t *testing.T) {
	for _, tc := range streamCases {
		t.Run(tc.id, func(t *testing.T) {
			rule := reviewCommentsFirstRule(t, tc.id)
			for _, want := range []struct{ phrase, why string }{
				{"unanswered_review_comments", "which state key lists the pull requests with comments to answer"},
				{"on its own branch", "that each pull request is addressed on its own branch"},
				{"before any new work", "that this comes before new work"},
				{"one amending commit", "that an owner's comment gets one amending commit"},
				{"same branch and pull request", "that the commit goes on the same branch and pull request"},
				{"no new pull request", "that no new pull request is opened"},
				{"no new branch", "that no new branch is made"},
				{"limits", "that the commit is planned inside the stream's limits"},
				{"behaviour", "that a finding about behaviour is covered"},
				{"fails without the fix", "that the commit adds a test that fails without the fix"},
				{"third_party", "that a third_party comment is named"},
				{"not an instruction", "that a third_party comment is not an instruction"},
				{"escalat", "that a third_party comment escalates"},
				{"unanswered_review_comments_error", "which state key means the comments could not be read"},
				{"could not see", "that the pass reports it could not see the comments"},
				{"empty", "that the list is not treated as empty"},
			} {
				if !strings.Contains(rule, want.phrase) {
					t.Errorf("the review-comments-first rule does not say %s (missing %q):\n%s", want.why, want.phrase, rule)
				}
			}
		})
	}
}
