package auth

import (
	"fmt"
	"slices"
	"strings"
)

// ConditionKind enumerates the supported condition types. This is a closed set
// rather than an expression language on purpose: every condition has to be
// readable by whoever audits the policy, and evaluation has to be total — no
// parse errors at request time.
type ConditionKind string

const (
	// CondOwnerEquals holds when the resource's Owner is the acting principal.
	// A resource with no owner never satisfies it, so ownership-scoped grants
	// do not silently cover unowned legacy rows.
	CondOwnerEquals ConditionKind = "owner_equals"

	// CondAttrEquals holds when the attribute named by Key equals Values[0].
	CondAttrEquals ConditionKind = "attr_equals"

	// CondAttrIn holds when the attribute named by Key is one of Values.
	CondAttrIn ConditionKind = "attr_in"
)

// Condition narrows a policy beyond its action and resource patterns. Keys are
// namespaced: "principal.<attr>" reads the acting principal, "resource.<attr>"
// reads the target. The pseudo-attributes principal.id, principal.name,
// principal.kind, resource.type, resource.id and resource.owner are always
// available.
type Condition struct {
	Kind   ConditionKind `json:"kind"`
	Key    string        `json:"key,omitempty"`
	Values []string      `json:"values,omitempty"`
}

// ConditionResult records how one condition evaluated, so a denial can explain
// itself instead of just saying "no".
type ConditionResult struct {
	Condition Condition `json:"condition"`
	Satisfied bool      `json:"satisfied"`
	Detail    string    `json:"detail,omitempty"`
}

// Validate checks a condition is well formed. Called at seed time so a bad
// policy fails on startup rather than at the first request that would match it.
func (c Condition) Validate() error {
	switch c.Kind {
	case CondOwnerEquals:
		return nil
	case CondAttrEquals:
		if c.Key == "" || len(c.Values) != 1 {
			return fmt.Errorf("%w: %s needs a key and exactly one value", ErrInvalidPattern, c.Kind)
		}
	case CondAttrIn:
		if c.Key == "" || len(c.Values) == 0 {
			return fmt.Errorf("%w: %s needs a key and at least one value", ErrInvalidPattern, c.Kind)
		}
	default:
		return fmt.Errorf("%w: unknown condition kind %q", ErrInvalidPattern, c.Kind)
	}
	if _, _, err := splitAttrKey(c.Key); err != nil {
		return err
	}
	return nil
}

// Evaluate resolves a condition against the acting principal and the target
// resource. It never fails: an unresolvable key is an unsatisfied condition
// with a detail explaining why.
func (c Condition) Evaluate(p Principal, r ResourceRef) ConditionResult {
	res := ConditionResult{Condition: c}
	switch c.Kind {
	case CondOwnerEquals:
		switch r.Owner {
		case "":
			res.Detail = fmt.Sprintf("resource %s has no owner", r)
		case p.ID:
			res.Satisfied = true
			res.Detail = fmt.Sprintf("owner %q matches principal", r.Owner)
		default:
			res.Detail = fmt.Sprintf("owner %q is not principal %q", r.Owner, p.ID)
		}
	case CondAttrEquals, CondAttrIn:
		got, ok := lookupAttr(c.Key, p, r)
		if !ok {
			res.Detail = fmt.Sprintf("attribute %q is not set", c.Key)
			return res
		}
		if slices.Contains(c.Values, got) {
			res.Satisfied = true
			res.Detail = fmt.Sprintf("%s=%q", c.Key, got)
		} else {
			res.Detail = fmt.Sprintf("%s=%q is not in [%s]", c.Key, got, strings.Join(c.Values, " "))
		}
	default:
		res.Detail = fmt.Sprintf("unknown condition kind %q", c.Kind)
	}
	return res
}

// evaluateConditions returns the per-condition trace plus whether all held. An
// empty condition list is unconditionally satisfied.
func evaluateConditions(conds []Condition, p Principal, r ResourceRef) ([]ConditionResult, bool) {
	if len(conds) == 0 {
		return nil, true
	}
	out := make([]ConditionResult, 0, len(conds))
	all := true
	for _, c := range conds {
		res := c.Evaluate(p, r)
		if !res.Satisfied {
			all = false
		}
		out = append(out, res)
	}
	return out, all
}

func splitAttrKey(key string) (namespace, attr string, err error) {
	namespace, attr, ok := strings.Cut(key, ".")
	if !ok || attr == "" {
		return "", "", fmt.Errorf("%w: condition key %q must be \"principal.<attr>\" or \"resource.<attr>\"", ErrInvalidPattern, key)
	}
	if namespace != "principal" && namespace != "resource" {
		return "", "", fmt.Errorf("%w: condition key %q has unknown namespace %q", ErrInvalidPattern, key, namespace)
	}
	return namespace, attr, nil
}

func lookupAttr(key string, p Principal, r ResourceRef) (string, bool) {
	namespace, attr, err := splitAttrKey(key)
	if err != nil {
		return "", false
	}
	if namespace == "principal" {
		switch attr {
		case "id":
			return p.ID, p.ID != ""
		case "name":
			return p.Name, p.Name != ""
		case "kind":
			return string(p.Kind), p.Kind != ""
		}
		v, ok := p.Attrs[attr]
		return v, ok
	}
	switch attr {
	case "type":
		return r.Type, r.Type != ""
	case "id":
		return r.ID, r.ID != ""
	case "owner":
		return r.Owner, r.Owner != ""
	}
	v, ok := r.Attrs[attr]
	return v, ok
}
