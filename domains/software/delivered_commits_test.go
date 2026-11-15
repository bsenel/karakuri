package software

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 44 step 3 slice B: each open pull request with a checklist also
// carries the commits on its head branch since its base, so a plan sees what
// was delivered on the branch, not only what was ticked.

// stackedPRs is what the forge lists in the slice B fixture: a delivery pull
// request with a checklist, stacked on an earlier group, and one whose body
// has no task list.
func stackedPRs() []versioncontrol.PRSummary {
	return []versioncontrol.PRSummary{
		{ID: "246", Number: 246, Title: "feat(software): flag a ticked step (phase 44 step 3)",
			Body:       "- [x] A: checklist in the observation\n- [ ] B: commits since base",
			BaseBranch: "karakuri/phase-44-mark-ready", HeadBranch: "karakuri/phase-44-step-03",
			Author: "stranger"},
		{ID: "42", Number: 42, Title: "fix(cli): no list",
			Body:       "A plain description with no task list at all.",
			BaseBranch: "main", HeadBranch: "karakuri/no-list", Author: "stranger"},
	}
}

// twoCommits is what the forge compares for pull request 246: two commits,
// oldest first, the first with a body under its subject.
func twoCommits() []versioncontrol.Commit {
	return []versioncontrol.Commit{
		{SHA: "a0a3ad28c5f4238900b6b9476156e77ff3a80e05", Message: "test(software): specify the checklist\n\nSlice A."},
		{SHA: "b2dca659d49bc624890076218fe35ee284da7a15", Message: "feat(software): show the checklist"},
	}
}

// observeStacked observes the git environment of repo acme/api over vc and
// returns the observation with its pull request entries read back as JSON,
// the shape the planner and the world-state record see.
func observeStacked(t *testing.T, vc *vcStub) (environment.Observation, []map[string]any) {
	t.Helper()
	env := &gitEnv{id: EnvGit, vc: vc}
	obs, err := env.Observe(context.Background(), environment.ObservationQuery{
		Filter: map[string]any{"repo": "acme/api"},
	})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	raw, err := json.Marshal(obs.State["prs"])
	if err != nil {
		t.Fatalf("marshal prs: %v", err)
	}
	var entries []map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("the observation's prs are not a list of objects: %v (%s)", err, raw)
	}
	if len(entries) != len(vc.prs) {
		t.Fatalf("observation carries %d pull requests, want %d", len(entries), len(vc.prs))
	}
	return obs, entries
}

// sinceBaseOf reads one entry's commits_since_base back as "sha subject"
// strings, failing when the entry has no such list.
func sinceBaseOf(t *testing.T, entry map[string]any) []string {
	t.Helper()
	list, ok := entry["commits_since_base"].([]any)
	if !ok {
		t.Fatalf("pull request entry has no commits_since_base list: %#v", entry["commits_since_base"])
	}
	got := make([]string, 0, len(list))
	for i, v := range list {
		c, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("commit %d is not an object: %#v", i, v)
		}
		sha, _ := c["sha"].(string)
		subject, _ := c["subject"].(string)
		got = append(got, sha+" "+subject)
	}
	return got
}

// sinceBaseLines is the observation's commits_since_base_lines, failing when
// it is not a []string.
func sinceBaseLines(t *testing.T, obs environment.Observation) []string {
	t.Helper()
	lines, ok := obs.State["commits_since_base_lines"].([]string)
	if !ok {
		t.Fatalf("observation has no commits_since_base_lines []string: %#v", obs.State["commits_since_base_lines"])
	}
	return lines
}

// A pull request with a checklist carries the commits from its base to its
// head, one compare call for it in the observed repository, each commit as a
// short SHA and its subject line, and the planner gets one line per commit.
func TestGitObservationListsTheCommitsSinceBaseOfAPullRequestWithAChecklist(t *testing.T) {
	vc := &vcStub{prs: stackedPRs(), compared: twoCommits()}
	obs, entries := observeStacked(t, vc)

	wantCalls := []string{"acme/api:karakuri/phase-44-mark-ready...karakuri/phase-44-step-03"}
	if !slices.Equal(vc.compares, wantCalls) {
		t.Errorf("compare calls = %q, want %q", vc.compares, wantCalls)
	}
	want := []string{
		"a0a3ad2 test(software): specify the checklist",
		"b2dca65 feat(software): show the checklist",
	}
	if got := sinceBaseOf(t, entries[0]); !reflect.DeepEqual(got, want) {
		t.Errorf("commits since base of pull request 246 =\n  %q\nwant\n  %q", got, want)
	}
	// Not capped and not unavailable, so neither is said.
	for _, key := range []string{"commits_since_base_more", "commits_since_base_unavailable"} {
		if v, ok := entries[0][key]; ok {
			t.Errorf("pull request 246 carries %s = %#v with every commit listed", key, v)
		}
	}

	wantLines := []string{
		"#246 a0a3ad2 test(software): specify the checklist",
		"#246 b2dca65 feat(software): show the checklist",
	}
	if got := sinceBaseLines(t, obs); !slices.Equal(got, wantLines) {
		t.Errorf("commits_since_base_lines =\n  %q\nwant\n  %q", got, wantLines)
	}
}

