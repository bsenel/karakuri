package software

import (
	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/objective"
)

func softwareObjectiveTemplates() []objective.Template {
	crit := func(id, desc string, verifier string, weight float64) objective.Criterion {
		return objective.Criterion{
			ID: id, Description: desc,
			Verifier: capability.CapabilityID(verifier),
			Weight:   weight,
		}
	}
	// judged declares a criterion no capability settles. Naming a verifier
	// that nothing serves does not fail the criterion — verify.go falls back
	// to asking a model — so the declaration was costing a judgement call per
	// iteration while claiming to be settled by a review that never ran.
	// A verifier answers the criterion; it does not merely relate to it.
	judged := func(id, desc string, weight float64) objective.Criterion {
		return objective.Criterion{ID: id, Description: desc, Weight: weight}
	}
	hard := func(id, desc, expr string) objective.Constraint {
		return objective.Constraint{ID: id, Description: desc, Hard: true, Expression: expr}
	}

	return []objective.Template{
		{
			ID: "software.objective.strategy", Title: "Strategy", Domain: "software",
			Risk:        objective.RiskRoutine,
			Description: "Research, business model, and value proposition",
			SuccessCriteria: []objective.Criterion{
				judged("strategy-doc", "Strategy document produced", 1.0),
			},
		},
		{
			ID: "software.objective.discovery", Title: "Discovery", Domain: "software",
			Risk:        objective.RiskRoutine,
			Description: "Requirements, design doc, user stories, and task breakdown",
			SuccessCriteria: []objective.Criterion{
				judged("design-doc", "Design document produced", 0.5),
				judged("tasks", "Task breakdown complete", 0.5),
			},
		},
		{
			ID: "software.objective.delivery", Title: "Delivery", Domain: "software",
			// Writes code and opens a pull request against a shared repository.
			Risk:            objective.RiskConsequential,
			Description:     "TDD implementation with design doc, review, and PR",
			SuggestedAgents: []agent.Definition{{ID: "software.agent.implementer"}},
			SuccessCriteria: []objective.Criterion{
				crit("tests-pass", "All tests pass", "software.verify.run_tests", 0.4),
				crit("lint-pass", "Linter passes", "software.verify.lint", 0.1),
				judged("peer-review", "Peer review approved", 0.25),
				judged("lead-review", "Tech lead review approved", 0.25),
			},
			Constraints: []objective.Constraint{
				hard("design-first", "Design doc must exist before any write_code action", "design_doc_exists"),
				hard("tdd-order", "write_test must precede the write_code it covers", "test_before_code"),
				hard("two-stage-review", "Both review and tech_lead_review must pass before create_pr", "reviews_passed"),
			},
		},
		{
			ID: "software.objective.code_review", Title: "Code Review", Domain: "software",
			// Produces a review report; it merges nothing.
			Risk:        objective.RiskRoutine,
			Description: "Review all open PRs or a specific PR",
			SuccessCriteria: []objective.Criterion{
				judged("review-complete", "Review report produced", 1.0),
			},
		},
		{
			ID: "software.objective.research", Title: "Research", Domain: "software",
			Risk:        objective.RiskRoutine,
			Description: "Deep research on a topic or ticket",
			SuccessCriteria: []objective.Criterion{
				crit("research-report", "Research report produced", "software.reason.research", 1.0),
			},
		},
		{
			ID: "software.objective.incident_response", Title: "Incident Response", Domain: "software",
			// Executes remediation against a running production system, under
			// time pressure: the one software template where a wrong action
			// reaches the people depending on the service.
			Risk:        objective.RiskHigh,
			Description: "Observe open alerts, fetch logs and metrics, propose a remediation that a human approves, verify the alerts cleared",
			SuccessCriteria: []objective.Criterion{
				judged("root-cause", "Root cause identified", 0.4),
				crit("remediation", "Remediation applied and the alerts cleared", CapAlertsResolved, 0.6),
			},
			SuggestedAgents: []agent.Definition{{ID: "software.agent.sre"}},
			Constraints: []objective.Constraint{
				hard("approval-required", "All act capabilities require human approval", "approval_given"),
			},
		},
		{
			ID: "software.objective.autonomous_watch", Title: "Autonomous Watch", Domain: "software",
			// Observes only. What it promotes to runs under that template's
			// own classification, not this one.
			Risk:        objective.RiskRoutine,
			Description: "Continuous environment observation; promotes to other templates on signal",
			SuccessCriteria: []objective.Criterion{
				judged("running", "Watcher active", 1.0),
			},
		},
	}
}
