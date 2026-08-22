package objective

import (
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
	Met         bool                    `json:"met"`
	// Domain optionally scopes the criterion to one of the objective's
	// domains; verifier resolution then prefers a capability from that pack
	// when multiple packs offer the same capability ID. Cross-domain
	// objectives use this to keep per-pack acceptance criteria isolated.
	Domain string `json:"domain,omitempty"`
}

type Constraint struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Hard        bool   `json:"hard"`
	Expression  string `json:"expression,omitempty"`
}
