package software

import "github.com/bsenel/karakuri/internal/core/domain"

func softwarePlannerHints() []domain.PlannerHint {
	return []domain.PlannerHint{
		{
			Condition: "objective.template == 'software.objective.delivery'",
			Guidance:  "write_design_doc must precede any write_code action",
			Priority:  10,
		},
		{
			Condition: "objective.template == 'software.objective.delivery'",
			Guidance:  "write_test must precede the write_code action it covers (TDD)",
			Priority:  9,
		},
		{
			Condition: "objective.template == 'software.objective.delivery'",
			Guidance:  "all write_code actions run in isolated worktrees",
			Priority:  8,
		},
		{
			Condition: "objective.template == 'software.objective.delivery'",
			Guidance:  "verify.tech_lead_review and verify.review must both pass before create_pr",
			Priority:  9,
		},
		{
			// This used to be the load-bearing routing rule for anything that
			// writes: stepAct sent an action to whatever environment its
			// env_id named, so a plan that wrote code without naming the CLI
			// environment reached noopEnv and failed as unimplemented — and a
			// hint is guidance, not a guarantee.
			//
			// Routing is now the registry's, from Factory.Serves (ADR 019), so
			// the env_id half of this is no longer a warning about how to
			// avoid a failure. What remains is the part a model genuinely has
			// to get right: which parameters to fill in, and where not to
			// write.
			Condition: "capability.id in ['software.act.write_code', 'software.act.write_test', 'software.act.delegate_to_cli']",
			Guidance: "put the task in params.prompt. The worktree is provisioned for you and arrives in " +
				"params.worktree_path; never write to the checked-out tree. Routing is automatic — " +
				"env_id is not needed for these.",
			Priority: 10,
		},
		{
			Condition: "capability.id startswith 'software.reason.research'",
			Guidance:  "prefer Gemini provider for research capabilities",
			Priority:  5,
		},
		{
			Condition: "capability.id startswith 'software.act.write'",
			Guidance:  "prefer Cursor provider for implementation actions",
			Priority:  5,
		},
	}
}
