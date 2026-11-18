package software

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 40 step 6, the review Blocker on #259: a branch counts as having a
// pull request when the forge has, or ever had, one in any state whose head is
// that branch, not only an open one. compare main...head still counts a
// squash-merged branch as ahead, so the open list alone would have the streams
// open a second pull request for work already on main. A salvage branch is not
// a stream's work and is not listed either.

// neverHadPR is a stream branch with commits and no pull request in any state:
// the one kind of branch the observation lists.
const neverHadPR = "karakuri/phase-40-step-06-branch-gets-pr"

// forgeWith holds neverHadPR and the given branches, each ahead of main, with
// hadPR naming those that have or had a pull request.
func forgeWith(hadPR map[string]bool, branches ...string) *branchForge {
	forge := &branchForge{
		ahead: map[string][]versioncontrol.Commit{neverHadPR: aheadBy(1)},
		hadPR: hadPR,
	}
	forge.branches = []versioncontrol.Branch{{Name: neverHadPR, SHA: "tip-never"}}
	for i, name := range branches {
		forge.branches = append(forge.branches, versioncontrol.Branch{Name: name, SHA: fmt.Sprintf("tip-%d", i)})
		forge.ahead[name] = aheadBy(3)
	}
	return forge
}

// assertOnlyNeverHadPRListed fails unless neverHadPR is the one branch listed
// and no planner line names any of left.
func assertOnlyNeverHadPRListed(t *testing.T, forge *branchForge, left ...string) {
	t.Helper()
	obs := observeGit(t, forge)
	if msg, ok := obs.State["branches_without_pr_error"]; ok {
		t.Fatalf("branches_without_pr_error = %v on a forge that answered", msg)
	}
	got := branchesWithoutPR(t, obs.State)
	if len(got) != 1 || got[0].Branch != neverHadPR {
		t.Errorf("branches_without_pr = %+v, want only %s", got, neverHadPR)
	}
	for _, l := range branchLines(t, obs.State) {
		for _, name := range left {
			if strings.Contains(l, name) {
				t.Errorf("planner line names %s, which needs no pull request: %q", name, l)
			}
		}
	}
}

// A squash-merged branch: its pull request was merged, the branch was never
// deleted, and compare main...head still counts its commits as ahead because a
// squash leaves them off main's history.
func TestGitObservationLeavesOutASquashMergedBranch(t *testing.T) {
	const merged = "karakuri/phase-40-roadmap"
	forge := forgeWith(map[string]bool{merged: true}, merged)

	assertOnlyNeverHadPRListed(t, forge, merged)

	if !slices.Contains(forge.headAsks, merged) {
		t.Errorf("the forge was not asked whether %s ever had a pull request; asked about %q", merged, forge.headAsks)
	}
}

// A branch whose pull request was closed without merging was looked at by
// somebody and turned down; opening another for it is not the stream's call.
func TestGitObservationLeavesOutABranchWhosePRWasClosedUnmerged(t *testing.T) {
	const closed = "karakuri/discovery-2026-10-25"
	forge := forgeWith(map[string]bool{closed: true}, closed)

	assertOnlyNeverHadPRListed(t, forge, closed)
}

// A salvage branch keeps a killed action's work for a human to look at. It
// matches the delivery stream's karakuri/phase-* pattern and never had a pull
// request, yet it is no stream's work to open one for.
func TestGitObservationLeavesOutSalvageBranches(t *testing.T) {
	salvage := []string{
		"karakuri/phase-36-salvage",
		"karakuri/phase-36-salvage-2",
		"karakuri/phase-41-salvage-step1",
	}
	forge := forgeWith(nil, salvage...)

	assertOnlyNeverHadPRListed(t, forge, salvage...)
}

// The fix leaves out branches that had a pull request; it does not leave out
// the one the step exists for: a stream branch pushed by a pass killed before
// it opened its pull request.
func TestGitObservationStillListsAStreamBranchThatNeverHadAPR(t *testing.T) {
	const merged = "karakuri/phase-40-blockers"
	forge := forgeWith(map[string]bool{merged: true}, merged)

	obs := observeGit(t, forge)

	got := branchesWithoutPR(t, obs.State)
	if len(got) != 1 || got[0].Branch != neverHadPR {
		t.Fatalf("branches_without_pr = %+v, want %s", got, neverHadPR)
	}
	if got[0].Stream != roadmapDeliveryTemplateID || got[0].SHA != "tip-never" || got[0].Ahead != 1 {
		t.Errorf("listed %+v, want stream %s, sha tip-never, ahead 1", got[0], roadmapDeliveryTemplateID)
	}
	named := false
	for _, l := range branchLines(t, obs.State) {
		if strings.Contains(l, neverHadPR) {
			named = true
		}
	}
	if !named {
		t.Errorf("no planner line names %s", neverHadPR)
	}
	if !slices.Contains(forge.headAsks, neverHadPR) {
		t.Errorf("the forge was not asked whether %s ever had a pull request; asked about %q", neverHadPR, forge.headAsks)
	}
}

// When the forge cannot say whether a branch had a pull request, the
// observation is blind, not empty: listing the branch could open a duplicate
// of merged work, and leaving it out would tell the planner there is nothing
// to open (AGENTS.md rule 10).
func TestGitObservationIsBlindWhenABranchsPRHistoryCannotBeRead(t *testing.T) {
	cases := map[string]error{
		"plain error": errors.New("github: GET /repos/acme/api/pulls -> 502: bad gateway"),
		"unsupported": fmt.Errorf("fake_forge: pull requests for head: %w", versioncontrol.ErrUnsupported),
	}
	for name, headErr := range cases {
		t.Run(name, func(t *testing.T) {
			forge := forgeWith(nil)
			forge.headErr = map[string]error{neverHadPR: headErr}

			obs := observeGit(t, forge)

			if raw, ok := obs.State["branches_without_pr"]; ok {
				t.Errorf("branches_without_pr = %v when a branch's pull requests could not be read; want no list", raw)
			}
			msg, _ := obs.State["branches_without_pr_error"].(string)
			if msg == "" {
				t.Fatalf("branches_without_pr_error missing; state keys: %v", stateKeys(obs.State))
			}
			if !strings.Contains(msg, headErr.Error()) {
				t.Errorf("branches_without_pr_error = %q, want it to carry %q", msg, headErr.Error())
			}
			saidSo := false
			for _, l := range branchLines(t, obs.State) {
				if strings.Contains(strings.ToLower(l), "could not") {
					saidSo = true
				}
			}
			if !saidSo {
				t.Errorf("no planner line says the branches could not be read")
			}
		})
	}
}
