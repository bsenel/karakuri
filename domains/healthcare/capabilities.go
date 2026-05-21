package healthcare

import "github.com/bsenel/karakuri/internal/core/capability"

// healthcareCapabilities returns the full capability set for the healthcare
// domain. Coverage spans every loop step type — observe, reason, decide, act,
// verify, learn — so an end-to-end clinical-decision-support objective can
// be expressed without leaving the slot/abstraction model.
//
// High-risk actions (recommend_treatment, order_test) are marked Verifiable
// so the verify step gates them; agent definitions in agents.go list them
// in RequiresApprovalFor so the decide step always escalates rather than
// firing autonomously. See ADR 005 for the isolation guarantees.
func healthcareCapabilities() []capability.Capability {
	prop := func(typ, desc string) capability.SchemaProperty {
		return capability.SchemaProperty{Type: typ, Description: desc}
	}

	return []capability.Capability{
		// ── Observe ───────────────────────────────────────────────────────────
		{
			ID:          "healthcare.observe.vital_signs",
			Name:        "Observe Vital Signs",
			Domain:      "healthcare",
			Description: "Read latest vitals (HR, BP, SpO2, temperature, RR) for the patient",
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"patient_id": prop("string", "Patient identifier (MRN or system ID)"),
					"since":      prop("string", "ISO-8601 timestamp lower bound; omit for latest"),
				},
				Required: []string{"patient_id"},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"heart_rate_bpm":      prop("number", "Heart rate in beats per minute"),
					"systolic_bp_mmhg":    prop("number", "Systolic blood pressure"),
					"diastolic_bp_mmhg":   prop("number", "Diastolic blood pressure"),
					"spo2_pct":            prop("number", "Peripheral capillary oxygen saturation %"),
					"temperature_c":       prop("number", "Body temperature in Celsius"),
					"respiratory_rate":    prop("number", "Respiratory rate (breaths/min)"),
					"observed_at":         prop("string", "ISO-8601 observation timestamp"),
				},
			},
		},
		{
			ID:          "healthcare.observe.lab_results",
			Name:        "Observe Lab Results",
			Domain:      "healthcare",
			Description: "Fetch recent laboratory test results for the patient (CBC, BMP, lipid panel, etc.)",
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"patient_id": prop("string", "Patient identifier"),
					"panels":     prop("array", "Optional list of panel codes; omit for all recent"),
					"since":      prop("string", "ISO-8601 lower bound"),
				},
				Required: []string{"patient_id"},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"results":         prop("array", "Array of {test_code, value, unit, reference_range, flag}"),
					"abnormal_count":  prop("integer", "Number of results outside reference range"),
					"critical_count":  prop("integer", "Number of critical-value results"),
				},
			},
		},
		{
			ID:          "healthcare.observe.medical_history",
			Name:        "Observe Medical History",
			Domain:      "healthcare",
			Description: "Read the patient's problem list, active medications, allergies, and prior encounters",
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"patient_id": prop("string", "Patient identifier"),
				},
				Required: []string{"patient_id"},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"problem_list":   prop("array", "Active diagnoses with ICD-10 codes"),
					"medications":    prop("array", "Active medications with dose + route"),
					"allergies":      prop("array", "Documented allergies and reactions"),
					"recent_visits":  prop("array", "Last N encounters with date + reason"),
				},
			},
		},
		{
			ID:          "healthcare.observe.symptoms",
			Name:        "Observe Symptoms",
			Domain:      "healthcare",
			Description: "Capture presenting complaint and structured symptom list from triage notes",
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"patient_id":    prop("string", "Patient identifier"),
					"encounter_id":  prop("string", "Current encounter identifier"),
				},
				Required: []string{"patient_id"},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"chief_complaint": prop("string", "Free-text presenting complaint"),
					"symptoms":        prop("array", "Structured symptom list with onset + severity"),
					"duration_hours":  prop("number", "Symptom duration in hours"),
				},
			},
		},

		// ── Reason ────────────────────────────────────────────────────────────
		{
			ID:          "healthcare.reason.differential_diagnosis",
			Name:        "Differential Diagnosis",
			Domain:      "healthcare",
			Description: "Generate a ranked list of candidate diagnoses with supporting evidence",
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"patient_id":       prop("string", "Patient identifier"),
					"chief_complaint":  prop("string", "Presenting complaint"),
					"evidence_summary": prop("string", "Synthesised vitals + labs + history"),
				},
				Required: []string{"patient_id", "chief_complaint"},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"candidates":  prop("array", "Ranked array of {diagnosis, icd10, probability, supporting, contradicting}"),
					"red_flags":   prop("array", "Findings requiring immediate escalation"),
				},
			},
		},
		{
			ID:          "healthcare.reason.risk_assessment",
			Name:        "Risk Assessment",
			Domain:      "healthcare",
			Description: "Compute clinical risk score (e.g. NEWS2, qSOFA, CHA2DS2-VASc) for the patient",
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"patient_id": prop("string", "Patient identifier"),
					"score_type": prop("string", "Risk score: news2 | qsofa | chads_vasc | curb65 | other"),
				},
				Required: []string{"patient_id", "score_type"},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"score":          prop("number", "Numerical risk score"),
					"interpretation": prop("string", "Clinical interpretation (low/moderate/high/critical)"),
					"components":     prop("array", "Per-component contribution to the total score"),
				},
			},
		},

		// ── Decide ────────────────────────────────────────────────────────────
		{
			ID:          "healthcare.decide.triage_priority",
			Name:        "Triage Priority",
			Domain:      "healthcare",
			Description: "Assign a triage priority level (e.g. ESI 1-5 or Manchester triage colour)",
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"patient_id":      prop("string", "Patient identifier"),
					"risk_score":      prop("number", "Computed risk score"),
					"red_flag_count":  prop("integer", "Count of red-flag findings"),
				},
				Required: []string{"patient_id"},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"priority":   prop("string", "Priority level (esi-1 | esi-2 | esi-3 | esi-4 | esi-5)"),
					"rationale":  prop("string", "Why this priority was chosen"),
				},
			},
		},

		// ── Act (high-stakes — require approval; see agents.go) ───────────────
		{
			ID:          "healthcare.act.order_test",
			Name:        "Order Test",
			Domain:      "healthcare",
			Description: "Place a diagnostic order (lab, imaging, ECG). Verifiable + escalated for non-routine orders.",
			Verifiable:  true,
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"patient_id":  prop("string", "Patient identifier"),
					"test_code":   prop("string", "Order code (LOINC / CPT)"),
					"priority":    prop("string", "Urgency: routine | stat | timed"),
					"indication":  prop("string", "Free-text clinical indication"),
				},
				Required: []string{"patient_id", "test_code", "indication"},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"order_id":      prop("string", "EHR order identifier"),
					"status":        prop("string", "ordered | scheduled | declined"),
					"submitted_at":  prop("string", "ISO-8601 submission timestamp"),
				},
			},
		},
		{
			ID:          "healthcare.act.recommend_treatment",
			Name:        "Recommend Treatment",
			Domain:      "healthcare",
			Description: "Propose a treatment plan (medication, procedure, referral) — ALWAYS requires human approval before execution",
			Verifiable:  true,
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"patient_id":     prop("string", "Patient identifier"),
					"diagnosis":      prop("string", "Working diagnosis driving the recommendation"),
					"intervention":   prop("string", "Free-text proposed intervention"),
					"contraindications_checked": prop("boolean", "Whether contraindications were reviewed"),
				},
				Required: []string{"patient_id", "diagnosis", "intervention"},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"recommendation_sha": prop("string", "SHA of the persisted recommendation artifact"),
					"requires_approval":  prop("boolean", "Always true — surfaced for callers that miss the AuthorityBounds gate"),
				},
			},
		},
		{
			ID:          "healthcare.act.write_clinical_note",
			Name:        "Write Clinical Note",
			Domain:      "healthcare",
			Description: "Compose a SOAP-format clinical note summarising the encounter and decisions made",
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"patient_id":   prop("string", "Patient identifier"),
					"encounter_id": prop("string", "Encounter identifier"),
					"subjective":   prop("string", "Subjective (history, symptoms)"),
					"objective":    prop("string", "Objective (vitals, exam, labs)"),
					"assessment":   prop("string", "Assessment (working diagnosis)"),
					"plan":         prop("string", "Plan (next steps, follow-up)"),
				},
				Required: []string{"patient_id", "encounter_id"},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"note_sha":     prop("string", "SHA of the persisted note artifact"),
					"signed_by":    prop("string", "Author or attending name"),
					"created_at":   prop("string", "ISO-8601 creation timestamp"),
				},
			},
		},

		// ── Verify ────────────────────────────────────────────────────────────
		{
			ID:          "healthcare.verify.guideline_adherence",
			Name:        "Verify Guideline Adherence",
			Domain:      "healthcare",
			Description: "Check whether the proposed plan matches the relevant clinical guideline (e.g. ATS, NICE, NCCN)",
			Verifiable:  true,
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"patient_id":    prop("string", "Patient identifier"),
					"diagnosis":     prop("string", "Working diagnosis"),
					"plan_sha":      prop("string", "SHA of the recommendation artifact"),
					"guideline_ref": prop("string", "Guideline identifier (e.g. NICE-CG137)"),
				},
				Required: []string{"patient_id", "diagnosis", "plan_sha"},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"adherent":      prop("boolean", "Whether the plan adheres to the guideline"),
					"deviations":    prop("array", "List of deviation descriptions if any"),
					"guideline_ver": prop("string", "Version of the guideline checked"),
				},
			},
		},
		{
			ID:          "healthcare.verify.clinical_review",
			Name:        "Clinical Review",
			Domain:      "healthcare",
			Description: "Independent senior-clinician review of the case and proposed plan; produces a clinical_review artifact",
			Verifiable:  true,
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"patient_id":    prop("string", "Patient identifier"),
					"encounter_id":  prop("string", "Encounter identifier"),
					"plan_sha":      prop("string", "SHA of the recommendation artifact"),
				},
				Required: []string{"patient_id", "plan_sha"},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"review_sha":   prop("string", "SHA of the persisted clinical_review artifact"),
					"approved":     prop("boolean", "Whether the senior reviewer approves the plan"),
					"comments":     prop("string", "Reviewer comments"),
				},
			},
		},

		// ── Learn ─────────────────────────────────────────────────────────────
		{
			ID:          "healthcare.learn.case_summary",
			Name:        "Case Summary",
			Domain:      "healthcare",
			Description: "Extract a structured, de-identified case summary into semantic memory for future recall",
			InputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"patient_id":   prop("string", "Patient identifier (kept local; summary is de-identified)"),
					"encounter_id": prop("string", "Encounter identifier"),
				},
				Required: []string{"patient_id"},
			},
			OutputSchema: capability.Schema{
				Type: "object",
				Properties: map[string]capability.SchemaProperty{
					"summary":       prop("string", "De-identified clinical reasoning summary"),
					"key_findings":  prop("array", "Structured findings list for retrieval"),
					"sources":       prop("array", "Artifact SHAs cited by the summary"),
				},
			},
		},
	}
}
