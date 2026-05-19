package software

import (
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/objective"
)

func softwareObjectiveTemplates() []objective.Template {
	crit := func(id, desc string, verifier string, weight float64) objective.Criterion {
		return objective.Criterion{
			ID: id, Description: desc,
			Verifier: capability.CapabilityID(verifier),
			Weight: weight,
		}
	}
	hard := func(id, desc, expr string) objective.Constraint {
		return objective.Constraint{ID: id, Description: desc, Hard: true, Expression: expr}
	}

	return []objective.Template{
		{
			ID: "software.objective.strategy", Title: "Strategy", Domain: "software",
			Description: "Research, business model, and value proposition",
			SuccessCriteria: []objective.Criterion{
				crit("strategy-doc", "Strategy document produced", "reason.evaluate", 1.0),
			},
		},
		{
			ID: "software.objective.discovery", Title: "Discovery", Domain: "software",
			Description: "Requirements, design doc, user stories, and task breakdown",
			SuccessCriteria: []objective.Criterion{
				crit("design-doc", "Design document produced", "software.verify.tech_lead_review", 0.5),
				crit("tasks", "Task breakdown complete", "reason.evaluate", 0.5),
			},
		},
		{
			ID: "software.objective.delivery", Title: "Delivery", Domain: "software",
			Description: "TDD implementation with design doc, review, and PR",
			SuccessCriteria: []objective.Criterion{
				crit("tests-pass", "All tests pass", "software.verify.run_tests", 0.4),
				crit("lint-pass", "Linter passes", "software.verify.lint", 0.1),
				crit("peer-review", "Peer review approved", "software.verify.review", 0.25),
				crit("lead-review", "Tech lead review approved", "software.verify.tech_lead_review", 0.25),
			},
			Constraints: []objective.Constraint{
				hard("design-first", "Design doc must exist before any write_code action", "design_doc_exists"),
				hard("tdd-order", "write_test must precede the write_code it covers", "test_before_code"),
				hard("two-stage-review", "Both review and tech_lead_review must pass before create_pr", "reviews_passed"),
			},
		},
		{
			ID: "software.objective.code_review", Title: "Code Review", Domain: "software",
			Description: "Review all open PRs or a specific PR",
			SuccessCriteria: []objective.Criterion{
				crit("review-complete", "Review report produced", "software.verify.review", 1.0),
			},
		},
		{
			ID: "software.objective.research", Title: "Research", Domain: "software",
			Description: "Deep research on a topic or ticket",
			SuccessCriteria: []objective.Criterion{
				crit("research-report", "Research report produced", "reason.evaluate", 1.0),
			},
		},
		{
			ID: "software.objective.incident_response", Title: "Incident Response", Domain: "software",
			Description: "Fetch logs/metrics, identify issues, produce and execute remediation plan",
			SuccessCriteria: []objective.Criterion{
				crit("root-cause", "Root cause identified", "reason.evaluate", 0.4),
				crit("remediation", "Remediation applied", "software.verify.run_tests", 0.6),
			},
			Constraints: []objective.Constraint{
				hard("approval-required", "All act capabilities require human approval", "approval_given"),
			},
		},
		{
			ID: "software.objective.autonomous_watch", Title: "Autonomous Watch", Domain: "software",
			Description: "Continuous environment observation; promotes to other templates on signal",
			SuccessCriteria: []objective.Criterion{
				crit("running", "Watcher active", "observe.fetch_signal", 1.0),
			},
		},
	}
}
