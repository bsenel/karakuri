package software

// Standing streams: the objectives an operator declares so a deployment keeps
// improving itself on a cadence.
//
// The operator wants four things to keep happening: the market researched
// every two weeks and roadmap phases proposed from it, the deployment's own
// telemetry and audit log read daily and technical enhancements recorded, a
// small CLI or web UX improvement made daily, and whatever the roadmap says
// is approved implemented. Each is a standing objective (Phase 20).
//
// None of the existing templates survives being run that way. A loop whose
// criteria are not met ends failed (finalizeLoop in
// internal/feature/loop/runner.go), and reconcile counts that pass against
// the circuit breaker (ConsecutiveFailures in internal/feature/reconcile/run.go;
// BreakerFailures defaults to 3), which pauses the objective.
// software.objective.strategy, software.objective.self_improve and
// software.objective.delivery each ask for something to have been produced,
// so each would fail on a quiet day and pause after three of them. The API's
// create request carries no criteria of its own: an objective gets them from
// the template it names and from nowhere else, so criteria that hold on a
// quiet day have to be declared in a pack.
//
// The rule: a stream's criteria describe the state a correct pass leaves
// behind, and a pass that looked and found nothing to do has left that state.
// They must not be worded so that a pass which did not look can meet them.

import (
	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/objective"
)

const (
	marketDiscoveryTemplateID    = "software.objective.market_discovery"
	engineeringBacklogTemplateID = "software.objective.engineering_backlog"
	uxImprovementTemplateID      = "software.objective.ux_improvement"
	roadmapDeliveryTemplateID    = "software.objective.roadmap_delivery"
)

// streamTemplates are the four standing streams.
//
// Every criterion is judged and none names a verifier: nothing deterministic
// settles "the data was read and held nothing new", and a verifier answers
// the criterion, it does not supply the material for answering it.
func streamTemplates() []objective.Template {
	judged := func(id, desc string, weight float64) objective.Criterion {
		return objective.Criterion{ID: id, Description: desc, Weight: weight}
	}
	hard := func(id, desc, expr string) objective.Constraint {
		return objective.Constraint{ID: id, Description: desc, Hard: true, Expression: expr}
	}
	// A stream keeps running, so the thing it must never do is a constraint
	// on the objective rather than something each pass is trusted to remember.
	noMerge := hard("no-merge", "A human merges; this objective never merges a pull request or pushes to main", "no_merge")

	return []objective.Template{
		{
			ID:     marketDiscoveryTemplateID,
			Title:  "Market discovery",
			Domain: "software",
			// It writes a report and proposes roadmap text in a pull request
			// a human merges.
			Risk:            objective.RiskRoutine,
			SuggestedAgents: []agent.Definition{{ID: "software.agent.strategist"}},
			Description: "Research who the users are, what they need, where the field is moving and what is feasible, " +
				"record it as a dated report with sources, and propose roadmap phases only where the evidence supports them.",
			SuccessCriteria: []objective.Criterion{
				judged("report", "This pass's actions produced a dated discovery report whose claims cite their sources, on a branch with an open pull request; or they show the previous report was read and state plainly that nothing material has changed since it", 0.5),
				judged("roadmap", "Every roadmap phase this pass proposed is backed by evidence in the report and is in that pull request; or the pass proposed none and its actions say why", 0.5),
			},
			Constraints: []objective.Constraint{noMerge},
		},
		{
			ID:              engineeringBacklogTemplateID,
			Title:           "Engineering backlog from this deployment's own data",
			Domain:          "software",
			Risk:            objective.RiskRoutine,
			SuggestedAgents: []agent.Definition{{ID: "software.agent.maintainer"}},
			Description: "Read this deployment's telemetry, audit log and logs, and record technical enhancements " +
				"in the roadmap's Engineering Backlog, each with the data that shows the problem.",
			SuccessCriteria: []objective.Criterion{
				judged("data-read", "This pass's actions show the deployment's audit log and telemetry for the last day were actually read", 0.5),
				judged("backlog", "A finding with its evidence was added to the Engineering Backlog in an open pull request; or the actions state that the data showed nothing not already recorded", 0.5),
			},
			Constraints: []objective.Constraint{noMerge},
		},
		{
			ID:     uxImprovementTemplateID,
			Title:  "CLI and web UX improvement",
			Domain: "software",
			// It changes code and opens a pull request.
			Risk:            objective.RiskConsequential,
			SuggestedAgents: []agent.Definition{{ID: "software.agent.implementer"}},
			Description:     "Use the CLI and the web interface as a new user would and make as many small, tested improvements per pass as fit.",
			SuccessCriteria: []objective.Criterion{
				judged("improvement", "This pass's actions made one or more small user-experience improvements to the CLI or the web interface, each with a test, in an open pull request; or they state that they looked and found nothing worth changing", 0.6),
				judged("verified", "The actions say exactly what was run to verify the change and what was not run", 0.4),
			},
			Constraints: []objective.Constraint{noMerge},
		},
		{
			ID:              roadmapDeliveryTemplateID,
			Title:           "Deliver what the roadmap approves",
			Domain:          "software",
			Risk:            objective.RiskConsequential,
			SuggestedAgents: []agent.Definition{{ID: "software.agent.implementer"}},
			Description: "Implement the roadmap phases and backlog entries marked Planned on main, as many verified slices per pass as fit, " +
				"and open each item's pull request when it is complete.",
			SuccessCriteria: []objective.Criterion{
				judged("slice", "This pass's actions delivered one or more slices of Planned roadmap items to those items' branches with their tests, or opened or updated their pull requests; or they state that nothing on main is Planned and not yet delivered", 0.6),
				judged("verified", "Tests and lint were run against the tip of every branch this pass pushed to and their results are in the actions; or no slice was delivered in this pass", 0.4),
			},
			Constraints: []objective.Constraint{noMerge},
		},
	}
}
