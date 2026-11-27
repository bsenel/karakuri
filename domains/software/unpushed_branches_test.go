package software

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 40 step 6: the git observation lists each stream's branches that are
// ahead of main and have no open pull request, so the stream's next pass opens
// that pull request before new work. A remote that cannot be read is an error
// on the observation, never an empty list (AGENTS.md rule 10).

// branchForge is the package's vcStub with what this step needs on top: the
// commits each branch has that main does not, and separate failures for
// listing branches and listing pull requests.
//
// ListPRs drops an open pull request last updated before since, as GitHub's
// adapter does. Only a zero since returns every open pull request, so an old
// open pull request is missed by a read bounded to the observation window.
//
// HasPullRequestForHead answers from hadPR, a per-branch table of branches
// with a pull request in any state (open, merged or closed unmerged); a branch
// with an open pull request in prs answers true as well. headErr fails the
// question for the branches it names, and prsErr fails every pull request
// read, the list and the per-branch question alike.
type branchForge struct {
	vcStub
	// ahead is what CompareCommits("main", branch) returns for each branch.
	ahead      map[string][]versioncontrol.Commit
	branchErr  error
	prsErr     error
	prsSinces  []time.Time
	branchRead int

	hadPR    map[string]bool
	headErr  map[string]error
	headAsks []string
}

func (f *branchForge) ListPullRequestComments(context.Context, string, int) ([]versioncontrol.PullRequestComment, error) {
	return nil, versioncontrol.ErrUnsupported
}

func (f *branchForge) FileAt(context.Context, string, string, string) (string, error) {
	return "", versioncontrol.ErrUnsupported
}

func (f *branchForge) HasPullRequestForHead(_ context.Context, _, branch string) (bool, error) {
	f.headAsks = append(f.headAsks, branch)
	if f.prsErr != nil {
		return false, f.prsErr
	}
	if err := f.headErr[branch]; err != nil {
		// true with the error, so a caller that reads the bool despite the
		// error leaves the branch out and is caught by the blind tests.
		return true, err
	}
	if f.hadPR[branch] {
		return true, nil
	}
	for _, pr := range f.prs {
		if pr.HeadBranch == branch {
			return true, nil
		}
	}
	return false, nil
}

func (f *branchForge) ListPRs(_ context.Context, _ string, since time.Time) ([]versioncontrol.PRSummary, error) {
	f.prsSinces = append(f.prsSinces, since)
	if f.prsErr != nil {
		return nil, f.prsErr
	}
	var out []versioncontrol.PRSummary
	for _, pr := range f.prs {
		if !since.IsZero() && pr.UpdatedAt.Before(since) {
			continue
		}
		out = append(out, pr)
	}
	return out, nil
}

func (f *branchForge) ListBranches(_ context.Context, _, prefix string) ([]versioncontrol.Branch, error) {
	f.branchRead++
	if f.branchErr != nil {
		return nil, f.branchErr
	}
	var out []versioncontrol.Branch
	for _, b := range f.branches {
		if strings.HasPrefix(b.Name, prefix) {
			out = append(out, b)
		}
	}
	return out, nil
}

func (f *branchForge) CompareCommits(_ context.Context, _, base, head string) ([]versioncontrol.Commit, error) {
	if base != "main" {
		return nil, nil
	}
	return f.ahead[head], nil
}

// branchWithoutPR is one entry of state["branches_without_pr"], read through
// JSON so the test does not fix the Go type the implementation chooses.
type branchWithoutPR struct {
	Branch string `json:"branch"`
	SHA    string `json:"sha"`
	Stream string `json:"stream"`
	Ahead  int    `json:"ahead"`
}

func branchesWithoutPR(t *testing.T, state map[string]any) []branchWithoutPR {
	t.Helper()
	raw, ok := state["branches_without_pr"]
	if !ok {
		t.Fatalf("observation has no branches_without_pr; state keys: %v", stateKeys(state))
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal branches_without_pr: %v", err)
	}
	var out []branchWithoutPR
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("branches_without_pr is not a list of {branch, sha, stream, ahead}: %v (%s)", err, b)
	}
	return out
}

func branchLines(t *testing.T, state map[string]any) []string {
	t.Helper()
	raw, ok := state["branches_without_pr_lines"]
	if !ok {
		t.Fatalf("observation has no branches_without_pr_lines; state keys: %v", stateKeys(state))
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal branches_without_pr_lines: %v", err)
	}
	var out []string
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("branches_without_pr_lines is not a list of strings: %v (%s)", err, b)
	}
	return out
}

