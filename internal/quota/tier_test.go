package quota_test

import (
	"context"
	stdsql "database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	extauth "github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/config"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	"github.com/bsenel/karakuri/quota"

	_ "modernc.org/sqlite"
)

// principalRequest is an authenticated request, which is what the limiter keys
// on — an unauthenticated one has no key and is exempt.
func principalRequest(id string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/twins", nil)
	return r.WithContext(extauth.WithPrincipal(r.Context(), extauth.Principal{ID: id}))
}

func tierDB(t *testing.T) *stdsql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tiers.db")
	db, err := stdsql.Open("sqlite",
		"file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func configured() karakuriquota.Tiers {
	return karakuriquota.DefaultTiers(config.QuotaConfig{
		RequestsPerMinute: 60, RequestBurst: 20,
		CapabilityPerDay: 1000, LLMTokensPerDay: 1_000_000, AdapterPerDay: 5000,
	})
}

// deps builds the wiring the API gets, with caching off so a test reads what it
// just wrote rather than what was cached a moment ago.
func tierDeps(t *testing.T, store karakuriquota.TierStore) karakuriquota.Deps {
	t.Helper()
	return karakuriquota.Deps{
		Backend:   quota.NewMemoryBackend(),
		Resolver:  quota.NewResolver(nil),
		TierStore: store,
		TierSet:   karakuriquota.NewTierResolver(configured(), store).WithTTL(-1),
		Costs:     &karakuriquota.Recorder{},
	}
}

// The whole point of the phase: a stored tier is what gets enforced, and
// configuration is what it fell back from.
func TestStoredTierTakesPrecedenceOverConfiguration(t *testing.T) {
	ctx := context.Background()
	d := tierDeps(t, karakuriquota.NewMemoryTierStore())

	if got := d.Tiers(ctx).LLMTokens.Cap; got != 1_000_000 {
		t.Fatalf("llm-tokens = %d before any edit, want the configured 1000000", got)
	}
	err := d.SetTier(ctx, karakuriquota.Tier{
		Name: karakuriquota.TierLLMTokens, Cap: 5_000_000,
		Reason: "the team grew", UpdatedBy: "ann",
	})
	if err != nil {
		t.Fatalf("SetTier: %v", err)
	}
	if got := d.Tiers(ctx).LLMTokens.Cap; got != 5_000_000 {
		t.Errorf("llm-tokens = %d after the edit, want the stored 5000000", got)
	}
	// Configuration is still readable, which is what lets the UI and the log
	// line show both. Losing it would make "reset" undefined.
	if got := d.TierSet.Configured().LLMTokens.Cap; got != 1_000_000 {
		t.Errorf("configured = %d, want the file's 1000000 unchanged", got)
	}

	if err := d.ResetTier(ctx, karakuriquota.TierLLMTokens); err != nil {
		t.Fatalf("ResetTier: %v", err)
	}
	if got := d.Tiers(ctx).LLMTokens.Cap; got != 1_000_000 {
		t.Errorf("llm-tokens = %d after reset, want the configured 1000000 back", got)
	}
}

// The request tier is a rate rather than a cap, so it stores a window and a
// refill and keeps its algorithm — raising a ceiling is not choosing to count
// differently.
func TestStoredRequestTierKeepsItsAlgorithm(t *testing.T) {
	ctx := context.Background()
	d := tierDeps(t, karakuriquota.NewMemoryTierStore())

	err := d.SetTier(ctx, karakuriquota.Tier{
		Name: karakuriquota.TierRequest, Cap: 40, Window: time.Minute, Rate: 2,
		Reason: "the SPA polls", UpdatedBy: "ann",
	})
	if err != nil {
		t.Fatalf("SetTier: %v", err)
	}
	got := d.Tiers(ctx).Request
	if got.Limit != 40 || got.Rate != 2 {
		t.Errorf("request = %+v, want the stored burst of 40 at 2/s", got)
	}
	if got.Algorithm != configured().Request.Algorithm {
		t.Errorf("algorithm = %q, want the configured %q", got.Algorithm, configured().Request.Algorithm)
	}
}

