package software

import "github.com/bsenel/karakuri/internal/core/capability"

func softwareCapabilities() []capability.Capability {
	obs := func(id, name, desc string) capability.Capability {
		return capability.Capability{
			ID: capability.CapabilityID(id), Name: name, Domain: "software",
			Description:  desc,
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
	// writes marks a capability that produces files and therefore needs an
	// isolated worktree. Declared here so the loop provisions one by what the
	// capability says it does rather than by what it is called.
	writes := func(c capability.Capability) capability.Capability {
		c.NeedsWorkspace = true
		return c
	}
	return []capability.Capability{
		// Inputs declared for the reason write_design_doc's are below. Neither
		// read requires an input; both fail rather than return an empty list
		// when the twin has no version control instance bound.
		{
			ID: "software.observe.fetch_commits", Name: "Fetch Commits", Domain: "software",
			Description: "Fetch recent commits from the twin's bound version control instance. Optional params.repo and params.since_days (default 7, max 90). Fails when no version control instance is bound. Result includes commits and count.",
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"repo":       {Type: "string", Description: "Repository to read. Defaults to the bound instance's repository"},
					"since_days": {Type: "integer", Description: "How far back to look, in days. Default 7, max 90"},
				},
			},
			OutputSchema: capability.Schema{Type: "object"},
		},
		{
			ID: "software.observe.fetch_prs", Name: "Fetch PRs", Domain: "software",
			Description: "Fetch pull requests from the twin's bound version control instance. Optional params.repo and params.since_days (default 7, max 90). Fails when no version control instance is bound. Result includes prs and count.",
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"repo":       {Type: "string", Description: "Repository to read. Defaults to the bound instance's repository"},
					"since_days": {Type: "integer", Description: "How far back to look, in days. Default 7, max 90"},
				},
			},
			OutputSchema: capability.Schema{Type: "object"},
		},
		// Inputs declared for the same reason write_design_doc's are below: a
		// capability whose inputs are undocumented is one models call with an
		// empty payload, and both of these refuse an empty payload.
		{
			ID: CapFetchLogs, Name: "Fetch Logs", Domain: "software",
			Description: "Fetch runtime logs from the twin's bound observability instance. Requires at least one of params.query and params.service. Optional params.since_minutes (default 60, max 1440) and params.limit (default 200). Result includes lines (time, service, message) and count.",
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"query":         {Type: "string", Description: "Text or backend query to match log lines against. Required unless service is given"},
					"service":       {Type: "string", Description: "Service whose logs to fetch. Required unless query is given"},
					"since_minutes": {Type: "integer", Description: "How far back to look, in minutes. Default 60, max 1440"},
					"limit":         {Type: "integer", Description: "Maximum number of lines to return. Default 200"},
				},
			},
			OutputSchema: capability.Schema{Type: "object"},
		},
		{
			ID: CapFetchMetrics, Name: "Fetch Metrics", Domain: "software",
			Description: "Fetch runtime metrics from the twin's bound observability instance. Requires params.query (in the backend's query language). Optional params.since_minutes (default 60, max 1440) and params.step_seconds (default 60). Result includes series (name, labels, points) and count.",
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"query":         {Type: "string", Description: "The metric query, in the bound backend's query language"},
					"since_minutes": {Type: "integer", Description: "How far back to look, in minutes. Default 60, max 1440"},
					"step_seconds":  {Type: "integer", Description: "Resolution of the returned points, in seconds. Default 60"},
				},
				Required: []string{"query"},
			},
			OutputSchema: capability.Schema{Type: "object"},
		},
		obs("software.observe.read_codebase", "Read Codebase", "Read the repository as evidence: the roadmap's deferred work, TODO density by package, packages with no tests, and where AGENTS.md rules live. Takes no params."),

		act("software.reason.architecture_review", "Architecture Review", "Evaluate a design against architectural principles", false),
		act("software.reason.research", "Research", "Research a topic across configured sources", false),

		act("software.decide.prioritize_tasks", "Prioritize Tasks", "Order a task list by impact and risk", false),

		writes(act("software.act.write_code", "Write Code", "Write implementation into an isolated worktree, via the configured coding-agent CLI", false)),
		writes(act("software.act.write_test", "Write Test", "Write tests into an isolated worktree, via the configured coding-agent CLI", false)),
		// Takes params.design: the document text. Declared here because a
		// capability whose only required input is undocumented is one models
		// call with an empty payload — and this one is planned by two agents
		// and required by a priority-9 hint before any write_code action, so
		// it is called often.
		{
			ID: "software.act.write_design_doc", Name: "Write Design Doc", Domain: "software",
			Description: "Produce mandatory design document before implementation. Requires params.design (the document text).",
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"design": {Type: "string", Description: "The design document: the problem, the approach, and what was rejected"},
				},
				Required: []string{"design"},
			},
			OutputSchema: capability.Schema{Type: "object"},
		},
		writes(act("software.act.create_pr", "Create PR", "Submit a worktree branch as a pull request", false)),
		act("software.act.create_ticket", "Create Ticket", "Create ticket in project management tool", false),
		act("software.act.send_message", "Send Message", "Send a message via MessagingAdapter", false),
		writes(act("software.act.delegate_to_cli", "Delegate to CLI Agent", "Hand a task to a coding-agent CLI (Claude Code, Cursor, Gemini, Copilot) in an isolated worktree", false)),
		act("software.act.shell_exec", "Shell Exec", "Run a /bin/sh command with params.cmd (required), optional params.workdir and params.timeout_sec (max 600). Result includes exit_code, stdout, stderr. Dangerous patterns (rm -rf /, mkfs, sudo, curl|sh) are blocked.", true),

		// Not shell_exec under another name: it names the alert it is for and
		// why, and the SRE agent's bounds escalate every call.
		{
			ID: CapRunRemediation, Name: "Run Remediation", Domain: "software",
			Description: "Run a /bin/sh command that changes a running system, to remediate one observed alert. Always escalated for approval under the SRE agent's bounds. Requires params.alert_id, params.rationale and params.cmd; optional params.workdir and params.timeout_sec (max 600). Result includes exit_code, stdout, stderr, alert_id and rationale. Dangerous patterns (rm -rf /, mkfs, sudo, curl|sh) are blocked.",
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"alert_id":    {Type: "string", Description: "ID of the observed alert this remediation is for, as observed"},
					"rationale":   {Type: "string", Description: "Why this command addresses that alert"},
					"cmd":         {Type: "string", Description: "The /bin/sh command to run"},
					"workdir":     {Type: "string", Description: "Directory to run the command in. Defaults to the environment's root"},
					"timeout_sec": {Type: "integer", Description: "Command timeout in seconds. Default 60, max 600"},
				},
				Required: []string{"alert_id", "rationale", "cmd"},
			},
			OutputSchema: capability.Schema{Type: "object"},
		},

		{
			ID: CapAlertsResolved, Name: "Alerts Resolved", Domain: "software",
			Description: "Verify that the alerts a remediation was for are no longer open, by asking the twin's bound observability instance what is firing or acknowledged. Requires params.alert_ids. Result includes resolved and still_open; it fails while any named alert is open, and when the instance could not be asked.",
			Verifiable:  true,
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"alert_ids": {Type: "array", Description: "IDs of the alerts the remediation was for, as observed. A list of strings, or one comma-separated string"},
				},
				Required: []string{"alert_ids"},
			},
			OutputSchema: capability.Schema{Type: "object"},
		},
		act("software.verify.run_tests", "Run Tests", "Execute test suite in worktree", true),
		act("software.verify.lint", "Lint", "Run linter in worktree", true),
		act("software.verify.review", "Code Review", "Peer review of an artifact", true),
		act("software.verify.tech_lead_review", "Tech Lead Review", "Senior review of an artifact against design doc", true),

		act("software.learn.extract_patterns", "Extract Patterns", "Extract reusable patterns from completed objective", false),
		act("software.learn.update_tech_radar", "Update Tech Radar", "Update the team's technology assessment", false),
	}
}
