package agriculture

import (
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/objective"
)

func agricultureObjectiveTemplates() []objective.Template {
	crit := func(id, desc string, verifier string, weight float64) objective.Criterion {
		return objective.Criterion{
			ID:          id,
			Description: desc,
			Verifier:    capability.CapabilityID(verifier),
			Weight:      weight,
		}
	}
	hard := func(id, desc, expr string) objective.Constraint {
		return objective.Constraint{ID: id, Description: desc, Hard: true, Expression: expr}
	}

	return []objective.Template{
		{
			ID:          "agriculture.objective.optimize_yield",
			Title:       "Optimize Crop Yield",
			Domain:      "agriculture",
			Description: "Observe field conditions, forecast yield, apply targeted treatments, and verify the yield target is met",
			SuccessCriteria: []objective.Criterion{
				crit("yield-forecast", "Yield forecast produced with confidence >= 80%", "agriculture.reason.yield_forecast", 0.3),
				crit("yield-target", "Forecasted yield meets or exceeds target threshold", "agriculture.verify.yield_target", 0.7),
			},
			Constraints: []objective.Constraint{
				hard("observe-first", "Soil conditions and crop health must be observed before any act capability", "observations_complete"),
				hard("approval-treatment", "apply_treatment requires explicit human approval", "treatment_approved"),
			},
		},
		{
			ID:          "agriculture.objective.irrigation_schedule",
			Title:       "Create Irrigation Schedule",
			Domain:      "agriculture",
			Description: "Analyse soil moisture and weather forecast, generate an optimised irrigation schedule, and execute it",
			SuccessCriteria: []objective.Criterion{
				crit("plan-produced", "Irrigation plan generated for the requested planning horizon", "agriculture.reason.irrigation_plan", 0.4),
				crit("irrigation-executed", "Scheduled irrigation events executed successfully", "agriculture.act.irrigate", 0.6),
			},
			Constraints: []objective.Constraint{
				hard("weather-check", "Weather forecast must be retrieved before planning", "weather_observed"),
			},
		},
	}
}