// A tier nobody enforces, a cap nobody could satisfy, and a change nobody wrote
// a reason for are all refused at write time. Accepting any of them stores a row
// that is silently ignored, which looks exactly like the write having been lost.
func TestSetTierRefusesWhatCannotBeEnforced(t *testing.T) {
	ctx := context.Background()
	d := tierDeps(t, karakuriquota.NewMemoryTierStore())

	for _, tc := range []struct {
		name string
		tier karakuriquota.Tier
	}{
		{"unknown tier", karakuriquota.Tier{Name: "storage", Cap: 10, Reason: "why not"}},
		{"no cap", karakuriquota.Tier{Name: karakuriquota.TierAdapter, Reason: "why not"}},
		{"no reason", karakuriquota.Tier{Name: karakuriquota.TierAdapter, Cap: 10}},
		{"window on a daily quota", karakuriquota.Tier{
			Name: karakuriquota.TierAdapter, Cap: 10, Window: time.Minute, Reason: "why not"}},
		{"rate tier with no window", karakuriquota.Tier{
			Name: karakuriquota.TierRequest, Cap: 10, Reason: "why not"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := d.SetTier(ctx, tc.tier); err == nil {
				t.Fatal("the write was accepted")
			}
			// And nothing was stored, so a refused write leaves the previous
			// limit standing rather than a half-applied one.
			if len(d.TierSet.Stored(ctx)) != 0 {
				t.Error("a refused write left a row behind")
			}
		})
	}
	if err := d.ResetTier(ctx, "storage"); err == nil {
		t.Error("resetting a tier nobody enforces was accepted")
	}
}

// A deployment with no database is told so, rather than accepting an edit that
// would vanish on restart.
func TestEditingWithoutAStoreIsRefused(t *testing.T) {
	ctx := context.Background()
	d := tierDeps(t, nil)

	err := d.SetTier(ctx, karakuriquota.Tier{
		Name: karakuriquota.TierLLMTokens, Cap: 5, Reason: "why not"})
	if err == nil {
		t.Fatal("an edit was accepted with nowhere to keep it")
	}
	if err := d.ResetTier(ctx, karakuriquota.TierLLMTokens); err == nil {
		t.Error("a reset was accepted with nowhere to keep it")
	}
	// And the limits still resolve, so the absence of a store is not an outage.
	if got := d.Tiers(ctx).LLMTokens.Cap; got != 1_000_000 {
		t.Errorf("llm-tokens = %d, want configuration to still apply", got)
	}
}

// The cache is the design — see TierResolver — so what matters is that a write
// through this process is visible at once rather than a TTL later.
func TestWritingInvalidatesTheCache(t *testing.T) {
	ctx := context.Background()
	store := karakuriquota.NewMemoryTierStore()
	d := karakuriquota.Deps{
		Backend:   quota.NewMemoryBackend(),
		Resolver:  quota.NewResolver(nil),
		TierStore: store,
		// A full-length TTL: without the Invalidate this read would be stale
		// for thirty seconds.
		TierSet: karakuriquota.NewTierResolver(configured(), store),
		Costs:   &karakuriquota.Recorder{},
	}

	// Prime the cache.
	if got := d.Tiers(ctx).Adapter.Cap; got != 5000 {
		t.Fatalf("adapter = %d, want the configured 5000", got)
	}
	err := d.SetTier(ctx, karakuriquota.Tier{
		Name: karakuriquota.TierAdapter, Cap: 9000, Reason: "a busy integration", UpdatedBy: "ann"})
	if err != nil {
		t.Fatalf("SetTier: %v", err)
	}
	if got := d.Tiers(ctx).Adapter.Cap; got != 9000 {
		t.Errorf("adapter = %d immediately after the write, want 9000", got)
	}

	if err := d.ResetTier(ctx, karakuriquota.TierAdapter); err != nil {
		t.Fatalf("ResetTier: %v", err)
	}
	if got := d.Tiers(ctx).Adapter.Cap; got != 5000 {
		t.Errorf("adapter = %d immediately after the reset, want 5000", got)
	}
}

