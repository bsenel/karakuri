package software

// What a stream's actions may hold (Phase 40 step 2).
//
// The no_merge constraint is a sentence the planner reads, and nothing
// evaluates it. These limits are what the decide step checks: each coding
// action's tool allow-list may hold only the tools the stream's brief gives
// it, so no merge tool and no unrestricted shell, and each coding action and
// pull request names a branch of the stream's own. At the rungs that act, a
// plan outside them is refused with a bounds row in the audit log and no
// checkpoint.

import (
	"strings"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// streamCodingCapabilities hand a coding agent a tool allow-list in
// params.allowed_tools. delegate_to_cli is among them because it runs the same
// agent: leaving it open would route a merge around the write_code limit.
var streamCodingCapabilities = []capability.CapabilityID{
	"software.act.write_code",
	"software.act.write_test",
	"software.act.delegate_to_cli",
}

// streamBaseTools are the tools every stream's brief gives its coding agent:
// reading and editing files, git, and the gh subcommands that open and edit a
// pull request. No gh pattern here is wide enough to hold `gh pr merge`.
var streamBaseTools = []string{
	"Read", "Edit", "Write", "Glob", "Grep",
	"Bash(git:*)",
	"Bash(gh pr list:*)", "Bash(gh pr view:*)", "Bash(gh pr create:*)", "Bash(gh pr edit:*)",
	"Bash(gh label create:*)",
	"Bash(ls:*)", "Bash(cat:*)", "Bash(grep:*)", "Bash(find:*)", "Bash(date:*)",
}

// krkReadCommands are the read-only krk subcommands the engineering backlog's
// brief lets its agent run. None of them changes anything: no resolve, no
// create, no stop.
var krkReadCommands = []string{
	"audit", "cost report", "eval",
	"objective list", "objective get", "objective reconcile-status",
	"checkpoint list", "checkpoint get",
	"twin list", "twin get", "domain list",
}

// krkToolPatterns admit `Bash(<path>/bin/krk <read command>:*)`, with or
// without `--output json`, for any absolute checkout of up to eight
// directories. The brief names krk by its path on the deployment, which a pack
// cannot know, and path.Match's * does not cross '/', so each depth is its own
// pattern.
func krkToolPatterns() []string {
	var out []string
	for depth := 1; depth <= 8; depth++ {
		bin := "/" + strings.Repeat("*/", depth) + "bin/krk "
		for _, cmd := range krkReadCommands {
			for _, flag := range []string{"", "--output json "} {
				out = append(out, "Bash("+bin+flag+cmd+":\\*)")
			}
		}
	}
	return out
}

// exactly turns tool entries into patterns that match only themselves. The
// entries carry a literal `*` (`Bash(git:*)`), which as a pattern would also
// admit `Bash(git:x) && gh pr merge 1)`.
func exactly(tools []string) []string {
	out := make([]string, len(tools))
	for i, t := range tools {
		out[i] = strings.ReplaceAll(t, "*", "\\*")
	}
	return out
}

// streamLimits are one stream's action limits: tools is the allow-list a
// coding action may hold, as patterns, and branches are the patterns of the
// stream's own branches. Every coding action pushes to one of them; a pull
// request opens from one, onto main or onto one (delivery stacks a later group
// on an earlier group's head). main is never a branch a coding action names.
func streamLimits(tools, branches []string) []agent.ActionLimit {
	out := make([]agent.ActionLimit, 0, 2*len(streamCodingCapabilities)+2)
	for _, id := range streamCodingCapabilities {
		out = append(out,
			agent.ActionLimit{Capability: id, Parameter: "allowed_tools", Patterns: tools},
			agent.ActionLimit{Capability: id, Parameter: "branch", Patterns: branches},
		)
	}
	return append(out,
		agent.ActionLimit{Capability: "software.act.create_pr", Parameter: "branch", Patterns: branches},
		agent.ActionLimit{Capability: "software.act.create_pr", Parameter: "base_branch", Patterns: append([]string{"main"}, branches...)},
	)
}

// branchConstraint tells the planner what the limits require of a plan. The
// planner reads the objective's constraints, which come from the template, and
// a coding action that leaves params.branch or params.allowed_tools out is
// refused for it, so the sentence has to say so.
func branchConstraint(branch, base string) objective.Constraint {
	return objective.Constraint{
		ID: "action-limits",
		Description: "Every software.act.write_code, software.act.write_test and software.act.delegate_to_cli action sets params.branch to " +
			branch + " and lists its tools in params.allowed_tools from this objective's allowed tools; " +
			"software.act.create_pr sets params.branch to that branch and params.base_branch to " + base + ". " +
			"An action that leaves either out, names main or another stream's branch, or holds a merge tool or an unrestricted shell is refused.",
		Hard: true,
	}
}

// branchesWithoutPRConstraint tells every stream's planner to open its own
// branches that the git observation lists without a pull request before any
// new work, so a pass that pushed and then died does not strand its branch
// (Phase 40 step 6). The action it names is the one branchConstraint already
// holds to the stream's branches, so the limits admit it. One constraint for
// all four streams: the rule does not depend on which stream reads it.
var branchesWithoutPRConstraint = objective.Constraint{
	ID: "branches-without-pr",
	Description: "The git observation's branches_without_pr lists branches ahead of main with no open pull request, each with its stream. " +
		"The plan opens a pull request for each branch of this stream it lists, first, before any other new work: " +
		"one software.act.create_pr action with params.branch set to the listed branch and params.base_branch set as the action limits require. " +
		"When branches_without_pr_error is set instead, the pass reports that it could not see the branches and does not treat the list as empty.",
	Hard: true,
}

// withBase is the base tools and a stream's own, in a fresh slice.
func withBase(tools ...string) []string {
	return append(append([]string{}, streamBaseTools...), tools...)
}

// The paths a proposal stream's pass may write (ADR 030). The same globs are
// its paths criterion's threshold, so the decide step, the check after a
// push and the check on the pull request hold a pass to one list.
var (
	discoveryPaths = []string{"docs/research/**", "docs/roadmap.md"}
	backlogPaths   = []string{"docs/roadmap.md"}
)

// proposalLimits narrow a stream's limits for one that acts as the proposer
// (Phase 40 step 10). Every coding action lists the files it writes in
// params.paths, each inside paths, and the check after the run fails one that
// pushed a change to any other file (checkPushedPaths); a pull request opens
// only against main. Both are added beside streamLimits' own limits, which
// they intersect, never replace. A path.Match pattern does not cross '/', so
// docs/research/** admits a declared file one level under docs/research and
// no deeper: narrower than the criterion's glob, which is the safe side.
func proposalLimits(tools, branches, paths []string) []agent.ActionLimit {
	out := streamLimits(tools, branches)
	for _, id := range streamCodingCapabilities {
		out = append(out, agent.ActionLimit{Capability: id, Parameter: "paths", Patterns: paths})
	}
	return append(out, agent.ActionLimit{Capability: "software.act.create_pr", Parameter: "base_branch", Patterns: []string{"main"}})
}

// pathsConstraint tells the planner what proposalLimits require of a plan.
func pathsConstraint(paths []string) objective.Constraint {
	return objective.Constraint{
		ID: "action-paths",
		Description: "Every software.act.write_code, software.act.write_test and software.act.delegate_to_cli action lists " +
			"the files it writes in params.paths, each inside " + strings.Join(paths, ", ") + ", " +
			"and software.act.create_pr sets params.base_branch to main. " +
			"An action that leaves params.paths out or lists another path is refused, and one that pushed a change to another file fails.",
		Hard: true,
	}
}

// Each stream's own branches, as path.Match patterns. The limits hold a coding
// action to them, and the git observation reads them to find a stream's branch
// that has no pull request (Phase 40 step 6). Delivery's are a phase's groups,
// karakuri/phase-<n>-..., and a backlog entry's branch, karakuri/eb-<id>-....
var (
	discoveryBranches = []string{"karakuri/discovery-*"}
	backlogBranches   = []string{"karakuri/tech-backlog-*"}
	uxBranches        = []string{"karakuri/ux-*"}
	deliveryBranches  = []string{"karakuri/phase-*", "karakuri/eb-*"}
)

func discoveryLimits() []agent.ActionLimit {
	return proposalLimits(exactly(withBase("WebSearch", "WebFetch")), discoveryBranches, discoveryPaths)
}

func backlogLimits() []agent.ActionLimit {
	tools := exactly(withBase("Bash(tail:*)", "Bash(wc:*)", "Bash(sort:*)", "Bash(uniq:*)", "Bash(head:*)"))
	return proposalLimits(append(tools, krkToolPatterns()...), backlogBranches, backlogPaths)
}

func uxLimits() []agent.ActionLimit {
	return streamLimits(exactly(withBase("Bash(gh pr checks:*)", "Bash(gh run view:*)", "Bash(go:*)", "Bash(gofmt:*)", "Bash(sed:*)")),
		uxBranches)
}

func deliveryLimits() []agent.ActionLimit {
	return streamLimits(exactly(withBase(
		"Bash(gh pr comment:*)", "Bash(gh pr ready:*)", "Bash(gh pr checks:*)", "Bash(gh run view:*)",
		"Bash(go:*)", "Bash(gofmt:*)", "Bash(scripts/check_langchaingo_imports.sh:*)", "Bash(sed:*)")),
		deliveryBranches)
}
