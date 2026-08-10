package auth

// RoleBinding grants a principal a role over a scope. The scope is what makes
// authorization fine-grained rather than global: "alice is an operator" and
// "alice is an operator on twin:abc" differ only in this field.
//
// Scope uses the same pattern grammar as policy resources — "*", "twin:*" or
// "twin:abc". An empty scope means "*", so a binding created without thinking
// about scope behaves like plain RBAC.
type RoleBinding struct {
	ID          string `json:"id"`
	PrincipalID string `json:"principal_id"`
	Role        string `json:"role"`
	Scope       string `json:"scope,omitempty"`
}

// EffectiveScope returns the binding's scope, defaulting to "*".
func (b RoleBinding) EffectiveScope() string {
	if b.Scope == "" {
		return "*"
	}
	return b.Scope
}

// covers reports whether the binding's scope reaches this resource. A binding
// that does not cover the target contributes no policies at all — its role is
// simply not in play for that request.
func (b RoleBinding) covers(r ResourceRef) bool {
	return matchPattern(b.EffectiveScope(), r.String())
}
