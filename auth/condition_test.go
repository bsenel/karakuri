package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestConditionValidate(t *testing.T) {
	valid := []Condition{
		{Kind: CondOwnerEquals},
		{Kind: CondAttrEquals, Key: "principal.team", Values: []string{"eng"}},
		{Kind: CondAttrIn, Key: "resource.env", Values: []string{"dev", "staging"}},
	}
	for _, c := range valid {
		if err := c.Validate(); err != nil {
			t.Errorf("Validate(%+v) = %v", c, err)
		}
	}

	invalid := []Condition{
		{Kind: "nonsense"},
		{Kind: CondAttrEquals, Key: "", Values: []string{"eng"}},
		{Kind: CondAttrEquals, Key: "principal.team"},
		{Kind: CondAttrEquals, Key: "principal.team", Values: []string{"a", "b"}},
		{Kind: CondAttrIn, Key: "principal.team"},
		{Kind: CondAttrIn, Key: "team", Values: []string{"eng"}},          // no namespace
		{Kind: CondAttrIn, Key: "user.team", Values: []string{"eng"}},     // unknown namespace
		{Kind: CondAttrEquals, Key: "principal.", Values: []string{"eng"}}, // empty attr
	}
	for _, c := range invalid {
		if err := c.Validate(); !errors.Is(err, ErrInvalidPattern) {
			t.Errorf("Validate(%+v) = %v, want ErrInvalidPattern", c, err)
		}
	}
}

func TestConditionOwnerEquals(t *testing.T) {
	alice := Principal{ID: "alice"}
	cond := Condition{Kind: CondOwnerEquals}

	res := cond.Evaluate(alice, Resource("twin", "t1").WithOwner("alice"))
	if !res.Satisfied {
		t.Errorf("owner match not satisfied: %s", res.Detail)
	}
	res = cond.Evaluate(alice, Resource("twin", "t1").WithOwner("bob"))
	if res.Satisfied || !strings.Contains(res.Detail, "bob") {
		t.Errorf("owner mismatch = %+v", res)
	}
	// An unowned resource never satisfies owner_equals, so ownership-scoped
	// grants do not silently cover legacy rows that predate ownership.
	res = cond.Evaluate(alice, Resource("twin", "t1"))
	if res.Satisfied || !strings.Contains(res.Detail, "no owner") {
		t.Errorf("unowned resource = %+v", res)
	}
}

func TestConditionAttrs(t *testing.T) {
	p := Principal{ID: "alice", Name: "Alice", Kind: KindUser, Attrs: map[string]string{"team": "eng"}}
	r := Resource("twin", "t1").WithOwner("alice").WithAttr("env", "prod")

	cases := []struct {
		name string
		cond Condition
		want bool
	}{
		{"principal attr hit", Condition{Kind: CondAttrEquals, Key: "principal.team", Values: []string{"eng"}}, true},
		{"principal attr miss", Condition{Kind: CondAttrEquals, Key: "principal.team", Values: []string{"hr"}}, false},
		{"principal id", Condition{Kind: CondAttrEquals, Key: "principal.id", Values: []string{"alice"}}, true},
		{"principal name", Condition{Kind: CondAttrEquals, Key: "principal.name", Values: []string{"Alice"}}, true},
		{"principal kind", Condition{Kind: CondAttrEquals, Key: "principal.kind", Values: []string{"user"}}, true},
		{"resource type", Condition{Kind: CondAttrEquals, Key: "resource.type", Values: []string{"twin"}}, true},
		{"resource id", Condition{Kind: CondAttrEquals, Key: "resource.id", Values: []string{"t1"}}, true},
		{"resource owner", Condition{Kind: CondAttrEquals, Key: "resource.owner", Values: []string{"alice"}}, true},
		{"resource attr", Condition{Kind: CondAttrIn, Key: "resource.env", Values: []string{"dev", "prod"}}, true},
		{"resource attr miss", Condition{Kind: CondAttrIn, Key: "resource.env", Values: []string{"dev"}}, false},
		{"unset attr", Condition{Kind: CondAttrEquals, Key: "principal.region", Values: []string{"eu"}}, false},
		{"bad key", Condition{Kind: CondAttrEquals, Key: "nope", Values: []string{"x"}}, false},
		{"unknown kind", Condition{Kind: "made_up"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.cond.Evaluate(p, r); got.Satisfied != c.want {
				t.Errorf("Satisfied = %v, want %v (detail: %s)", got.Satisfied, c.want, got.Detail)
			}
		})
	}
}

func TestConditionUnsetPseudoAttrs(t *testing.T) {
	// Pseudo-attributes report "not set" rather than matching the empty string.
	empty := Principal{}
	bare := ResourceRef{}
	for _, key := range []string{"principal.id", "principal.name", "principal.kind", "resource.type", "resource.id", "resource.owner"} {
		cond := Condition{Kind: CondAttrEquals, Key: key, Values: []string{""}}
		res := cond.Evaluate(empty, bare)
		if res.Satisfied || !strings.Contains(res.Detail, "not set") {
			t.Errorf("%s = %+v, want unsatisfied/not set", key, res)
		}
	}
}

func TestEvaluateConditions(t *testing.T) {
	p := Principal{ID: "alice", Attrs: map[string]string{"team": "eng"}}
	r := Resource("twin", "t1").WithOwner("alice")

	if results, ok := evaluateConditions(nil, p, r); !ok || results != nil {
		t.Errorf("no conditions = %v, %v; want nil, true", results, ok)
	}

	all := []Condition{
		{Kind: CondOwnerEquals},
		{Kind: CondAttrEquals, Key: "principal.team", Values: []string{"eng"}},
	}
	results, ok := evaluateConditions(all, p, r)
	if !ok || len(results) != 2 {
		t.Fatalf("all satisfied = %v, %d results", ok, len(results))
	}

	mixed := append(all, Condition{Kind: CondAttrEquals, Key: "principal.team", Values: []string{"hr"}})
	results, ok = evaluateConditions(mixed, p, r)
	if ok {
		t.Error("expected the failing condition to fail the set")
	}
	if len(results) != 3 || results[2].Satisfied {
		t.Errorf("trace = %+v", results)
	}
}
