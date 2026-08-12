package quota_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bsenel/karakuri/quota"
)

// countingStore records reads so a test can prove the cache is doing its job —
// the whole reason the resolver exists rather than a store read per request.
type countingStore struct {
	quota.OverrideStore
	mu    sync.Mutex
	reads int
	err   error
}

func (s *countingStore) Overrides(ctx context.Context, subject quota.Key) ([]quota.Override, error) {
	s.mu.Lock()
	s.reads++
	err := s.err
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return s.OverrideStore.Overrides(ctx, subject)
}

func (s *countingStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reads
}

func newCounting(t *testing.T, overrides ...quota.Override) *countingStore {
	t.Helper()
	inner := quota.NewMemoryOverrideStore()
	for _, o := range overrides {
		if err := inner.PutOverride(context.Background(), o); err != nil {
			t.Fatalf("seed override: %v", err)
		}
	}
	return &countingStore{OverrideStore: inner}
}

func TestResolverAppliesAnOverride(t *testing.T) {
	t.Parallel()
	ctx, now := context.Background(), time.Now()

	store := newCounting(t, quota.Override{Subject: "principal|alice", Name: "request", Cap: 600})
	r := quota.NewResolver(store)

	base := quota.PerMinute(60)
	if got := r.Policy(ctx, "principal|alice", "request", base, now); got.Limit != 600 {
		t.Errorf("alice's limit = %d, want the override's 600", got.Limit)
	}
	// Everybody else keeps what the operator configured.
	if got := r.Policy(ctx, "principal|bob", "request", base, now); got.Limit != 60 {
		t.Errorf("bob's limit = %d, want the configured 60", got.Limit)
	}
	// An override for one limit does not touch another.
	if got := r.Policy(ctx, "principal|alice", "capability", base, now); got.Limit != 60 {
		t.Errorf("alice's capability limit = %d, want the base", got.Limit)
	}
}

func TestResolverAppliesAQuotaOverride(t *testing.T) {
	t.Parallel()
	ctx, now := context.Background(), time.Now()

	store := newCounting(t, quota.Override{Subject: "twin|t1", Name: "llm-tokens", Cap: 5_000_000})
	r := quota.NewResolver(store)
	base := quota.Quota{Name: "llm-tokens", Cap: 1_000_000, Period: quota.Daily}

	got := base.Resolved(ctx, r, "twin|t1", now)
	if got.Cap != 5_000_000 {
		t.Errorf("Cap = %d, want the override's", got.Cap)
	}
	if got.Period != quota.Daily || got.Name != "llm-tokens" {
		t.Errorf("resolved quota changed more than the cap: %+v", got)
	}
	if other := base.Resolved(ctx, r, "twin|t2", now); other.Cap != 1_000_000 {
		t.Errorf("another twin's cap = %d, want the base", other.Cap)
	}
}

// An expired override stops applying without anybody having to delete it, which
// is what makes "raise it for launch week" safe to approve.
func TestResolverIgnoresExpiredOverrides(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

	store := newCounting(t, quota.Override{
		Subject: "principal|alice", Name: "request", Cap: 600,
		ExpiresAt: now.Add(time.Hour),
	})
	// No cache, so each call re-reads and the clock is the only thing moving.
	r := quota.NewResolver(store, quota.CacheTTL(-1))
	base := quota.PerMinute(60)

	if got := r.Policy(ctx, "principal|alice", "request", base, now); got.Limit != 600 {
		t.Fatalf("before expiry = %d, want 600", got.Limit)
	}
	if got := r.Policy(ctx, "principal|alice", "request", base, now.Add(2*time.Hour)); got.Limit != 60 {
		t.Fatalf("after expiry = %d, want the base 60 back", got.Limit)
	}
}

func TestResolverCaches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

	store := newCounting(t, quota.Override{Subject: "principal|alice", Name: "request", Cap: 600})
	r := quota.NewResolver(store, quota.CacheTTL(30*time.Second))
	base := quota.PerMinute(60)

	for i := 0; i < 50; i++ {
		r.Policy(ctx, "principal|alice", "request", base, now)
	}
	if got := store.count(); got != 1 {
		t.Fatalf("50 resolutions cost %d store reads, want 1 — the cache is not working", got)
	}

	// Inside the TTL: still cached.
	r.Policy(ctx, "principal|alice", "request", base, now.Add(29*time.Second))
	if got := store.count(); got != 1 {
		t.Fatalf("a read inside the TTL cost %d reads, want 1", got)
	}
	// Past it: read through.
	r.Policy(ctx, "principal|alice", "request", base, now.Add(31*time.Second))
	if got := store.count(); got != 2 {
		t.Fatalf("a read past the TTL cost %d reads total, want 2", got)
	}
}

// The bound the acceptance criterion rests on: an approval is visible within the
// TTL, and immediately to whoever wrote it.
func TestResolverInvalidate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

	inner := quota.NewMemoryOverrideStore()
	store := &countingStore{OverrideStore: inner}
	r := quota.NewResolver(store, quota.CacheTTL(30*time.Second))
	base := quota.PerMinute(60)

	if got := r.Policy(ctx, "principal|alice", "request", base, now); got.Limit != 60 {
		t.Fatalf("limit = %d, want the base before anything is approved", got.Limit)
	}

	if err := inner.PutOverride(ctx, quota.Override{Subject: "principal|alice", Name: "request", Cap: 600}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	// Still cached — this is the staleness the TTL buys, stated rather than
	// hidden.
	if got := r.Policy(ctx, "principal|alice", "request", base, now); got.Limit != 60 {
		t.Fatalf("limit = %d, want the cached base until the TTL lapses", got.Limit)
	}

	r.Invalidate("principal|alice")
	if got := r.Policy(ctx, "principal|alice", "request", base, now); got.Limit != 600 {
		t.Fatalf("limit = %d, want the approval visible immediately after Invalidate", got.Limit)
	}

	// And without invalidation it lands once the TTL lapses.
	if err := inner.PutOverride(ctx, quota.Override{Subject: "principal|alice", Name: "request", Cap: 900}); err != nil {
		t.Fatalf("approve again: %v", err)
	}
	if got := r.Policy(ctx, "principal|alice", "request", base, now.Add(31*time.Second)); got.Limit != 900 {
		t.Fatalf("limit = %d, want the second approval after the TTL", got.Limit)
	}
	// Invalidating a subject nobody cached is harmless.
	r.Invalidate("principal|nobody")
}

