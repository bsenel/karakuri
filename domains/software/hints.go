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
			// The deployment's own data — audit log, checkpoints, cost,
			// reconcile status — is served as MCP tools a delegated agent can
			// be handed for one action (Phase 34). This hint is the only place
			// a model learns the field exists; without it a plan writes
			// "run `krk audit list`" into the prompt and the agent shells out
			// with whatever credential is lying around.
			//
			// cliEnv refuses an action that asks when the instance has not
			// opted in, so the hint says so rather than "always ask". The
			// names are the ones internal/api/handler/mcp.go serves.
			Condition: "capability.id in ['software.act.write_code', 'software.act.write_test', 'software.act.delegate_to_cli']",
			Guidance: "when the task needs this deployment's own data, list the Karakuri MCP tools it needs in " +
				"params.karakuri_tools instead of telling the agent to shell out to `krk`. The tools are read-only: " +
				"objectives_list, objective_read, digest_read, reconcile_status, telemetry_read, audit_list, " +
				"audit_export, checkpoints_list, checkpoint_read, cost_report. They are attached only when the " +
				"cli_agents instance sets attach_karakuri_mcp and karakuri_mcp_url; an action that asks without " +
				"them is refused, not run without the tools, so leave the field out when the task does not need the data.",
			Priority: 7,
		},
		{
			Condition: "objective.template == 'software.objective.incident_response'",
			Guidance: "fetch evidence first (observe.fetch_logs / observe.fetch_metrics), name the observed alert_id " +
				"in software.act.run_remediation, and end with software.verify.alerts_resolved for the same alert_ids",
			Priority: 9,
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
