package objective

import (
	"encoding/json"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
)

type ObjectiveID string

type ObjectiveStatus string

const (
	StatusPending   ObjectiveStatus = "pending"
	StatusActive    ObjectiveStatus = "active"
	StatusBlocked   ObjectiveStatus = "blocked"
	StatusCompleted ObjectiveStatus = "completed"
	StatusFailed    ObjectiveStatus = "failed"

	// StatusConverged is where a standing objective rests: the criteria are
	// met and the supervisor is watching for them to stop being met. It is
	// deliberately not StatusCompleted, which says the work is over and the
	// row can be forgotten.
	StatusConverged ObjectiveStatus = "converged"
)

// Terminal reports whether a status means no further work will happen without
// somebody asking for it. Standing objectives never reach one.
func (s ObjectiveStatus) Terminal() bool {
	return s == StatusCompleted || s == StatusFailed
}

type Objective struct {
	ID                ObjectiveID     `json:"id"`
	Title             string          `json:"title"`
	Description       string          `json:"description,omitempty"`
	Domain            string          `json:"domain"`
	AdditionalDomains []string        `json:"additional_domains,omitempty"`
	TwinID            string          `json:"twin_id,omitempty"`
	Priority          int             `json:"priority,omitempty"`
	MaxIterations     int             `json:"max_iterations,omitempty"`
	Deadline          *time.Time      `json:"deadline,omitempty"`
	SuccessCriteria   []Criterion     `json:"success_criteria,omitempty"`
	Constraints       []Constraint    `json:"constraints,omitempty"`
	ParentID          *ObjectiveID    `json:"parent_id,omitempty"`
	Status            ObjectiveStatus `json:"status"`

	// Mode says whether this objective finishes or is held. Empty is
	// oneshot, so every objective that predates standing mode keeps its
	// behaviour exactly.
	Mode Mode `json:"mode,omitempty"`
	// Cadence declares when a standing objective is sensed and reconciled.
	// Nil on a oneshot objective, and on a standing one that only ever
	// reconciles when asked.
	Cadence *Cadence `json:"cadence,omitempty"`
	// Autonomy declares how much a standing objective may do without
	// asking, and how much it may earn. Nil means propose-only.
	Autonomy *Autonomy `json:"autonomy,omitempty"`
	// Budget caps what this objective may spend on its own, separately from
	// its twin's allowance. Nil means no ceiling of its own, which is what
	// every objective written before Phase 23 has.
	Budget *Budget `json:"budget,omitempty"`

	// AgentID names the agent this objective runs under. Empty falls back to
	// the first agent its domain declares, which is what every objective did
	// before templates could say otherwise.
	//
	// It exists because Template.SuggestedAgents was declared and read by
	// nothing: an objective created from a template kept no reference to it,
	// so a template naming the right agent could not make that agent run. In
	// a two-agent pack the default happened to be correct; in a nine-agent
	// pack it silently was not.
	AgentID string `json:"agent_id,omitempty"`

	// TemplateID names the template this objective was created from. Empty
	// means none was used, which is what every objective written before the
	// field existed reads as.
	TemplateID string `json:"template_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AutonomyDeclaration returns the objective's autonomy declaration, or the
// safe default for one that declared none. Callers deciding what an objective
// may do should go through here rather than dereferencing the pointer, so an
// absent declaration and a zero-valued one mean the same thing.
func (o Objective) AutonomyDeclaration() Autonomy {
	if o.Autonomy == nil {
		return Autonomy{}
	}
	return *o.Autonomy
}

// BudgetDeclaration returns the objective's spend ceiling, or the zero value
// when none was declared. Nil-safe for the same reason as the others: an
// absent declaration and a zero-valued one mean the same thing, and no caller
// should have to know which it got.
func (o Objective) BudgetDeclaration() Budget {
	if o.Budget == nil {
		return Budget{}
	}
	return *o.Budget
}

// CadenceDeclaration returns the objective's cadence, or an empty one. An
// empty cadence never becomes due on its own, which is the correct reading of
// a standing objective that declared no schedule: it reconciles when asked.
func (o Objective) CadenceDeclaration() Cadence {
	if o.Cadence == nil {
		return Cadence{}
	}
	return *o.Cadence
}

// AllDomains returns the deduplicated union of Domain and AdditionalDomains.
// The primary Domain always appears first; additional domains preserve their
// declared order. Empty strings are skipped.
func (o Objective) AllDomains() []string {
	out := make([]string, 0, 1+len(o.AdditionalDomains))
	seen := make(map[string]bool, 1+len(o.AdditionalDomains))
	if o.Domain != "" {
		out = append(out, o.Domain)
		seen[o.Domain] = true
	}
	for _, d := range o.AdditionalDomains {
		if d == "" || seen[d] {
			continue
		}
		out = append(out, d)
		seen[d] = true
	}
	return out
}

// CriterionDomains returns the deduplicated set of domains referenced by
// success criteria via the optional Domain field. Criteria without an
// explicit domain are not included. Used by stepVerify to weight per-domain
// scores in cross-domain objectives.
func (o Objective) CriterionDomains() []string {
	if len(o.SuccessCriteria) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(o.SuccessCriteria))
	out := make([]string, 0, len(o.SuccessCriteria))
	for _, c := range o.SuccessCriteria {
		if c.Domain == "" || seen[c.Domain] {
			continue
		}
		out = append(out, c.Domain)
		seen[c.Domain] = true
	}
	return out
}

type Criterion struct {
	ID          string                  `json:"id"`
	Description string                  `json:"description"`
	Verifier    capability.CapabilityID `json:"verifier,omitempty"`
	Threshold   any                     `json:"threshold,omitempty"`
	Weight      float64                 `json:"weight"`
	// Domain optionally scopes the criterion to one of the objective's
	// domains; verifier resolution then prefers a capability from that pack
	// when multiple packs offer the same capability ID. Cross-domain
	// objectives use this to keep per-pack acceptance criteria isolated.
	Domain string `json:"domain,omitempty"`

	// Required says whether the criterion gates completion. Nil is required,
	// so every criterion written before the field existed still gates. Read
	// it through IsRequired.
	Required *bool `json:"required,omitempty"`
	// State is what the last verify step settled, and Reason says why when
	// the state is inconclusive. "met" on the wire is derived from it; see
	// MarshalJSON.
	State  CriterionState     `json:"state,omitempty"`
	Reason InconclusiveReason `json:"reason,omitempty"`
}

// CriterionState is what a verify step settled about one criterion.
type CriterionState string

const (
	CriterionMet          CriterionState = "met"
	CriterionNotMet       CriterionState = "not_met"
	CriterionInconclusive CriterionState = "inconclusive"
)

// InconclusiveReason says why a criterion could not be settled.
type InconclusiveReason string

// ReasonVerifierError is a named verifier that could not run. It blocks
// completion without saying anything about the work.
const ReasonVerifierError InconclusiveReason = "verifier_error"

// IsRequired reports whether the criterion gates completion.
func (c Criterion) IsRequired() bool {
	return c.Required == nil || *c.Required
}

// IsMet reports whether the last verify step settled the criterion met.
func (c Criterion) IsMet() bool { return c.State == CriterionMet }

// criterionFields is Criterion without its methods, so the JSON methods below
// can encode the fields without calling themselves.
type criterionFields Criterion

// MarshalJSON writes "met" beside the state, derived from it, so the API's
// readers keep the field and it can never disagree with the state.
func (c Criterion) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		criterionFields
		Met bool `json:"met"`
	}{criterionFields(c), c.IsMet()})
}

// UnmarshalJSON reads a state when there is one. An objective stored before
// states existed carries only "met", and a true one reads back met; a "met"
// beside a state is ignored, so a stale flag cannot override what was settled.
func (c *Criterion) UnmarshalJSON(data []byte) error {
	var in struct {
		criterionFields
		Met bool `json:"met"`
	}
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	*c = Criterion(in.criterionFields)
	if c.State == "" && in.Met {
		c.State = CriterionMet
	}
	return nil
}

// AllRequiredMet is the completion rule: at least one criterion is required,
// and every required one is met. An optional criterion neither blocks nor
// grants completion, and zero required criteria never completes.
func AllRequiredMet(criteria []Criterion) bool {
	required := 0
	for _, c := range criteria {
		if !c.IsRequired() {
			continue
		}
		if !c.IsMet() {
			return false
		}
		required++
	}
	return required > 0
}

// VerifierIsReserved reports whether this criterion names a verifier from the
// reserved MCP namespace, which is never valid however the criterion was
// written — by a pack, by a template, or by whoever posted the objective.
//
// It is the second of ADR 022's four bounds, and the one with the sharpest
// argument: a criterion is how this deployment decides its own work is done. A
// verifier discovered from a server somebody else runs would let that server
// declare an objective satisfied, and the tool it named could have appeared this
// morning. Verification is the one thing a third party must not be able to
// settle, so the prefix is refused rather than resolved.
func (c Criterion) VerifierIsReserved() bool {
	return capability.IsMCPCapability(c.Verifier)
}

type Constraint struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Hard        bool   `json:"hard"`
	Expression  string `json:"expression,omitempty"`
}
