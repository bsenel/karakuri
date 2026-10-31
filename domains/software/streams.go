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
	"strings"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
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
// Each pass is required to leave a pull request a machine can check (Phase
// 41): opened or updated in this pass, touching only the paths the stream
// owns, with every check green at its head. What only a reader can tell is
// kept as judged progress, required: false with a weight, so a model's reading
// scores the pass but can no longer fail one that did the work. A pass that
// found nothing new opens no pull request; stream_pr is then inconclusive:
// no_data, which blocks completion without tripping the breaker.
func streamTemplates() []objective.Template {
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
			SuccessCriteria: append(pullRequestCriteria("docs/research/**"),
				progress("findings-new", "The report's findings are new since the previous report and cite their sources, and every roadmap phase it proposes is backed by them", 0.3),
			),
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
			SuccessCriteria: append(pullRequestCriteria("docs/roadmap.md"),
				progress("evidence", "Each Engineering Backlog entry this pass added cites the audit log, telemetry or log data that shows the problem", 0.3),
			),
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
			SuccessCriteria: append(pullRequestCriteria("cli/**", "web/**"),
				progress("visible", "Each improvement in the pull request is visible to a user of the CLI or the web interface, and has a test", 0.3),
			),
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
				verified("pr", "The item's pull request was opened or updated in this pass (params: branch, since)", CapStreamPR, 0.2),
				verified("ci", "Every check is green at the pull request's head (params: branch)", CapCIGreen, 0.2),
				verified("tests-pass", "The test suite passes at the tip of the branch this pass pushed to", "software.verify.run_tests", 0.2),
				progress("peer-review", "Peer review approved", 0.2),
				progress("lead-review", "Tech lead review approved", 0.2),
			},
			Constraints: []objective.Constraint{noMerge},
		},
	}
}

// verified is a criterion its named verifier settles, and nothing else: when
// the verifier did not run, the criterion is inconclusive, not judged.
func verified(id, desc, verifier string, weight float64) objective.Criterion {
	return objective.Criterion{ID: id, Description: desc, Verifier: capability.CapabilityID(verifier), Weight: weight}
}

// progress is a judged criterion kept to score progress: required: false, so
// a model's reading can neither block nor grant completion.
func progress(id, desc string, weight float64) objective.Criterion {
	required := false
	return objective.Criterion{ID: id, Description: desc, Weight: weight, Required: &required}
}

// pullRequestCriteria are the required criteria of a pass that delivers
// through a pull request touching only paths: opened or updated in this pass,
// inside those paths, and green at its head. Their weights sum to 0.7, which
// leaves 0.3 for the template's judged progress.
func pullRequestCriteria(paths ...string) []objective.Criterion {
	return []objective.Criterion{
		verified("pr", "The pull request on the stream's branch was opened or updated in this pass (params: branch, since)", CapStreamPR, 0.25),
		{
			ID:          "paths",
			Description: "The pull request's diff touches only " + strings.Join(paths, ", ") + " (params: branch, and paths as this criterion's threshold)",
			Verifier:    CapPathsDeclared,
			Threshold:   paths,
			Weight:      0.2,
		},
		verified("ci", "Every check is green at the pull request's head (params: branch)", CapCIGreen, 0.25),
	}
}
