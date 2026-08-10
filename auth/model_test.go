package auth

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestResourceRefString(t *testing.T) {
	cases := []struct {
		ref  ResourceRef
		want string
	}{
		{Resource("twin", "abc"), "twin:abc"},
		{Collection("twin"), "twin:*"},
		{ResourceRef{}, "*"},
		{ResourceRef{Type: "  ", ID: "abc"}, "*"},
		{ResourceRef{Type: "twin", ID: "  "}, "twin:*"},
	}
	for _, c := range cases {
		if got := c.ref.String(); got != c.want {
			t.Errorf("%#v.String() = %q, want %q", c.ref, got, c.want)
		}
	}
}

func TestResourceRefBuilders(t *testing.T) {
	base := Resource("twin", "abc")
	owned := base.WithOwner("alice")
	if owned.Owner != "alice" || base.Owner != "" {
		t.Fatalf("WithOwner mutated the receiver: base=%+v owned=%+v", base, owned)
	}
	withAttr := owned.WithAttr("env", "prod").WithAttr("team", "eng")
	if withAttr.Attrs["env"] != "prod" || withAttr.Attrs["team"] != "eng" {
		t.Fatalf("WithAttr lost data: %+v", withAttr.Attrs)
	}
	if owned.Attrs != nil {
		t.Fatalf("WithAttr mutated the receiver: %+v", owned.Attrs)
	}
}

func TestPrincipalCloneAndContext(t *testing.T) {
	p := Principal{ID: "alice", Name: "Alice", Kind: KindUser, Attrs: map[string]string{"team": "eng"}}
	clone := p.Clone()
	clone.Attrs["team"] = "hr"
	if p.Attrs["team"] != "eng" {
		t.Fatal("Clone shares the Attrs map with the original")
	}

	if _, ok := PrincipalFromContext(context.Background()); ok {
		t.Fatal("PrincipalFromContext returned ok on a bare context")
	}
	ctx := WithPrincipal(context.Background(), p)
	got, ok := PrincipalFromContext(ctx)
	if !ok || got.ID != "alice" {
		t.Fatalf("PrincipalFromContext = %+v, %v", got, ok)
	}

	var zero Principal
	if zero.Clone().Attrs != nil {
		t.Fatal("Clone of a zero principal invented an Attrs map")
	}
}

func TestPolicyHelpers(t *testing.T) {
	p := Allow("p1", "twin:read", "twin:*")
	if p.Effect != EffectAllow {
		t.Errorf("Allow effect = %q", p.Effect)
	}
	if d := Deny("p2", "auth:write", "*"); d.Effect != EffectDeny {
		t.Errorf("Deny effect = %q", d.Effect)
	}

	cond := Condition{Kind: CondAttrIn, Key: "principal.team", Values: []string{"eng"}}
	narrowed := p.When(cond)
	if len(narrowed.Conditions) != 1 || len(p.Conditions) != 0 {
		t.Fatalf("When mutated the receiver: p=%+v narrowed=%+v", p.Conditions, narrowed.Conditions)
	}

	clone := narrowed.Clone()
	clone.Conditions[0].Values[0] = "hr"
	if narrowed.Conditions[0].Values[0] != "eng" {
		t.Fatal("Clone shares condition Values with the original")
	}
}

func TestPolicyMatches(t *testing.T) {
	p := Allow("p", "twin:*", "twin:abc")
	if !p.matches("twin:read", Resource("twin", "abc")) {
		t.Error("expected match on twin:read / twin:abc")
	}
	if p.matches("loop:start", Resource("twin", "abc")) {
		t.Error("matched a different action type")
	}
	if p.matches("twin:read", Resource("twin", "xyz")) {
		t.Error("matched a different resource ID")
	}
	if p.matches("twin:read", Collection("twin")) {
		t.Error("a single-object policy matched the collection")
	}
}

func TestRoleBindingScope(t *testing.T) {
	if got := (RoleBinding{}).EffectiveScope(); got != "*" {
		t.Errorf("empty scope = %q, want *", got)
	}
	global := RoleBinding{Scope: "*"}
	scoped := RoleBinding{Scope: "twin:abc"}
	typeScoped := RoleBinding{Scope: "twin:*"}

	if !global.covers(Resource("objective", "o1")) {
		t.Error("global scope did not cover objective:o1")
	}
	if !scoped.covers(Resource("twin", "abc")) {
		t.Error("twin:abc scope did not cover twin:abc")
	}
	if scoped.covers(Resource("twin", "xyz")) {
		t.Error("twin:abc scope covered twin:xyz")
	}
	if scoped.covers(Collection("twin")) {
		t.Error("twin:abc scope covered the twin collection")
	}
	if !typeScoped.covers(Collection("twin")) {
		t.Error("twin:* scope did not cover the twin collection")
	}
	if typeScoped.covers(Resource("objective", "o1")) {
		t.Error("twin:* scope covered an objective")
	}
}