// A store that cannot be read falls back to configuration. It must never raise
// a limit it could not verify, and must not cache the failure.
func TestResolverFailsToTheConfiguredLimit(t *testing.T) {
	t.Parallel()
	ctx, now := context.Background(), time.Now()

	store := newCounting(t, quota.Override{Subject: "principal|alice", Name: "request", Cap: 600})
	var reported []error
	r := quota.NewResolver(store,
		quota.CacheTTL(time.Hour),
		quota.OnResolveError(func(_ quota.Key, err error) { reported = append(reported, err) }),
	)
	base := quota.PerMinute(60)

	store.err = errors.New("store down")
	if got := r.Policy(ctx, "principal|alice", "request", base, now); got.Limit != 60 {
		t.Fatalf("limit = %d, want the configured base when the store cannot be read", got.Limit)
	}
	if len(reported) != 1 {
		t.Fatalf("reported %d errors, want the failure surfaced", len(reported))
	}

	// The failure was not cached: the next call reads through and finds the
	// override, rather than serving "no overrides" for an hour.
	store.err = nil
	if got := r.Policy(ctx, "principal|alice", "request", base, now); got.Limit != 600 {
		t.Fatalf("limit = %d, want the override once the store recovers", got.Limit)
	}
}

// A deployment with no overrides configured must not have to branch: a nil
// resolver and a nil store both resolve to the base.
func TestResolverZeroValues(t *testing.T) {
	t.Parallel()
	ctx, now := context.Background(), time.Now()
	base := quota.PerMinute(60)
	baseQuota := quota.Quota{Name: "llm-tokens", Cap: 1000, Period: quota.Daily}

	var nilResolver *quota.Resolver
	if got := nilResolver.Policy(ctx, "principal|alice", "request", base, now); got.Limit != 60 {
		t.Errorf("nil resolver = %d, want the base", got.Limit)
	}
	if got := nilResolver.Quota(ctx, "twin|t1", baseQuota, now); got.Cap != 1000 {
		t.Errorf("nil resolver quota = %d, want the base", got.Cap)
	}
	nilResolver.Invalidate("principal|alice")

	empty := quota.NewResolver(nil)
	if got := empty.Policy(ctx, "principal|alice", "request", base, now); got.Limit != 60 {
		t.Errorf("resolver over no store = %d, want the base", got.Limit)
	}

	// An empty subject or limit name has nothing to look up.
	r := quota.NewResolver(newCounting(t))
	if got := r.Policy(ctx, "", "request", base, now); got.Limit != 60 {
		t.Errorf("empty subject = %d, want the base", got.Limit)
	}
	if got := r.Policy(ctx, "principal|alice", "", base, now); got.Limit != 60 {
		t.Errorf("empty name = %d, want the base", got.Limit)
	}
}

func TestResolverDefaultTTL(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

	store := newCounting(t, quota.Override{Subject: "principal|alice", Name: "request", Cap: 600})
	r := quota.NewResolver(store) // no CacheTTL option
	base := quota.PerMinute(60)

	r.Policy(ctx, "principal|alice", "request", base, now)
	r.Policy(ctx, "principal|alice", "request", base, now.Add(quota.DefaultCacheTTL-time.Second))
	if got := store.count(); got != 1 {
		t.Fatalf("reads = %d, want the default TTL to be holding", got)
	}
	r.Policy(ctx, "principal|alice", "request", base, now.Add(quota.DefaultCacheTTL))
	if got := store.count(); got != 2 {
		t.Fatalf("reads = %d, want a read once the default TTL lapsed", got)
	}

	// An explicit zero means the default too, rather than "no caching" — that
	// is what a negative duration is for, so a caller cannot disable the cache
	// by leaving a config field unset.
	zero := newCounting(t, quota.Override{Subject: "principal|alice", Name: "request", Cap: 600})
	rz := quota.NewResolver(zero, quota.CacheTTL(0))
	rz.Policy(ctx, "principal|alice", "request", base, now)
	rz.Policy(ctx, "principal|alice", "request", base, now.Add(quota.DefaultCacheTTL-time.Second))
	if got := zero.count(); got != 1 {
		t.Fatalf("reads with CacheTTL(0) = %d, want the default TTL holding", got)
	}
}

func TestResolverIsSafeUnderConcurrency(t *testing.T) {
	t.Parallel()
	ctx, now := context.Background(), time.Now()

	store := newCounting(t, quota.Override{Subject: "principal|alice", Name: "request", Cap: 600})
	r := quota.NewResolver(store)
	base := quota.PerMinute(60)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if got := r.Policy(ctx, "principal|alice", "request", base, now); got.Limit != 600 {
				t.Errorf("limit = %d, want 600", got.Limit)
			}
			if i%10 == 0 {
				r.Invalidate("principal|alice")
			}
		}(i)
	}
	wg.Wait()
}
