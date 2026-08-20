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
			// The routing hint, and the load-bearing one for anything that
			// writes. stepAct sends an action to the environment its env_id
			// names, so a plan that writes code without naming the CLI
			// environment reaches noopEnv and fails as unimplemented.
			Condition: "capability.id in ['software.act.write_code', 'software.act.write_test', 'software.act.delegate_to_cli']",
			Guidance: "set env_id to 'software.env.cli_agent' and put the task in params.prompt. " +
				"The worktree is provisioned for you and arrives in params.worktree_path; never write to the checked-out tree.",
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
