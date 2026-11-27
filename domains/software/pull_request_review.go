package software

// Phase 40 step 11: a review that builds and runs the branch.
//
// The reviewer is its own agent, so it is never the context that wrote the
// change. What it may do is written into the agent's bounds and the template's
// limits (ADR 015), not into a second gate: build, test, lint, run a command
// in its worktree, and post one review. Every capability that pushes or
// changes the repository is held to an empty limit, which no value satisfies.

import (
	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/objective"
)

const (
	pullRequestReviewTemplateID = "software.objective.pull_request_review"
	pullRequestReviewAgentID    = "software.agent.pr_reviewer"
)

// reviewShellCommands are the only commands the reviewer's shell may run:
// build, test, lint and the checks, each exactly. None pushes, commits,
// switches branch or reaches the network, and no pattern carries a wildcard,
// which would also admit `go test ./... ; git push`.
var reviewShellCommands = []string{
	"go build ./...", "go test ./...", "go vet ./...", "gofmt -l .",
	"make build", "make test", "make lint",
	"scripts/check_langchaingo_imports.sh",
	// A binary the pull request changes, each exactly: a wildcard here would
	// also admit `go run ./cmd/krk x; git push`.
	"go run ./cmd/krk bogus", "go run ./cmd/krk --help",
}

// reviewRefusedCapabilities push or change the repository. A limit with no
// pattern admits no value, and an action that leaves the parameter out
// violates it as well.
var reviewRefusedCapabilities = []capability.CapabilityID{
	"software.act.write_code",
	"software.act.write_test",
	"software.act.delegate_to_cli",
	"software.act.create_pr",
	CapMarkReady,
}

func pullRequestReviewAgent() agent.Definition {
	return agent.Definition{
		ID:     pullRequestReviewAgentID,
		Name:   "Pull Request Reviewer",
		Domain: "software",
		Capabilities: []capability.CapabilityID{
			"software.verify.run_tests", "software.verify.lint", "software.act.shell_exec", CapPostReview,
		},
		ReasoningStrategy: agent.ReasoningReflexion,
		Authority: agent.AuthorityBounds{
			MaxAutonomousActions: 10,
			CanDelegate:          false,
			CanModifyObjective:   false,
			ConfidenceThreshold:  0.8,
			ActionLimits:         reviewActionLimits(),
		},
		Memory: agent.MemoryConfig{SemanticEnabled: true},
	}
}

func reviewActionLimits() []agent.ActionLimit {
	limits := []agent.ActionLimit{
		{Capability: "software.act.shell_exec", Parameter: "cmd", Patterns: append([]string{}, reviewShellCommands...)},
	}
	for _, id := range reviewRefusedCapabilities {
		limits = append(limits,
			agent.ActionLimit{Capability: id, Parameter: "branch"},
			// A tool list is where a coding action holds a merge tool; an
			// empty limit admits no value, so no list and no tool in one.
			agent.ActionLimit{Capability: id, Parameter: "allowed_tools"})
	}
	return limits
}

func pullRequestReviewTemplate() objective.Template {
	noMerge := objective.Constraint{ID: "no-merge", Description: "A human merges; this objective never merges a pull request or pushes to main", Hard: true, Expression: "no_merge"}
	// Names the observation that wakes the pass, because a planner that is not
	// told where the unreviewed pull requests are has nothing to review.
	unreviewed := objective.Constraint{ID: "prs-without-review", Description: "Review the pull requests listed in the git observation's prs_without_review (their head has no review), one software.act.post_review each; " +
		"when prs_without_review_error is set the list could not be read, so say that and review nothing rather than treat it as empty", Hard: true, Expression: "prs_without_review"}
	return objective.Template{
		ID:     pullRequestReviewTemplateID,
		Title:  "Review a pull request by building and running it",
		Domain: "software",
		// It posts a comment and changes nothing.
		Risk:            objective.RiskRoutine,
		SuggestedAgents: []agent.Definition{{ID: pullRequestReviewAgentID}},
		Description: "Review the open pull request by checking out its branch in a worktree, building it, running its tests and lint, " +
			"and running the binaries it changes, then post one software.act.post_review comment review on it. " +
			"The review states what it ran and what it did not, with the result of each. " +
			"It lists what it cannot verify: a dependency that is not installed, a web page it cannot render, " +
			"and anything where a decision or a proposal is wanted rather than a fact. " +
			"On a forge where author and reviewer are one account the review is a comment review; approval stays the owner's merge. " +
			"The reviewer may push nothing: no commit, no branch, no pull request, no ready mark, and no merge.",
		SuccessCriteria: []objective.Criterion{
			progress("states-ran", "The review says what it ran and what it did not, and lists what it cannot verify", 1.0),
		},
		Constraints:  []objective.Constraint{noMerge, unreviewed},
		ActionLimits: reviewActionLimits(),
	}
}
