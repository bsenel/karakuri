package auth

import (
	"fmt"
	"slices"
	"strings"
	"sync"
)

// Action names an operation in "<type>:<verb>" form, e.g. "twin:create". The
// same string space is used for policy patterns, where the verb (or the whole
// action) may be "*".
type Action string

// Type returns the resource type an action applies to ("twin" for
// "twin:create"). It is empty for the bare "*" wildcard.
func (a Action) Type() string {
	typ, _, ok := strings.Cut(string(a), ":")
	if !ok {
		return ""
	}
	return typ
}

// Verb returns the operation part of an action ("create" for "twin:create").
func (a Action) Verb() string {
	_, verb, ok := strings.Cut(string(a), ":")
	if !ok {
		return ""
	}
	return verb
}

// Catalog is the exhaustive set of actions a deployment recognises. Nothing is
// implicit: a policy naming an action that was never registered is rejected, so
// a typo in a role definition fails loudly at seed time instead of silently
// granting or withholding access.
//
// The catalog lives here rather than being hard-coded because this module knows
// nothing about any particular application's resources — the consumer registers
// its own action set at startup.
type Catalog struct {
	mu      sync.RWMutex
	actions map[Action]string
}

// NewCatalog returns an empty catalog.
func NewCatalog() *Catalog { return &Catalog{actions: map[Action]string{}} }

// Register adds an action with a human-readable description. Re-registering the
// same action with the same description is a no-op; changing the description of
// an existing action is an error, since two subsystems disagreeing about what
// an action means is a bug worth surfacing.
func (c *Catalog) Register(a Action, description string) error {
	if err := validateActionName(a); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.actions[a]; ok && existing != description {
		return fmt.Errorf("auth: action %q already registered with a different description", a)
	}
	c.actions[a] = description
	return nil
}

// MustRegister is Register for package-level catalogs built at init time.
func (c *Catalog) MustRegister(a Action, description string) {
	if err := c.Register(a, description); err != nil {
		panic(err)
	}
}

// Has reports whether an exact action is registered.
func (c *Catalog) Has(a Action) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.actions[a]
	return ok
}

// Describe returns an action's description.
func (c *Catalog) Describe(a Action) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	d, ok := c.actions[a]
	return d, ok
}

// Actions returns every registered action, sorted.
func (c *Catalog) Actions() []Action {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Action, 0, len(c.actions))
	for a := range c.actions {
		out = append(out, a)
	}
	slices.Sort(out)
	return out
}

// Expand returns every registered action matching a pattern, sorted. It is what
// turns a role's "twin:*" policy into the concrete permission list shown by
// an effective-permissions endpoint.
func (c *Catalog) Expand(pattern Action) []Action {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Action, 0, len(c.actions))
	for a := range c.actions {
		if matchPattern(string(pattern), string(a)) {
			out = append(out, a)
		}
	}
	slices.Sort(out)
	return out
}

// ValidatePolicy checks that a policy's action pattern matches at least one
// registered action and that its patterns are well formed. A pattern matching
// nothing is almost always a typo.
func (c *Catalog) ValidatePolicy(p Policy) error {
	if err := validatePattern(string(p.Action)); err != nil {
		return fmt.Errorf("policy %q action: %w", p.ID, err)
	}
	if err := validatePattern(p.Resource); err != nil {
		return fmt.Errorf("policy %q resource: %w", p.ID, err)
	}
	if p.Effect != EffectAllow && p.Effect != EffectDeny {
		return fmt.Errorf("policy %q: %w: effect %q", p.ID, ErrInvalidPattern, p.Effect)
	}
	for _, cond := range p.Conditions {
		if err := cond.Validate(); err != nil {
			return fmt.Errorf("policy %q: %w", p.ID, err)
		}
	}
	if len(c.Expand(p.Action)) == 0 {
		return fmt.Errorf("policy %q action %q: %w", p.ID, p.Action, ErrUnknownAction)
	}
	return nil
}

// ValidateRole validates every policy on a role.
func (c *Catalog) ValidateRole(r Role) error {
	for _, p := range r.Policies {
		if err := c.ValidatePolicy(p); err != nil {
			return fmt.Errorf("role %q: %w", r.Name, err)
		}
	}
	return nil
}

// validateActionName rejects patterns where a concrete action is required.
func validateActionName(a Action) error {
	s := strings.TrimSpace(string(a))
	if s == "" || s != string(a) {
		return fmt.Errorf("%w: action %q", ErrInvalidPattern, a)
	}
	typ, verb, ok := strings.Cut(s, ":")
	if !ok || typ == "" || verb == "" {
		return fmt.Errorf("%w: action %q must be \"<type>:<verb>\"", ErrInvalidPattern, a)
	}
	if strings.Contains(typ, "*") || strings.Contains(verb, "*") {
		return fmt.Errorf("%w: catalog action %q must not contain a wildcard", ErrInvalidPattern, a)
	}
	return nil
}

// validatePattern accepts "*", "<prefix>:*" and exact "<type>:<verb>" forms.
func validatePattern(s string) error {
	if s == "" {
		return fmt.Errorf("%w: empty pattern", ErrInvalidPattern)
	}
	if s == "*" {
		return nil
	}
	if strings.HasSuffix(s, ":*") {
		if prefix := strings.TrimSuffix(s, ":*"); prefix != "" && !strings.Contains(prefix, "*") {
			return nil
		}
		return fmt.Errorf("%w: %q", ErrInvalidPattern, s)
	}
	if strings.Contains(s, "*") {
		return fmt.Errorf("%w: %q — wildcards are only allowed as a trailing \":*\"", ErrInvalidPattern, s)
	}
	if typ, verb, ok := strings.Cut(s, ":"); !ok || typ == "" || verb == "" {
		return fmt.Errorf("%w: %q must be \"<type>:<verb>\", \"<type>:*\" or \"*\"", ErrInvalidPattern, s)
	}
	return nil
}

// matchPattern implements the one matching rule used for actions, resources and
// binding scopes alike: exact match, the bare "*", or a trailing ":*" prefix
// glob. Deliberately not a full glob language — every pattern in a policy has to
// be readable by whoever is auditing it.
func matchPattern(pattern, value string) bool {
	if pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, ":*") {
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == value
}
