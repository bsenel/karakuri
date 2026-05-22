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
)

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
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
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
