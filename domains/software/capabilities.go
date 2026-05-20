package software

import "github.com/bsenel/karakuri/internal/core/capability"

func softwareCapabilities() []capability.Capability {
	obs := func(id, name, desc string) capability.Capability {
		return capability.Capability{
			ID: capability.CapabilityID(id), Name: name, Domain: "software",
			Description: desc,
			InputSchema:  capability.Schema{Type: "object", Properties: map[string]capability.SchemaProperty{}},
			OutputSchema: capability.Schema{Type: "object"},
		}
	}
	act := func(id, name, desc string, verifiable bool) capability.Capability {
		return capability.Capability{
			ID: capability.CapabilityID(id), Name: name, Domain: "software",
			Description: desc, Verifiable: verifiable,
			InputSchema:  capability.Schema{Type: "object", Properties: map[string]capability.SchemaProperty{}},
			OutputSchema: capability.Schema{Type: "object"},
		}
	}
	return []capability.Capability{
		obs("software.observe.fetch_commits", "Fetch Commits", "Fetch recent commits from GitEnvironment"),
		obs("software.observe.fetch_prs", "Fetch PRs", "Fetch pull requests awaiting review"),
		obs("software.observe.fetch_logs", "Fetch Logs", "Fetch runtime logs from ObservabilityEnvironment"),
		obs("software.observe.fetch_metrics", "Fetch Metrics", "Fetch runtime metrics"),
		obs("software.observe.read_codebase", "Read Codebase", "Read file tree, symbols, and dependencies"),

		act("software.reason.architecture_review", "Architecture Review", "Evaluate a design against architectural principles", false),
		act("software.reason.research", "Research", "Research a topic across configured sources", false),

		act("software.decide.prioritize_tasks", "Prioritize Tasks", "Order a task list by impact and risk", false),

		act("software.act.write_code", "Write Code", "Produce implementation artifact in worktree", false),
		act("software.act.write_test", "Write Test", "Produce test artifact in worktree (TDD)", false),
		act("software.act.write_design_doc", "Write Design Doc", "Produce mandatory design document before implementation", false),
		act("software.act.create_pr", "Create PR", "Submit worktree branch as pull request", false),
		act("software.act.create_ticket", "Create Ticket", "Create ticket in project management tool", false),
		act("software.act.send_message", "Send Message", "Send a message via MessagingAdapter", false),
		act("software.act.delegate_to_cli", "Delegate to CLI Agent", "Hand a task to a coding-agent CLI (Claude Code, Cursor, Gemini, Copilot) in the active worktree", false),

		act("software.verify.run_tests", "Run Tests", "Execute test suite in worktree", true),
		act("software.verify.lint", "Lint", "Run linter in worktree", true),
		act("software.verify.review", "Code Review", "Peer review of an artifact", true),
		act("software.verify.tech_lead_review", "Tech Lead Review", "Senior review of an artifact against design doc", true),

		act("software.learn.extract_patterns", "Extract Patterns", "Extract reusable patterns from completed objective", false),
		act("software.learn.update_tech_radar", "Update Tech Radar", "Update the team's technology assessment", false),
	}
}
