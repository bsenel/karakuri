package agriculture

import "github.com/bsenel/karakuri/internal/core/capability"

func agricultureCapabilities() []capability.Capability {
	prop := func(typ, desc string) capability.SchemaProperty {
		return capability.SchemaProperty{Type: typ, Description: desc}
	}

	return []capability.Capability{
		{
			ID:          "agriculture.observe.soil_conditions",
			Name:        "Observe Soil Conditions",
			Domain:      "agriculture",
			Description: "Observe soil moisture, pH, and nutrient levels from field sensors",
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"field_id": prop("string", "Unique identifier of the field"),
					"zone_id":  prop("string", "Optional zone within the field"),
					"depth_cm": prop("number", "Sensor depth in centimetres"),
				},
				Required: []string{"field_id"},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"moisture_pct": prop("number", "Volumetric water content percentage"),
					"ph":           prop("number", "Soil pH value"),
					"nitrogen_ppm": prop("number", "Nitrogen concentration in ppm"),
					"phosphorus_ppm": prop("number", "Phosphorus concentration in ppm"),
					"potassium_ppm": prop("number", "Potassium concentration in ppm"),
					"timestamp":    prop("string", "ISO-8601 observation timestamp"),
				},
			},
		},
		{
			ID:          "agriculture.observe.weather",
			Name:        "Observe Weather",
			Domain:      "agriculture",
			Description: "Retrieve current and forecast weather data for the farm location",
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"latitude":       prop("number", "Farm latitude in decimal degrees"),
					"longitude":      prop("number", "Farm longitude in decimal degrees"),
					"forecast_days":  prop("integer", "Number of forecast days to retrieve (1-14)"),
				},
				Required: []string{"latitude", "longitude"},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"temperature_c":    prop("number", "Current temperature in Celsius"),
					"humidity_pct":     prop("number", "Relative humidity percentage"),
					"precipitation_mm": prop("number", "Expected precipitation in mm"),
					"wind_speed_kph":   prop("number", "Wind speed in km/h"),
					"forecast":         prop("array", "Array of daily forecast objects"),
				},
			},
		},
		{
			ID:          "agriculture.observe.crop_health",
			Name:        "Observe Crop Health",
			Domain:      "agriculture",
			Description: "Visual and sensor-based crop health assessment using NDVI and disease indicators",
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"field_id":    prop("string", "Field identifier"),
					"crop_type":   prop("string", "Type of crop being assessed"),
					"image_sha":   prop("string", "Optional SHA of drone/satellite image artifact"),
				},
				Required: []string{"field_id", "crop_type"},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"ndvi_score":        prop("number", "Normalised difference vegetation index (0-1)"),
					"health_status":     prop("string", "Overall status: healthy | stressed | diseased | critical"),
					"disease_risk_pct":  prop("number", "Estimated disease risk percentage"),
					"affected_area_pct": prop("number", "Percentage of field area showing issues"),
					"recommendations":   prop("array", "List of recommended interventions"),
				},
			},
		},
		{
			ID:          "agriculture.reason.yield_forecast",
			Name:        "Yield Forecast",
			Domain:      "agriculture",
			Description: "Predict expected yield based on current crop health, soil, and weather conditions",
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"field_id":         prop("string", "Field identifier"),
					"crop_type":        prop("string", "Crop variety"),
					"planting_date":    prop("string", "ISO-8601 planting date"),
					"historical_yield": prop("number", "Historical average yield in tonnes/ha"),
				},
				Required: []string{"field_id", "crop_type"},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"forecast_tonnes_ha": prop("number", "Predicted yield in tonnes per hectare"),
					"confidence_pct":     prop("number", "Forecast confidence percentage"),
					"limiting_factors":   prop("array", "Key factors constraining yield"),
					"harvest_window":     prop("string", "Recommended harvest date range"),
				},
			},
		},
		{
			ID:          "agriculture.reason.irrigation_plan",
			Name:        "Irrigation Plan",
			Domain:      "agriculture",
			Description: "Generate an optimised irrigation schedule based on soil moisture, weather, and crop stage",
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"field_id":      prop("string", "Field identifier"),
					"crop_type":     prop("string", "Crop variety"),
					"planning_days": prop("integer", "Number of days to plan ahead"),
					"system_type":   prop("string", "Irrigation system: drip | sprinkler | furrow"),
				},
				Required: []string{"field_id", "crop_type"},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"schedule":        prop("array", "Array of scheduled irrigation events"),
					"total_water_m3":  prop("number", "Total water volume in cubic metres"),
					"efficiency_pct":  prop("number", "Estimated water-use efficiency"),
					"next_event":      prop("string", "ISO-8601 timestamp of next irrigation"),
				},
			},
		},
		{
			ID:          "agriculture.act.irrigate",
			Name:        "Irrigate Field",
			Domain:      "agriculture",
			Description: "Trigger the field irrigation system for a specified zone and duration",
			Verifiable:  true,
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"field_id":      prop("string", "Field identifier"),
					"zone_id":       prop("string", "Zone to irrigate"),
					"duration_min":  prop("number", "Irrigation duration in minutes"),
					"flow_rate_lpm": prop("number", "Flow rate in litres per minute"),
				},
				Required: []string{"field_id", "zone_id", "duration_min"},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"success":         prop("boolean", "Whether irrigation was triggered successfully"),
					"water_applied_l": prop("number", "Actual water applied in litres"),
					"started_at":      prop("string", "ISO-8601 start timestamp"),
					"completed_at":    prop("string", "ISO-8601 completion timestamp"),
				},
			},
		},
		{
			ID:          "agriculture.act.apply_treatment",
			Name:        "Apply Treatment",
			Domain:      "agriculture",
			Description: "Apply fertilizer, pesticide, or other agrochemical treatment to a field zone",
			Verifiable:  true,
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"field_id":        prop("string", "Field identifier"),
					"zone_id":         prop("string", "Target zone"),
					"treatment_type":  prop("string", "Type: fertilizer | pesticide | herbicide | fungicide"),
					"product_id":      prop("string", "Product SKU or identifier"),
					"rate_kg_ha":      prop("number", "Application rate in kg per hectare"),
				},
				Required: []string{"field_id", "zone_id", "treatment_type", "product_id", "rate_kg_ha"},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"success":          prop("boolean", "Whether treatment was applied successfully"),
					"area_treated_ha":  prop("number", "Area covered in hectares"),
					"product_used_kg":  prop("number", "Actual product quantity used"),
					"applied_at":       prop("string", "ISO-8601 application timestamp"),
				},
			},
		},
		{
			ID:          "agriculture.verify.yield_target",
			Name:        "Verify Yield Target",
			Domain:      "agriculture",
			Description: "Verify that the forecasted or actual yield meets the configured target threshold",
			Verifiable:  true,
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"field_id":        prop("string", "Field identifier"),
					"target_tonnes_ha": prop("number", "Target yield threshold in tonnes per hectare"),
					"actual_tonnes_ha": prop("number", "Actual or forecasted yield for comparison"),
				},
				Required: []string{"field_id", "target_tonnes_ha", "actual_tonnes_ha"},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"met":           prop("boolean", "Whether the target was met"),
					"variance_pct":  prop("number", "Percentage variance from target (positive = over)"),
					"grade":         prop("string", "Performance grade: excellent | on_target | below_target | critical"),
				},
			},
		},
	}
}
