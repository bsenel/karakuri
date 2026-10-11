package software

import (
	"regexp"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/domain"
)

// pullRequestTitleHint finds the hint by the form it teaches rather than by
// position: a second hint about titles is a second place for the rule to drift.
func pullRequestTitleHint(t *testing.T) domain.PlannerHint {
	t.Helper()
	var found []domain.PlannerHint
	for _, h := range softwarePlannerHints() {
		if strings.Contains(h.Guidance, pullRequestTitleForm) {
			found = append(found, h)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d planner hints name the form %s, want 1", len(found), pullRequestTitleForm)
	}
	return found[0]
}

// A title is set by whatever opens or edits the pull request, which is not
// only create_pr: a delegated run pushes and titles its own.
func TestPullRequestTitleHintAppliesWhereverAPullRequestIsOpenedOrEdited(t *testing.T) {
	hint := pullRequestTitleHint(t)
	for _, capability := range []string{
		"software.act.create_pr",
		"software.act.write_code",
		"software.act.write_test",
		"software.act.delegate_to_cli",
	} {
		if !strings.Contains(hint.Condition, capability) {
			t.Errorf("Condition %q does not name %s", hint.Condition, capability)
		}
	}
}

func TestPullRequestTitleHintNamesEveryTypeAndWhatItReleases(t *testing.T) {
	guidance := pullRequestTitleHint(t).Guidance
	for _, typ := range []string{"feat", "fix", "perf", "docs", "refactor", "test", "build", "ci", "chore"} {
		if !regexp.MustCompile(`\b` + typ + `\b`).MatchString(guidance) {
			t.Errorf("Guidance does not name the type %q", typ)
		}
	}
	for _, bump := range []string{"minor", "patch"} {
		if !strings.Contains(guidance, bump) {
			t.Errorf("Guidance does not say which types are a %s release", bump)
		}
	}
	if !strings.Contains(guidance, "!") {
		t.Error("Guidance does not mention `!` for a breaking change")
	}
}

func TestPullRequestTitleHintCarriesExamplesThatPass(t *testing.T) {
	guidance := pullRequestTitleHint(t).Guidance
	carried := 0
	for _, example := range []string{
		"feat(mcp): attach karakuri tools to delegated agent runs (phase 34)",
		"fix(cli): shift audit export window to the requested hours (EB-003)",
		"docs(roadmap): propose phases 41 to 43 (discovery 2026-10-18)",
	} {
		if !strings.Contains(guidance, example) {
			continue
		}
		carried++
		if err := ValidPullRequestTitle(example); err != nil {
			t.Errorf("the hint's example %q does not pass: %v", example, err)
		}
	}
	if carried < 2 {
		t.Errorf("Guidance carries %d of the rule's examples, want at least 2", carried)
	}
}

// A hint is read on every plan.
func TestPullRequestTitleHintStaysShort(t *testing.T) {
	guidance := pullRequestTitleHint(t).Guidance
	if n := len(guidance); n > 600 {
		t.Errorf("Guidance is %d characters, want at most 600", n)
	}
}