func TestEffectivePoliciesInheritance(t *testing.T) {
	roles := []Role{
		{Name: "viewer", Policies: []Policy{Allow("v-read", "twin:read", "*")}},
		{Name: "operator", Inherits: []string{"viewer"}, Policies: []Policy{Allow("o-write", "twin:update", "*")}},
		{Name: "admin", Inherits: []string{"operator"}, Policies: []Policy{Allow("a-all", "*", "*")}},
	}
	index := RoleIndex(roles)

	granted, err := EffectivePolicies("admin", index)
	if err != nil {
		t.Fatalf("EffectivePolicies: %v", err)
	}
	var ids, vias []string
	for _, g := range granted {
		ids = append(ids, g.ID)
		vias = append(vias, g.ViaRole)
	}
	// Breadth-first, nearest role first.
	if !slices.Equal(ids, []string{"a-all", "o-write", "v-read"}) {
		t.Errorf("policy order = %v", ids)
	}
	if !slices.Equal(vias, []string{"admin", "operator", "viewer"}) {
		t.Errorf("attribution = %v", vias)
	}

	if granted, err = EffectivePolicies("viewer", index); err != nil || len(granted) != 1 {
		t.Fatalf("viewer = %v, %v", granted, err)
	}
}

func TestEffectivePoliciesDiamondDedupes(t *testing.T) {
	// Two paths reach "base"; its policy must appear once, attributed to the
	// role that reached it first.
	roles := []Role{
		{Name: "base", Policies: []Policy{Allow("b1", "twin:read", "*")}},
		{Name: "left", Inherits: []string{"base"}},
		{Name: "right", Inherits: []string{"base"}},
		{Name: "top", Inherits: []string{"left", "right"}},
	}
	granted, err := EffectivePolicies("top", RoleIndex(roles))
	if err != nil {
		t.Fatalf("EffectivePolicies: %v", err)
	}
	if len(granted) != 1 || granted[0].ID != "b1" || granted[0].ViaRole != "base" {
		t.Fatalf("diamond inheritance = %+v", granted)
	}
}

func TestEffectivePoliciesUnnamedPoliciesAreNotDeduped(t *testing.T) {
	roles := []Role{{Name: "r", Policies: []Policy{
		{Action: "twin:read", Resource: "*", Effect: EffectAllow},
		{Action: "twin:update", Resource: "*", Effect: EffectAllow},
	}}}
	granted, err := EffectivePolicies("r", RoleIndex(roles))
	if err != nil || len(granted) != 2 {
		t.Fatalf("granted = %+v, err = %v", granted, err)
	}
}

func TestEffectivePoliciesErrors(t *testing.T) {
	index := RoleIndex([]Role{
		{Name: "orphan", Inherits: []string{"missing"}},
		{Name: "a", Inherits: []string{"b"}},
		{Name: "b", Inherits: []string{"a"}},
		{Name: "self", Inherits: []string{"self"}},
	})

	if _, err := EffectivePolicies("nope", index); !errors.Is(err, ErrRoleNotFound) {
		t.Errorf("unknown role = %v, want ErrRoleNotFound", err)
	}
	if _, err := EffectivePolicies("orphan", index); !errors.Is(err, ErrRoleNotFound) {
		t.Errorf("missing parent = %v, want ErrRoleNotFound", err)
	}
	if _, err := EffectivePolicies("a", index); !errors.Is(err, ErrRoleCycle) {
		t.Errorf("cycle = %v, want ErrRoleCycle", err)
	}
	if _, err := EffectivePolicies("self", index); !errors.Is(err, ErrRoleCycle) {
		t.Errorf("self-inheritance = %v, want ErrRoleCycle", err)
	}
}

func TestValidateRoles(t *testing.T) {
	ok := []Role{
		{Name: "viewer"},
		{Name: "operator", Inherits: []string{"viewer"}},
	}
	if err := ValidateRoles(ok); err != nil {
		t.Errorf("ValidateRoles(ok) = %v", err)
	}
	bad := []Role{{Name: "a", Inherits: []string{"b"}}, {Name: "b", Inherits: []string{"a"}}}
	if err := ValidateRoles(bad); !errors.Is(err, ErrRoleCycle) {
		t.Errorf("ValidateRoles(cycle) = %v", err)
	}
}

func TestRoleClone(t *testing.T) {
	r := Role{
		Name:     "r",
		Inherits: []string{"parent"},
		Policies: []Policy{Allow("p", "twin:read", "*").When(Condition{Kind: CondAttrIn, Key: "principal.team", Values: []string{"eng"}})},
	}
	clone := r.Clone()
	clone.Inherits[0] = "other"
	clone.Policies[0].Conditions[0].Values[0] = "hr"
	if r.Inherits[0] != "parent" || r.Policies[0].Conditions[0].Values[0] != "eng" {
		t.Fatal("Role.Clone shares state with the original")
	}
}
