package auth

import (
	"errors"
	"slices"
	"testing"
)

func TestMatchPattern(t *testing.T) {
	cases := []struct {
		pattern, value string
		want           bool
	}{
		{"*", "twin:create", true},
		{"*", "anything", true},
		{"twin:*", "twin:create", true},
		{"twin:*", "twin:*", true},
		{"twin:*", "twins:create", false},
		{"twin:*", "objective:create", false},
		{"twin:create", "twin:create", true},
		{"twin:create", "twin:read", false},
		{"twin:abc", "twin:*", false}, // a single-object grant must not cover the collection
		{"", "twin:create", false},
	}
	for _, c := range cases {
		if got := matchPattern(c.pattern, c.value); got != c.want {
			t.Errorf("matchPattern(%q, %q) = %v, want %v", c.pattern, c.value, got, c.want)
		}
	}
}

func TestValidatePattern(t *testing.T) {
	valid := []string{"*", "twin:*", "twin:create", "loop:resume"}
	for _, s := range valid {
		if err := validatePattern(s); err != nil {
			t.Errorf("validatePattern(%q) = %v, want nil", s, err)
		}
	}
	invalid := []string{"", "twin", "twin:", ":create", "tw*in:create", "*:create", ":*", "twin:cre*ate"}
	for _, s := range invalid {
		if err := validatePattern(s); !errors.Is(err, ErrInvalidPattern) {
			t.Errorf("validatePattern(%q) = %v, want ErrInvalidPattern", s, err)
		}
	}
}

func TestActionTypeAndVerb(t *testing.T) {
	if got := Action("twin:create").Type(); got != "twin" {
		t.Errorf("Type() = %q, want twin", got)
	}
	if got := Action("twin:create").Verb(); got != "create" {
		t.Errorf("Verb() = %q, want create", got)
	}
	if got := Action("*").Type(); got != "" {
		t.Errorf("Type() on wildcard = %q, want empty", got)
	}
	if got := Action("*").Verb(); got != "" {
		t.Errorf("Verb() on wildcard = %q, want empty", got)
	}
}

func TestCatalogRegister(t *testing.T) {
	c := NewCatalog()
	if err := c.Register("twin:create", "create a twin"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// Idempotent with the same description.
	if err := c.Register("twin:create", "create a twin"); err != nil {
		t.Fatalf("re-Register: %v", err)
	}
	// Conflicting description is an error — two subsystems disagreeing is a bug.
	if err := c.Register("twin:create", "something else"); err == nil {
		t.Fatal("expected error re-registering with a different description")
	}
	if !c.Has("twin:create") {
		t.Error("Has(twin:create) = false")
	}
	if c.Has("twin:delete") {
		t.Error("Has(twin:delete) = true for unregistered action")
	}
	if desc, ok := c.Describe("twin:create"); !ok || desc != "create a twin" {
		t.Errorf("Describe = %q, %v", desc, ok)
	}
	if _, ok := c.Describe("nope:nope"); ok {
		t.Error("Describe returned ok for unregistered action")
	}

	for _, bad := range []Action{"", " twin:create", "twin", "twin:", ":create", "twin:*", "*:create"} {
		if err := c.Register(bad, "x"); !errors.Is(err, ErrInvalidPattern) {
			t.Errorf("Register(%q) = %v, want ErrInvalidPattern", bad, err)
		}
	}
}

func TestCatalogMustRegisterPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustRegister did not panic on an invalid action")
		}
	}()
	NewCatalog().MustRegister("nope", "x")
}

func TestCatalogMustRegisterSucceeds(t *testing.T) {
	c := NewCatalog()
	c.MustRegister("twin:read", "read")
	if !c.Has("twin:read") {
		t.Fatal("MustRegister did not register the action")
	}
}

func testCatalog(t *testing.T) *Catalog {
	t.Helper()
	c := NewCatalog()
	for _, a := range []Action{"twin:create", "twin:read", "twin:update", "loop:start", "audit:read"} {
		c.MustRegister(a, string(a))
	}
	return c
}

func TestCatalogActionsAndExpand(t *testing.T) {
	c := testCatalog(t)
	want := []Action{"audit:read", "loop:start", "twin:create", "twin:read", "twin:update"}
	if got := c.Actions(); !slices.Equal(got, want) {
		t.Errorf("Actions() = %v, want %v", got, want)
	}
	if got := c.Expand("twin:*"); !slices.Equal(got, []Action{"twin:create", "twin:read", "twin:update"}) {
		t.Errorf("Expand(twin:*) = %v", got)
	}
	if got := c.Expand("twin:read"); !slices.Equal(got, []Action{"twin:read"}) {
		t.Errorf("Expand(twin:read) = %v", got)
	}
	if got := c.Expand("*"); len(got) != 5 {
		t.Errorf("Expand(*) returned %d actions, want 5", len(got))
	}
	if got := c.Expand("nope:*"); len(got) != 0 {
		t.Errorf("Expand(nope:*) = %v, want empty", got)
	}
}

func TestCatalogValidatePolicy(t *testing.T) {
	c := testCatalog(t)

	if err := c.ValidatePolicy(Allow("p1", "twin:*", "twin:*")); err != nil {
		t.Errorf("valid policy rejected: %v", err)
	}
	if err := c.ValidatePolicy(Allow("p2", "twin:read", "twin:abc").When(Condition{Kind: CondOwnerEquals})); err != nil {
		t.Errorf("valid conditional policy rejected: %v", err)
	}

	// Action pattern matching nothing in the catalog is almost always a typo.
	if err := c.ValidatePolicy(Allow("p3", "twim:*", "*")); !errors.Is(err, ErrUnknownAction) {
		t.Errorf("unknown action = %v, want ErrUnknownAction", err)
	}
	if err := c.ValidatePolicy(Allow("p4", "twin:delete", "*")); !errors.Is(err, ErrUnknownAction) {
		t.Errorf("uncatalogued action = %v, want ErrUnknownAction", err)
	}
	// Malformed patterns.
	if err := c.ValidatePolicy(Allow("p5", "twin", "*")); !errors.Is(err, ErrInvalidPattern) {
		t.Errorf("bad action pattern = %v", err)
	}
	if err := c.ValidatePolicy(Allow("p6", "twin:read", "tw*in")); !errors.Is(err, ErrInvalidPattern) {
		t.Errorf("bad resource pattern = %v", err)
	}
	if err := c.ValidatePolicy(Policy{ID: "p7", Action: "twin:read", Resource: "*", Effect: "maybe"}); !errors.Is(err, ErrInvalidPattern) {
		t.Errorf("bad effect = %v", err)
	}
	if err := c.ValidatePolicy(Allow("p8", "twin:read", "*").When(Condition{Kind: "made_up"})); !errors.Is(err, ErrInvalidPattern) {
		t.Errorf("bad condition = %v", err)
	}
}

func TestCatalogValidateRole(t *testing.T) {
	c := testCatalog(t)
	good := Role{Name: "viewer", Policies: []Policy{Allow("v1", "twin:read", "*")}}
	if err := c.ValidateRole(good); err != nil {
		t.Errorf("ValidateRole(good) = %v", err)
	}
	bad := Role{Name: "broken", Policies: []Policy{Allow("b1", "nope:read", "*")}}
	if err := c.ValidateRole(bad); !errors.Is(err, ErrUnknownAction) {
		t.Errorf("ValidateRole(bad) = %v, want ErrUnknownAction", err)
	}
}
