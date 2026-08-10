package auth

import "slices"

// Effect is a policy's verdict when it matches.
type Effect string

const (
	EffectAllow Effect = "allow"
	EffectDeny  Effect = "deny"
)

// Policy grants (or withholds) one action pattern on one resource pattern,
// optionally narrowed by conditions. Policies belong to roles — a policy with
// no role has nobody to apply to.
//
// Action and Resource use the same pattern grammar: an exact value, a trailing
// "<prefix>:*" glob, or the bare "*".
type Policy struct {
	ID         string      `json:"id"`
	Action     Action      `json:"action"`
	Resource   string      `json:"resource"`
	Effect     Effect      `json:"effect"`
	Conditions []Condition `json:"conditions,omitempty"`
}

// Allow is a shorthand constructor for an unconditional allow policy.
func Allow(id string, action Action, resource string) Policy {
	return Policy{ID: id, Action: action, Resource: resource, Effect: EffectAllow}
}

// Deny is a shorthand constructor for an unconditional deny policy.
func Deny(id string, action Action, resource string) Policy {
	return Policy{ID: id, Action: action, Resource: resource, Effect: EffectDeny}
}

// When returns a copy of the policy narrowed by additional conditions.
func (p Policy) When(conds ...Condition) Policy {
	p.Conditions = append(slices.Clone(p.Conditions), conds...)
	return p
}

// Clone deep-copies a policy so stores can hand out values without aliasing.
func (p Policy) Clone() Policy {
	p.Conditions = slices.Clone(p.Conditions)
	for i := range p.Conditions {
		p.Conditions[i].Values = slices.Clone(p.Conditions[i].Values)
	}
	return p
}

// matches reports whether the policy covers this action/resource pair, ignoring
// conditions (which are evaluated separately so their results reach the trace).
func (p Policy) matches(action Action, r ResourceRef) bool {
	return matchPattern(string(p.Action), string(action)) && matchPattern(p.Resource, r.String())
}
