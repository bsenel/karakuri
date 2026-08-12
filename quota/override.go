package quota

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

// Overrides raise (or lower) one subject's limit without a redeploy.
//
// A Policy and a Quota are configuration: they are read at startup and they are
// the same for everybody. That is the right default and the wrong answer to
// "the launch is Thursday and we need double for a week", which otherwise means
// editing a file, restarting a process, and remembering to put it back.
//
// An override is the exception, stored beside the counters rather than in the
// configuration file, and it is deliberately a *replacement* rather than a
// multiplier — an operator approving a limit should be able to read the number
// they are approving.

// Override replaces one named limit for one subject.
type Override struct {
	// Subject is the key the limit applies to, in whatever shape the caller's
	// keys take: "principal|alice", "twin|t_7f2a". Opaque here, as always.
	Subject Key `json:"subject"`

	// Name selects which limit this replaces. It matches Quota.Name for a
	// quota, and whatever name the caller gave a Policy for a rate limit —
	// this package does not name limits, so the caller's naming is the only
	// naming there is.
	Name string `json:"name"`

	// Cap is the new ceiling: Quota.Cap or Policy.Limit.
	Cap int `json:"cap"`

	// Window replaces Policy.Window when non-zero. It is ignored for quotas,
	// whose period is a calendar span rather than a duration — changing "daily"
	// to "every 36 hours" is not a thing a quota can express, and pretending
	// otherwise would put a second calendar in the key.
	Window time.Duration `json:"window,omitempty"`

	// ExpiresAt is when the override stops applying. Zero means until somebody
	// removes it.
	//
	// Both are ordinary: a raise for a launch week, and a raise because the team
	// grew. Expiry is here so the first does not silently become the second.
	ExpiresAt time.Time `json:"expires_at,omitempty"`

	// Reason is free text an operator wrote. It is carried so that reading the
	// overrides on a system answers "why is this one different" without a
	// separate audit query.
	Reason string `json:"reason,omitempty"`
}

// Validate reports whether the override is usable.
func (o Override) Validate() error {
	if o.Subject == "" {
		return fmt.Errorf("%w: override has no subject", ErrInvalidPolicy)
	}
	if strings.TrimSpace(o.Name) == "" {
		return fmt.Errorf("%w: override for %q names no limit", ErrInvalidPolicy, o.Subject)
	}
	if o.Cap <= 0 {
		return fmt.Errorf("%w: override %q cap must be positive, got %d", ErrInvalidPolicy, o.Name, o.Cap)
	}
	if o.Window < 0 {
		return fmt.Errorf("%w: override %q window must not be negative, got %s", ErrInvalidPolicy, o.Name, o.Window)
	}
	return nil
}

// Active reports whether the override applies at now.
func (o Override) Active(now time.Time) bool {
	return o.ExpiresAt.IsZero() || now.Before(o.ExpiresAt)
}

// Apply returns p with this override's ceiling, leaving the algorithm alone.
//
// The algorithm is not overridable on purpose. It is a decision about the shape
// of the traffic — smooth it, or count it in windows — and an operator raising a
// ceiling is not making that decision. Letting a request change it would turn a
// "please give me more" workflow into a way to swap the enforcement model.
func (o Override) Apply(p Policy) Policy {
	p.Limit = o.Cap
	if o.Window > 0 {
		p.Window = o.Window
		// Rate was derived from the old Limit and Window, so a raise that keeps
		// an explicit rate would refill at the old speed into a bigger bucket.
		// Clearing it re-derives from the pair the operator actually approved.
		p.Rate = 0
	}
	return p
}

// ApplyQuota returns q with this override's cap. The period is untouched — see
// the note on Window.
func (o Override) ApplyQuota(q Quota) Quota {
	q.Cap = o.Cap
	return q
}

// OverrideStore holds the overrides in force.
//
// It is separate from Backend because the two have opposite access patterns: a
// backend write happens on every request and must be atomic per key, while an
// override changes when a human approves something and is read constantly. That
// difference is what Resolver's cache exists to exploit.
type OverrideStore interface {
	// Overrides returns every override for a subject, including expired ones —
	// filtering is the caller's job, so that a store need not be told the time.
	Overrides(ctx context.Context, subject Key) ([]Override, error)

	// PutOverride stores one, replacing any existing override with the same
	// subject and name.
	PutOverride(ctx context.Context, o Override) error

	// DeleteOverride removes one. Removing an override that does not exist is
	// not an error.
	DeleteOverride(ctx context.Context, subject Key, name string) error

	// ListOverrides returns every override in the store, for an operator
	// reading what has been granted.
	ListOverrides(ctx context.Context) ([]Override, error)
}

// MemoryOverrideStore keeps overrides in this process.
//
// It is the reference implementation, and it is genuinely useful on a single
// replica — but overrides granted through it are lost on restart, which is
// usually not what somebody approving a limit expects. Persistent deployments
// want quota/sql.
//
// Zero value is not usable; call NewMemoryOverrideStore.
type MemoryOverrideStore struct {
	mu sync.Mutex
	by map[Key]map[string]Override
}

// NewMemoryOverrideStore returns an empty in-process override store.
func NewMemoryOverrideStore() *MemoryOverrideStore {
	return &MemoryOverrideStore{by: map[Key]map[string]Override{}}
}

var _ OverrideStore = (*MemoryOverrideStore)(nil)

func (s *MemoryOverrideStore) Overrides(_ context.Context, subject Key) ([]Override, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sortedOverrides(s.by[subject]), nil
}

func (s *MemoryOverrideStore) PutOverride(_ context.Context, o Override) error {
	if err := o.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.by[o.Subject] == nil {
		s.by[o.Subject] = map[string]Override{}
	}
	s.by[o.Subject][o.Name] = o
	return nil
}

func (s *MemoryOverrideStore) DeleteOverride(_ context.Context, subject Key, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.by[subject], name)
	if len(s.by[subject]) == 0 {
		delete(s.by, subject)
	}
	return nil
}

func (s *MemoryOverrideStore) ListOverrides(_ context.Context) ([]Override, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Override
	for _, byName := range s.by {
		out = append(out, sortedOverrides(byName)...)
	}
	slices.SortFunc(out, func(a, b Override) int {
		if a.Subject != b.Subject {
			return strings.Compare(string(a.Subject), string(b.Subject))
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
}

// sortedOverrides copies a subject's overrides out under the lock, in a stable
// order. Copying is the store contract everywhere in this repository: a caller
// must never be able to mutate stored state through a returned value.
func sortedOverrides(byName map[string]Override) []Override {
	if len(byName) == 0 {
		return nil
	}
	out := make([]Override, 0, len(byName))
	for _, o := range byName {
		out = append(out, o)
	}
	slices.SortFunc(out, func(a, b Override) int { return strings.Compare(a.Name, b.Name) })
	return out
}
