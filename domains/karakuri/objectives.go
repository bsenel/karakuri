package karakuri

import (
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/objective"
)

func karakuriObjectiveTemplates() []objective.Template {
	crit := func(id, desc, verifier string, weight float64) objective.Criterion {
		return objective.Criterion{
			ID:          id,
			Description: desc,
			Verifier:    capability.CapabilityID(verifier),
			Weight:      weight,
		}
	}
	// foreign declares a criterion verified by another pack. Phase 13's
	// Criterion.Domain is what makes that legible rather than a dangling
	// reference — and it is what lets the loop prefer that pack's capability
	// when two export the same ID.
	foreign := func(id, desc, domainID, verifier string, weight float64) objective.Criterion {
		c := crit(id, desc, verifier, weight)
		c.Domain = domainID
		return c
	}
	hard := func(id, desc, expr string) objective.Constraint {
		return objective.Constraint{ID: id, Description: desc, Hard: true, Expression: expr}
	}

	return []objective.Template{
		{
			ID:     "karakuri.objective.watch_health",
			Title:  "Watch this deployment's health",
			Domain: "karakuri",
			Description: "Read the deployment's own telemetry and report what is limiting it. " +
				"Reads only — declare it standing at sense autonomy and it will never spend a model call on a quiet week.",
			SuccessCriteria: []objective.Criterion{
				crit("no-blocked", "No standing objective is blocked by the breaker or the stall detector", CapAnalyseUsage, 0.5),
				crit("no-stale-decisions", "No checkpoint has been waiting more than a day", CapAnalyseUsage, 0.5),
			},
			Constraints: []objective.Constraint{
				hard("read-only", "This objective observes and reports; it must not act on the deployment", "no_write_capabilities"),
			},
		},
		{
			ID:     "karakuri.objective.self_improve",
			Title:  "Improve Karakuri from its own evidence",
			Domain: "karakuri",
			Description: "Analyse telemetry, decide what is worth changing, and open a pull request that changes it. " +
				"Cross-domain by construction: this pack analyses and drafts, and the software pack does the writing " +
				"in a worktree, so the change arrives as a pull request somebody reviews.",
			SuccessCriteria: []objective.Criterion{
				crit("evidence", "The proposal names the telemetry that says the problem is real", CapAnalyseUsage, 0.3),
				crit("proposal", "A roadmap phase exists in the repository's established style", CapProposeRoadmap, 0.3),
				// Verified by the software pack, which is the point of the
				// cross-domain shape: this pack cannot mark its own homework
				// on the part it does not do.
				foreign("pull-request", "A pull request is open with the change and its tests",
					"software", "software.act.open_pull_request", 0.4),
			},
			Constraints: []objective.Constraint{
				hard("evidence-first", "No proposal may be drafted before analyse_usage has run", "analysis_complete"),
				hard("human-approves", "Every change to the repository requires explicit approval", "change_approved"),
				hard("respect-repo-rules", "Changes must follow AGENTS.md: clean-architecture boundaries, tests for non-trivial logic, docs updated", "repo_rules_followed"),
			},
		},
	}
}

func karakuriPlannerHints() []domain.PlannerHint {
	return []domain.PlannerHint{
		{
			Condition: "capability.id == 'karakuri.propose_roadmap_phase'",
			Guidance: "Run karakuri.analyse_usage first and quote its bottlenecks. A phase proposed without " +
				"evidence from telemetry is a preference, and the repository already has a roadmap full of " +
				"decisions somebody justified.",
			Priority: 10,
		},
		{
			Condition: "objective.template == 'karakuri.objective.self_improve'",
			Guidance: "Read AGENTS.md before writing anything. Karakuri's rules for contributors apply to " +
				"Karakuri: YAGNI, the inward-only dependency direction, tests for non-trivial logic in " +
				"internal/feature and internal/platform, and no vendor imports in internal/core.",
			Priority: 10,
		},
		{
			Condition: "capability.id startswith 'software.act'",
			Guidance: "Changes to Karakuri itself go through a git worktree and a pull request. Never write to " +
				"the checked-out working tree directly, and never push to the default branch.",
			Priority: 9,
		},
		{
			Condition: "environment.id == 'karakuri.env.telemetry'",
			Guidance: "This environment is read-only. It answers what the deployment has been doing; anything " +
				"that changes the deployment belongs to another environment.",
			Priority: 8,
		},
	}
}