func stateKeys(state map[string]any) []string {
	keys := make([]string, 0, len(state))
	for k := range state {
		keys = append(keys, k)
	}
	return keys
}

func aheadBy(n int) []versioncontrol.Commit {
	out := make([]versioncontrol.Commit, n)
	for i := range out {
		out[i] = versioncontrol.Commit{SHA: strings.Repeat("c", 7) + string(rune('0'+i)), Message: "work"}
	}
	return out
}

func observeGit(t *testing.T, forge *branchForge) environment.Observation {
	t.Helper()
	obs, err := (&gitEnv{id: EnvGit, vc: forge}).Observe(context.Background(), environment.ObservationQuery{})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	return obs
}

// Each stream's branch that is ahead of main with no open pull request is
// listed, tagged with the stream whose patterns it matches, both as a state
// field and as a line the planner reads.
func TestGitObservationListsEachStreamsBranchAheadOfMainWithNoPR(t *testing.T) {
	want := map[string]string{
		"karakuri/discovery-2026-11-16":            marketDiscoveryTemplateID,
		"karakuri/tech-backlog-2026-11-16":         engineeringBacklogTemplateID,
		"karakuri/ux-2026-11-16":                   uxImprovementTemplateID,
		"karakuri/phase-40-step-06-branch-gets-pr": roadmapDeliveryTemplateID,
		"karakuri/eb-012-rate-limit-resume":        roadmapDeliveryTemplateID,
	}
	forge := &branchForge{ahead: map[string][]versioncontrol.Commit{}}
	for name := range want {
		forge.branches = append(forge.branches, versioncontrol.Branch{Name: name, SHA: "sha-" + name})
		forge.ahead[name] = aheadBy(2)
	}

	obs := observeGit(t, forge)

	got := branchesWithoutPR(t, obs.State)
	if len(got) != len(want) {
		t.Fatalf("listed %d branches, want %d: %+v", len(got), len(want), got)
	}
	for _, b := range got {
		stream, ok := want[b.Branch]
		if !ok {
			t.Errorf("listed unexpected branch %q", b.Branch)
			continue
		}
		if b.Stream != stream {
			t.Errorf("%s: stream = %q, want %q", b.Branch, b.Stream, stream)
		}
		if b.SHA != "sha-"+b.Branch {
			t.Errorf("%s: sha = %q, want the branch tip", b.Branch, b.SHA)
		}
		if b.Ahead != 2 {
			t.Errorf("%s: ahead = %d, want 2", b.Branch, b.Ahead)
		}
	}

	lines := branchLines(t, obs.State)
	for name, stream := range want {
		found := false
		for _, l := range lines {
			if strings.Contains(l, name) && strings.Contains(l, stream) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no planner line names %s and its stream %s; lines: %q", name, stream, lines)
		}
	}
}

