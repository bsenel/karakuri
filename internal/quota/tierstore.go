package quota

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Where the limits actually come from.
//
// Until Phase 19 a tier was boot state: DefaultTiers read the YAML once and the
// result was frozen into Deps for the life of the process. Raising a limit meant
// editing a file and restarting, which is the same problem self-service solved
// for one subject in Phase 18 — left unsolved for everybody.
//
// A stored tier is the answer, and it is deliberately the *base* rather than an
// override: "everybody now gets a million tokens" is a different statement from
// "this twin gets five million until Friday", and conflating them would put two
// kinds of thing in one table with no way to tell them apart in a report.

// Tier is one limit as an operator set it, rather than as configuration shipped
// it.
//
// It carries a ceiling and, for a rate limit, the shape of the window. It
// deliberately does not carry the algorithm or the calendar period: changing
// how a limit is counted is a decision about the shape of the traffic, and
// somebody typing a bigger number is not choosing to swap fixed windows for a
// token bucket. That is the same line Override.Apply draws, for the same reason.
type Tier struct {
	// Name matches the tier it replaces: request, capability, llm-tokens or
	// adapter. It is also the name an override is looked up by, so the two
	// mechanisms agree about what they are talking about.
	Name string `json:"name"`

	// Cap is the new ceiling. For the request tier that is the burst capacity,
	// which is what Policy.Limit means for a token bucket.
	Cap int `json:"cap"`

	// Window and Rate apply to the request tier only. Rate is the sustained
	// per-second refill; zero derives it from Cap and Window, which is what
	// "sixty a minute bursting to sixty" means.
	Window time.Duration `json:"window,omitempty"`
	Rate   float64       `json:"rate,omitempty"`

	// Reason is why, in the operator's words. Required, for the same reason an
	// override needs one: a limit changed for a reason nobody wrote down is one
	// nobody can review later — and unlike an override, this one changed the
	// limit for everybody.
	Reason    string    `json:"reason"`
	UpdatedBy string    `json:"updated_by"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TierNames are the limits that can be stored, which are exactly the ones that
// can be enforced. Anything else is refused at write time rather than accepted
// and then never read.
func TierNames() []string {
	return []string{TierAdapter, TierCapability, TierLLMTokens, TierRequest}
}

// Validate reports whether the tier is usable as stored.
func (t Tier) Validate() error {
	if !storableTier[t.Name] {
		return fmt.Errorf("unknown tier %q; one of %s", t.Name, strings.Join(TierNames(), ", "))
	}
	if t.Cap <= 0 {
		return fmt.Errorf("tier %q needs a positive cap", t.Name)
	}
	if strings.TrimSpace(t.Reason) == "" {
		return fmt.Errorf("tier %q needs a reason: this changes the limit for everybody", t.Name)
	}
	if t.Name != TierRequest && (t.Window != 0 || t.Rate != 0) {
		// A daily quota's period is a calendar span, not a duration. Accepting
		// a window here would store a number nothing reads.
		return fmt.Errorf("tier %q is a daily quota; window and rate apply to the request tier only", t.Name)
	}
	if t.Name == TierRequest && t.Window <= 0 {
		return fmt.Errorf("tier %q needs a window", t.Name)
	}
	return nil
}

var storableTier = map[string]bool{
	TierRequest: true, TierCapability: true, TierLLMTokens: true, TierAdapter: true,
}

// TierStore holds the limits in force.
//
// Reading returns every stored tier at once rather than one at a time: there
// are four, they are read together on the hot path, and one round trip that
// fills the cache beats four that each fill a quarter of it.
type TierStore interface {
	Tiers(ctx context.Context) ([]Tier, error)
	PutTier(ctx context.Context, t Tier) error

	// DeleteTier returns a tier to whatever configuration says. It is the
	// "reset" an operator reaches for after an experiment, and it is why the
	// YAML keeps meaning something.
	DeleteTier(ctx context.Context, name string) error
}

// MemoryTierStore is a TierStore for tests and for a deployment with no
// database, where it holds nothing and every read returns configuration.
type MemoryTierStore struct {
	mu    sync.RWMutex
	tiers map[string]Tier
}

func NewMemoryTierStore() *MemoryTierStore {
	return &MemoryTierStore{tiers: map[string]Tier{}}
}

func (s *MemoryTierStore) Tiers(context.Context) ([]Tier, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Tier, 0, len(s.tiers))
	for _, t := range s.tiers {
		out = append(out, t)
	}
	return out, nil
}

func (s *MemoryTierStore) PutTier(_ context.Context, t Tier) error {
	if err := t.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tiers[t.Name] = t
	return nil
}

func (s *MemoryTierStore) DeleteTier(_ context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tiers, name)
	return nil
}

// SQLTierStore keeps the limits in the application database.
//
// It is here rather than in the quota module because the four tiers are
// Karakuri's vocabulary: the module enforces a Policy it is handed and has no
// opinion about how many kinds of limit a host has. quota.Base is the seam
// between the two.
type SQLTierStore struct {
	db       *sql.DB
	postgres bool
}

func NewSQLTierStore(db *sql.DB) *SQLTierStore {
	return &SQLTierStore{db: db, postgres: isPostgres(db)}
}

// Migrate creates the table. Mirrored by migrations/000006_quota_tiers.up.sql
// for operators who apply schema by hand.
func (s *SQLTierStore) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS quota_tiers (
	name       TEXT PRIMARY KEY,
	cap_value  INTEGER NOT NULL,
	window_ms  INTEGER NOT NULL DEFAULT 0,
	rate       DOUBLE PRECISION NOT NULL DEFAULT 0,
	reason     TEXT NOT NULL DEFAULT '',
	updated_by TEXT NOT NULL DEFAULT '',
	updated_ms INTEGER NOT NULL DEFAULT 0
)`)
	if err != nil {
		return fmt.Errorf("quota tier schema: %w", err)
	}
	return nil
}

func (s *SQLTierStore) Tiers(ctx context.Context) ([]Tier, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT name, cap_value, window_ms, rate, reason, updated_by, updated_ms FROM quota_tiers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Tier
	for rows.Next() {
		var (
			t         Tier
			windowMS  int64
			updatedMS int64
		)
		if err := rows.Scan(&t.Name, &t.Cap, &windowMS, &t.Rate, &t.Reason, &t.UpdatedBy, &updatedMS); err != nil {
			return nil, err
		}
		t.Window = time.Duration(windowMS) * time.Millisecond
		if updatedMS > 0 {
			t.UpdatedAt = time.UnixMilli(updatedMS).UTC()
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *SQLTierStore) PutTier(ctx context.Context, t Tier) error {
	if err := t.Validate(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, s.rebind(`
INSERT INTO quota_tiers (name, cap_value, window_ms, rate, reason, updated_by, updated_ms)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (name) DO UPDATE SET
	cap_value = excluded.cap_value,
	window_ms = excluded.window_ms,
	rate = excluded.rate,
	reason = excluded.reason,
	updated_by = excluded.updated_by,
	updated_ms = excluded.updated_ms`),
		t.Name, t.Cap, t.Window.Milliseconds(), t.Rate, t.Reason, t.UpdatedBy, t.UpdatedAt.UTC().UnixMilli())
	return err
}

func (s *SQLTierStore) DeleteTier(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, s.rebind(`DELETE FROM quota_tiers WHERE name = ?`), name)
	return err
}

// rebind turns ? into $n for Postgres, which is the only dialect here whose
// placeholders differ.
func (s *SQLTierStore) rebind(q string) string {
	if !s.postgres {
		return q
	}
	var b strings.Builder
	n := 0
	for _, r := range q {
		if r == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