// At most 30 commits are carried, the first 30 the forge lists, and the entry
// and the lines say how many more there are.
func TestGitObservationCapsTheCommitsSinceBaseAndSaysHowManyMore(t *testing.T) {
	many := make([]versioncontrol.Commit, 35)
	for i := range many {
		many[i] = versioncontrol.Commit{SHA: fmt.Sprintf("%07x%033d", i+1, 0), Message: fmt.Sprintf("commit %d", i+1)}
	}
	obs, entries := observeStacked(t, &vcStub{prs: stackedPRs(), compared: many})

	got := sinceBaseOf(t, entries[0])
	if len(got) != 30 {
		t.Fatalf("pull request 246 carries %d commits since base, want 30", len(got))
	}
	if got[0] != "0000001 commit 1" || got[29] != "000001e commit 30" {
		t.Errorf("carried commits run %q to %q, want the forge's first 30", got[0], got[29])
	}
	if more, _ := entries[0]["commits_since_base_more"].(float64); more != 5 {
		t.Errorf("commits_since_base_more = %#v, want 5", entries[0]["commits_since_base_more"])
	}

	lines := sinceBaseLines(t, obs)
	if len(lines) != 31 {
		t.Errorf("commits_since_base_lines has %d lines, want 30 commits and one for the rest: %q", len(lines), lines)
	}
	if !slices.Contains(lines, "#246 commits since base: 5 more not shown") {
		t.Errorf("no line says how many more commits there are: %q", lines)
	}
}

// When the forge cannot compare, the pull request says so with the reason,
// never an empty list that would read as "nothing delivered on this branch"
// (AGENTS.md rule 10). An unsupported read and a failed one are told alike.
func TestGitObservationSaysWhenTheCommitsSinceBaseAreUnavailable(t *testing.T) {
	for name, err := range map[string]error{
		"unsupported": fmt.Errorf("stub: compare commits: %w", versioncontrol.ErrUnsupported),
		"failed":      errors.New("github: GET compare: 502 Bad Gateway"),
	} {
		t.Run(name, func(t *testing.T) {
			obs, entries := observeStacked(t, &vcStub{prs: stackedPRs(), compareErr: err})

			if v, ok := entries[0]["commits_since_base"]; ok {
				t.Errorf("pull request 246 carries commits_since_base %#v though the compare failed", v)
			}
			if got, _ := entries[0]["commits_since_base_unavailable"].(string); got != err.Error() {
				t.Errorf("commits_since_base_unavailable = %#v, want %q", entries[0]["commits_since_base_unavailable"], err.Error())
			}
			want := []string{"#246 commits since base: unavailable: " + err.Error()}
			if got := sinceBaseLines(t, obs); !slices.Equal(got, want) {
				t.Errorf("commits_since_base_lines = %q, want %q", got, want)
			}
		})
	}
}

// A pull request whose body has no task list is not a delivery group the plan
// checks against, so the observation spends no compare call on it and its
// entry carries nothing about commits since base.
func TestGitObservationMakesNoCompareCallForAPullRequestWithoutAChecklist(t *testing.T) {
	vc := &vcStub{prs: stackedPRs(), compared: twoCommits()}
	obs, entries := observeStacked(t, vc)

	for _, call := range vc.compares {
		if strings.Contains(call, "karakuri/no-list") {
			t.Errorf("compared pull request 42, which has no checklist: %q", call)
		}
	}
	for _, key := range []string{"commits_since_base", "commits_since_base_more", "commits_since_base_unavailable"} {
		if v, ok := entries[1][key]; ok {
			t.Errorf("pull request 42 has no checklist but carries %s = %#v", key, v)
		}
	}
	for _, line := range sinceBaseLines(t, obs) {
		if strings.HasPrefix(line, "#42 ") {
			t.Errorf("a line about pull request 42, which has no checklist: %q", line)
		}
	}

	// With no pull request that has a checklist, no compare call at all.
	alone := &vcStub{prs: stackedPRs()[1:], compared: twoCommits()}
	observeStacked(t, alone)
	if len(alone.compares) != 0 {
		t.Errorf("compare calls %q with no pull request that has a checklist", alone.compares)
	}
}
