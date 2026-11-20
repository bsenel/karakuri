package software

// A stream's branch with commits and no pull request (Phase 40 step 6).
//
// A pass that pushed its branch and was killed before it opened the pull
// request leaves work nobody reviews. The git observation lists each such
// branch so the stream's next pass opens that pull request before new work.

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// streamBranchPrefix is what every stream's branch starts with; the forge
// filters on it, so one read covers all four streams.
const streamBranchPrefix = "karakuri/"

// streamBranchPatterns map each stream's template to its own branches, the
// same patterns its action limits hold a coding action to.
var streamBranchPatterns = []struct {
	stream   string
	patterns []string
}{
	{marketDiscoveryTemplateID, discoveryBranches},
	{engineeringBacklogTemplateID, backlogBranches},
	{uxImprovementTemplateID, uxBranches},
	{roadmapDeliveryTemplateID, deliveryBranches},
}

// branchStream is the template of the stream whose patterns match branch, or
// "" for a branch no stream owns (a salvage branch, main, a human's branch).
func branchStream(branch string) string {
	for _, s := range streamBranchPatterns {
		for _, p := range s.patterns {
			if ok, err := path.Match(p, branch); err == nil && ok {
				return s.stream
			}
		}
	}
	return ""
}

// observeBranchesWithoutPR writes branches_without_pr and its planner lines
// into state. A branch counts as having a pull request when one in any state
// names it, since compare main...head still counts a squash-merged branch as
// ahead. A forge that cannot list the branches or say whether one had a pull
// request leaves the list out and says why (AGENTS.md rule 10): an empty list
// would tell the planner there is nothing to open.
func observeBranchesWithoutPR(ctx context.Context, adapter versioncontrol.VersionControlAdapter, repo string, state map[string]any) {
	blind := func(err error) {
		state["branches_without_pr_error"] = err.Error()
		state["branches_without_pr_lines"] = []string{
			"The stream branches with no pull request could not be read: " + err.Error() +
				". Do not take this as there being none.",
		}
	}
	branches, err := adapter.ListBranches(ctx, repo, streamBranchPrefix)
	if err != nil {
		blind(err)
		return
	}
	listed := []map[string]any{}
	lines := []string{}
	for _, b := range branches {
		// A salvage branch keeps a killed action's work for a human; it is no
		// stream's work to open a pull request for.
		if strings.Contains(b.Name, "-salvage") {
			continue
		}
		stream := branchStream(b.Name)
		if stream == "" {
			continue
		}
		ahead, err := adapter.CompareCommits(ctx, repo, "main", b.Name)
		if err != nil {
			// Not listed, since it may hold nothing, but not dropped quietly:
			// the planner is told this one branch could not be checked.
			lines = append(lines, fmt.Sprintf("Branch %s (%s) could not be compared with main: %v.", b.Name, stream, err))
			continue
		}
		if len(ahead) == 0 {
			continue
		}
		hadPR, err := adapter.HasPullRequestForHead(ctx, repo, b.Name)
		if err != nil {
			blind(err)
			return
		}
		if hadPR {
			continue
		}
		listed = append(listed, map[string]any{
			"branch": b.Name, "sha": b.SHA, "stream": stream, "ahead": len(ahead),
		})
		lines = append(lines, fmt.Sprintf(
			"Branch %s of stream %s is %d commit(s) ahead of main at %s and has never had a pull request: open its pull request before any other work.",
			b.Name, stream, len(ahead), shortSHA(b.SHA)))
	}
	state["branches_without_pr"] = listed
	state["branches_without_pr_lines"] = lines
}

// shortSHA is a SHA as a planner line names it.
func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
