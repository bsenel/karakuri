package auth

import (
	"context"
	"fmt"
	"slices"
)

// Decision is the outcome of an authorization check together with the trace
// that produced it. Every field exists so a denial can explain itself: which
// policy matched, which role carried it, which binding scope was in play, and
// how each condition evaluated.
type Decision struct {
	Allowed       bool              `json:"allowed"`
	Effect        Effect            `json:"effect,omitempty"`
	PrincipalID   string            `json:"principal_id"`
	Action        Action            `json:"action"`
	Resource      string            `json:"resource"`
	MatchedPolicy string            `json:"matched_policy,omitempty"`
	ViaRole       string            `json:"via_role,omitempty"`
	BindingScope  string            `json:"binding_scope,omitempty"`
	Conditions    []ConditionResult `json:"conditions,omitempty"`
	Reason        string            `json:"reason"`

	// ConsideredRoles lists the roles whose bindings covered the resource. An
	// empty list on a denial is the single most common misconfiguration —
	// the principal holds no binding reaching this resource at all.
	ConsideredRoles []string `json:"considered_roles,omitempty"`
}

// Authorizer answers "may this principal do this to that?".
type Authorizer interface {
	Authorize(ctx context.Context, p Principal, action Action, res ResourceRef) (Decision, error)
}

// StoreAuthorizer evaluates decisions against a Store.
//
// Precedence is deliberately simple and total:
//
//	explicit deny  >  explicit allow  >  default deny
//
// Pattern specificity does NOT break allow/deny ties. A more specific allow can
// never override a broader deny — that is the classic IAM footgun, where adding
// a narrow grant silently punches a hole in a blanket restriction.
type StoreAuthorizer struct {
	store Store
}

// NewAuthorizer returns an Authorizer backed by a Store.
func NewAuthorizer(s Store) *StoreAuthorizer { return &StoreAuthorizer{store: s} }

var _ Authorizer = (*StoreAuthorizer)(nil)

// Authorize evaluates the principal's bindings against the requested action and
// resource. The Principal is passed by value rather than looked up by ID so
// conditions can read the attributes the token carried, without a store round
// trip on the hot path.
func (a *StoreAuthorizer) Authorize(ctx context.Context, p Principal, action Action, res ResourceRef) (Decision, error) {
	d := Decision{
		PrincipalID: p.ID,
		Action:      action,
		Resource:    res.String(),
		Effect:      EffectDeny,
	}

	if p.ID == "" {
		d.Reason = "no principal"
		return d, nil
	}
	if p.Disabled {
		d.Reason = fmt.Sprintf("principal %q is disabled", p.ID)
		return d, nil
	}

	bindings, err := a.store.ListBindings(ctx, p.ID)
	if err != nil {
		return d, fmt.Errorf("list bindings for %q: %w", p.ID, err)
	}
	if len(bindings) == 0 {
		d.Reason = fmt.Sprintf("principal %q holds no role bindings", p.ID)
		return d, nil
	}

	index := map[string]Role{}
	var allow *Decision

	for _, b := range bindings {
		if !b.covers(res) {
			continue
		}
		if err := loadRoleClosure(ctx, a.store, b.Role, index); err != nil {
			return d, fmt.Errorf("resolve role %q: %w", b.Role, err)
		}
		granted, err := EffectivePolicies(b.Role, index)
		if err != nil {
			return d, fmt.Errorf("resolve role %q: %w", b.Role, err)
		}
		if !slices.Contains(d.ConsideredRoles, b.Role) {
			d.ConsideredRoles = append(d.ConsideredRoles, b.Role)
		}

		for _, gp := range granted {
			if !gp.matches(action, res) {
				continue
			}
			results, satisfied := evaluateConditions(gp.Conditions, p, res)
			if !satisfied {
				continue
			}
			if gp.Effect == EffectDeny {
				// Deny wins outright — no need to look at anything else.
				d.Allowed = false
				d.Effect = EffectDeny
				d.MatchedPolicy = gp.ID
				d.ViaRole = gp.ViaRole
				d.BindingScope = b.EffectiveScope()
				d.Conditions = results
				d.Reason = fmt.Sprintf("explicit deny by policy %q via role %q", gp.ID, gp.ViaRole)
				return d, nil
			}
			if allow == nil {
				candidate := d
				candidate.Allowed = true
				candidate.Effect = EffectAllow
				candidate.MatchedPolicy = gp.ID
				candidate.ViaRole = gp.ViaRole
				candidate.BindingScope = b.EffectiveScope()
				candidate.Conditions = results
				candidate.Reason = fmt.Sprintf("allowed by policy %q via role %q", gp.ID, gp.ViaRole)
				allow = &candidate
			}
		}
	}

	if allow != nil {
		allow.ConsideredRoles = d.ConsideredRoles
		return *allow, nil
	}
	if len(d.ConsideredRoles) == 0 {
		d.Reason = fmt.Sprintf("no role binding of %q covers %s", p.ID, res)
		return d, nil
	}
	d.Reason = fmt.Sprintf("no policy grants %s on %s (default deny)", action, res)
	return d, nil
}

// loadRoleClosure loads a role and everything it inherits into index, so
// authorization costs one lookup per role actually in play rather than a full
// ListRoles on every request.
func loadRoleClosure(ctx context.Context, s Store, name string, index map[string]Role) error {
	if _, ok := index[name]; ok {
		return nil
	}
	role, err := s.GetRole(ctx, name)
	if err != nil {
		return err
	}
	index[name] = role
	for _, parent := range role.Inherits {
		if err := loadRoleClosure(ctx, s, parent, index); err != nil {
			return err
		}
	}
	return nil
}