// A store that cannot be read falls back to configuration rather than to
// nothing, and never upward — the same trade the override resolver makes.
func TestTiersFallBackWhenTheStoreCannotBeRead(t *testing.T) {
	ctx := context.Background()
	d := tierDeps(t, brokenTierStore{})

	got := d.Tiers(ctx)
	if got.LLMTokens.Cap != 1_000_000 || got.Request.Limit != 20 {
		t.Fatalf("tiers = %+v, want configuration while the store is down", got)
	}
}

type brokenTierStore struct{}

func (brokenTierStore) Tiers(context.Context) ([]karakuriquota.Tier, error) {
	return nil, errors.New("database down")
}
func (brokenTierStore) PutTier(context.Context, karakuriquota.Tier) error { return nil }
func (brokenTierStore) DeleteTier(context.Context, string) error          { return nil }

// The stored limit has to reach the middleware, not just the reports. Before
// quota.Base this was impossible: Limit captured its policy at wire-up.
func TestTheLimiterEnforcesTheStoredTier(t *testing.T) {
	ctx := context.Background()
	d := tierDeps(t, karakuriquota.NewMemoryTierStore())
	err := d.SetTier(ctx, karakuriquota.Tier{
		Name: karakuriquota.TierRequest, Cap: 2, Window: time.Minute, Rate: 2,
		Reason: "tightened while we investigate", UpdatedBy: "ann",
	})
	if err != nil {
		t.Fatalf("SetTier: %v", err)
	}

	handler := d.Limiter()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	allowed := 0
	var last int
	for range 5 {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, principalRequest("ann"))
		last = rec.Code
		if rec.Code == http.StatusOK {
			allowed++
		}
	}
	if allowed != 2 {
		t.Errorf("allowed %d of 5 against a stored ceiling of 2", allowed)
	}
	if last != http.StatusTooManyRequests {
		t.Errorf("last status = %d, want 429", last)
	}
}

// SQLite is what a single-node deployment actually runs, and the round trip is
// where a column type or a placeholder gets it wrong.
func TestSQLTierStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := karakuriquota.NewSQLTierStore(tierDB(t))
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// Migration runs at every boot, so it has to be idempotent.
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}

	want := karakuriquota.Tier{
		Name: karakuriquota.TierRequest, Cap: 40, Window: 90 * time.Second, Rate: 1.5,
		Reason: "the SPA polls", UpdatedBy: "ann",
		UpdatedAt: time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC),
	}
	if err := store.PutTier(ctx, want); err != nil {
		t.Fatalf("PutTier: %v", err)
	}
	got, err := store.Tiers(ctx)
	if err != nil {
		t.Fatalf("Tiers: %v", err)
	}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}

	// Writing the same tier twice replaces it rather than failing on the key,
	// because an operator adjusting a number types the same command again.
	want.Cap, want.Reason = 60, "adjusted"
	if err := store.PutTier(ctx, want); err != nil {
		t.Fatalf("second PutTier: %v", err)
	}
	if got, _ := store.Tiers(ctx); len(got) != 1 || got[0].Cap != 60 {
		t.Fatalf("after replace = %+v, want one row at 60", got)
	}

	if err := store.DeleteTier(ctx, karakuriquota.TierRequest); err != nil {
		t.Fatalf("DeleteTier: %v", err)
	}
	if got, _ := store.Tiers(ctx); len(got) != 0 {
		t.Fatalf("after delete = %+v, want nothing", got)
	}
	// Deleting a tier nobody stored is not an error: it is the state the caller
	// asked for.
	if err := store.DeleteTier(ctx, karakuriquota.TierAdapter); err != nil {
		t.Errorf("deleting an absent tier: %v", err)
	}
}