// A branch with an open pull request, one not ahead of main and one outside
// every stream's patterns are not listed. The open pull request is older than
// the observation window, which a read bounded to the window would miss.
func TestGitObservationLeavesOutBranchesThatNeedNoPR(t *testing.T) {
	const (
		listed     = "karakuri/phase-40-step-06-branch-gets-pr"
		recentPR   = "karakuri/ux-2026-11-15"
		oldPR      = "karakuri/phase-37-step-01-old-open-pr"
		notAhead   = "karakuri/discovery-2026-11-01"
		foreign    = "feature/someone-elses-work"
		salvage    = "karakuri/salvage/phase-44-step-02"
		mainBranch = "main"
	)
	now := time.Now().UTC()
	forge := &branchForge{
		vcStub: vcStub{
			branches: []versioncontrol.Branch{
				{Name: listed, SHA: "a1"}, {Name: recentPR, SHA: "a2"}, {Name: oldPR, SHA: "a3"},
				{Name: notAhead, SHA: "a4"}, {Name: foreign, SHA: "a5"}, {Name: salvage, SHA: "a6"},
				{Name: mainBranch, SHA: "a7"},
			},
			prs: []versioncontrol.PRSummary{
				{ID: "300", Number: 300, HeadBranch: recentPR, BaseBranch: "main", OwnAuthor: true,
					CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now.Add(-time.Hour)},
				{ID: "120", Number: 120, HeadBranch: oldPR, BaseBranch: "main", OwnAuthor: true,
					CreatedAt: now.Add(-60 * 24 * time.Hour), UpdatedAt: now.Add(-30 * 24 * time.Hour)},
			},
		},
		ahead: map[string][]versioncontrol.Commit{
			listed:   aheadBy(1),
			recentPR: aheadBy(3),
			oldPR:    aheadBy(4),
			notAhead: nil,
			foreign:  aheadBy(5),
			salvage:  aheadBy(1),
		},
	}

	obs := observeGit(t, forge)

	got := branchesWithoutPR(t, obs.State)
	if len(got) != 1 || got[0].Branch != listed {
		t.Fatalf("branches_without_pr = %+v, want only %s", got, listed)
	}
	for _, l := range branchLines(t, obs.State) {
		for _, name := range []string{recentPR, oldPR, notAhead, foreign, salvage} {
			if strings.Contains(l, name) {
				t.Errorf("planner line names %s, which needs no pull request: %q", name, l)
			}
		}
	}

	// The old open pull request is found by asking the forge about its branch,
	// not by a list bounded to the observation window, which would miss it.
	// The git observation still lists open pull requests for the window for
	// other fields, so ListPRs' since says nothing about this one.
	if !slices.Contains(forge.headAsks, oldPR) {
		t.Errorf("the forge was not asked whether %s ever had a pull request; asked about %q", oldPR, forge.headAsks)
	}
}

// With no branch to report the field is an empty list, not missing: missing is
// reserved for "could not read".
func TestGitObservationReportsNoBranchesAsAnEmptyList(t *testing.T) {
	forge := &branchForge{
		vcStub: vcStub{branches: []versioncontrol.Branch{{Name: "karakuri/ux-2026-11-16", SHA: "a1"}}},
		ahead:  map[string][]versioncontrol.Commit{},
	}

	obs := observeGit(t, forge)

	if got := branchesWithoutPR(t, obs.State); len(got) != 0 {
		t.Fatalf("branches_without_pr = %+v, want empty", got)
	}
	if _, ok := obs.State["branches_without_pr_error"]; ok {
		t.Errorf("branches_without_pr_error set on a remote that was read: %v", obs.State["branches_without_pr_error"])
	}
}

// When the branches or the open pull requests cannot be listed, the
// observation does not say "no branches": it carries the error, has no list,
// and tells the planner it could not see.
func TestGitObservationSaysItCouldNotSeeBranchesWithoutPR(t *testing.T) {
	unreachable := errors.New("github: dial tcp: connection refused")
	cases := map[string]*branchForge{
		"branches cannot be listed":      {branchErr: unreachable},
		"pull requests cannot be listed": {prsErr: unreachable},
		"branch listing unsupported":     {branchErr: versioncontrol.ErrUnsupported},
	}
	for name, forge := range cases {
		t.Run(name, func(t *testing.T) {
			forge.branches = []versioncontrol.Branch{{Name: "karakuri/phase-40-step-06-x", SHA: "a1"}}
			forge.ahead = map[string][]versioncontrol.Commit{"karakuri/phase-40-step-06-x": aheadBy(1)}

			obs := observeGit(t, forge)

			if raw, ok := obs.State["branches_without_pr"]; ok {
				t.Errorf("branches_without_pr = %v on a remote that could not be read; want no list", raw)
			}
			msg, _ := obs.State["branches_without_pr_error"].(string)
			if msg == "" {
				t.Fatalf("branches_without_pr_error missing; state keys: %v", stateKeys(obs.State))
			}
			wantErr := unreachable.Error()
			if forge.branchErr != nil {
				wantErr = forge.branchErr.Error()
			}
			if !strings.Contains(msg, wantErr) {
				t.Errorf("branches_without_pr_error = %q, want it to carry %q", msg, wantErr)
			}

			lines := branchLines(t, obs.State)
			saidSo := false
			for _, l := range lines {
				if strings.Contains(strings.ToLower(l), "could not") {
					saidSo = true
				}
				if strings.Contains(l, "karakuri/phase-40-step-06-x") {
					t.Errorf("planner line lists a branch the observation could not check: %q", l)
				}
			}
			if !saidSo {
				t.Errorf("no planner line says the branches could not be read; lines: %q", lines)
			}
		})
	}
}
