package sql_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bsenel/karakuri/quota"
	quotasql "github.com/bsenel/karakuri/quota/sql"
)

// The stores are exercised against a real database rather than asserted about,
// for the same reason the counters are: what the SQL does is the thing under
// test, and a fake would only prove the fake agrees with itself.

func TestOverrideStoreRoundTrip(t *testing.T) {
	b, _ := open(t)
	ctx := context.Background()
	expires := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)

	o := quota.Override{
		Subject: "twin|t1", Name: "llm-tokens", Cap: 5_000_000,
		Window: time.Minute, ExpiresAt: expires, Reason: "launch week",
	}
	if err := b.PutOverride(ctx, o); err != nil {
		t.Fatalf("PutOverride: %v", err)
	}

	got, err := b.Overrides(ctx, "twin|t1")
	if err != nil {
		t.Fatalf("Overrides: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("overrides = %+v, want one", got)
	}
	// Every field survives the round trip, expiry included — an override that
	// came back permanent because a column was dropped is the failure mode
	// nobody notices until the raise never ends.
	if got[0] != o {
		t.Fatalf("round trip = %+v, want %+v", got[0], o)
	}

	// Same subject and name replaces rather than accumulating.
	o.Cap = 9_000_000
	if err := b.PutOverride(ctx, o); err != nil {
		t.Fatalf("PutOverride: %v", err)
	}
	got, _ = b.Overrides(ctx, "twin|t1")
	if len(got) != 1 || got[0].Cap != 9_000_000 {
		t.Fatalf("after replace = %+v, want one row at the new cap", got)
	}

	if err := b.DeleteOverride(ctx, "twin|t1", "llm-tokens"); err != nil {
		t.Fatalf("DeleteOverride: %v", err)
	}
	if got, _ := b.Overrides(ctx, "twin|t1"); len(got) != 0 {
		t.Fatalf("after delete = %+v, want none", got)
	}
	// Deleting what is not there is not an error, so a retried revocation is safe.
	if err := b.DeleteOverride(ctx, "twin|t1", "llm-tokens"); err != nil {
		t.Fatalf("second DeleteOverride: %v", err)
	}
}

// A permanent override stores a zero expiry and comes back permanent, rather
// than as a date in the year 1.
func TestOverrideStorePermanent(t *testing.T) {
	b, _ := open(t)
	ctx := context.Background()

	if err := b.PutOverride(ctx, quota.Override{Subject: "twin|t1", Name: "llm-tokens", Cap: 10}); err != nil {
		t.Fatalf("PutOverride: %v", err)
	}
	got, _ := b.Overrides(ctx, "twin|t1")
	if len(got) != 1 || !got[0].ExpiresAt.IsZero() {
		t.Fatalf("ExpiresAt = %v, want the zero time", got[0].ExpiresAt)
	}
	if !got[0].Active(time.Now().Add(100 * 365 * 24 * time.Hour)) {
		t.Error("a permanent override expired")
	}
}

func TestOverrideStoreListsAndValidates(t *testing.T) {
	b, _ := open(t)
	ctx := context.Background()

	for _, o := range []quota.Override{
		{Subject: "twin|t2", Name: "llm-tokens", Cap: 10},
		{Subject: "twin|t1", Name: "llm-tokens", Cap: 20},
		{Subject: "twin|t1", Name: "capability", Cap: 30},
	} {
		if err := b.PutOverride(ctx, o); err != nil {
			t.Fatalf("PutOverride: %v", err)
		}
	}

	all, err := b.ListOverrides(ctx)
	if err != nil {
		t.Fatalf("ListOverrides: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("ListOverrides = %+v, want three", all)
	}
	if all[0].Subject != "twin|t1" || all[0].Name != "capability" || all[2].Subject != "twin|t2" {
		t.Fatalf("ListOverrides is not sorted: %+v", all)
	}

	// An unusable override is refused here rather than stored and ignored later.
	if err := b.PutOverride(ctx, quota.Override{Name: "broken"}); !errors.Is(err, quota.ErrInvalidPolicy) {
		t.Fatalf("PutOverride of an invalid override = %v, want ErrInvalidPolicy", err)
	}
	// And an unknown subject is empty rather than an error.
	if got, err := b.Overrides(ctx, "twin|nobody"); err != nil || len(got) != 0 {
		t.Fatalf("Overrides for an unknown subject = %+v, %v", got, err)
	}
}

func TestRequestStoreRoundTrip(t *testing.T) {
	b, _ := open(t)
	ctx := context.Background()
	created := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

	r := quota.Request{
		ID: "r1", Subject: "twin|t1", Name: "llm-tokens", Cap: 5_000_000,
		Window: time.Minute, ExpiresAt: created.AddDate(0, 0, 7),
		Reason: "launch week", Status: quota.Pending,
		RequestedBy: "alice", CreatedAt: created,
	}
	if err := b.PutRequest(ctx, r); err != nil {
		t.Fatalf("PutRequest: %v", err)
	}

	got, err := b.GetRequest(ctx, "r1")
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if got != r {
		t.Fatalf("round trip = %+v, want %+v", got, r)
	}
	// A pending request has no decision, and the zero time must survive as one.
	if !got.DecidedAt.IsZero() || got.DecidedBy != "" {
		t.Errorf("a pending request came back decided: %+v", got)
	}

	r.Status, r.DecidedBy, r.DecidedAt = quota.Approved, "bob", created.Add(time.Hour)
	r.DecisionNote = "one week only"
	if err := b.PutRequest(ctx, r); err != nil {
		t.Fatalf("PutRequest: %v", err)
	}
	if got, _ := b.GetRequest(ctx, "r1"); got != r {
		t.Fatalf("after decision = %+v, want %+v", got, r)
	}

	if _, err := b.GetRequest(ctx, "nope"); !errors.Is(err, quota.ErrRequestNotFound) {
		t.Fatalf("GetRequest of an unknown id = %v, want ErrRequestNotFound", err)
	}
}

func TestRequestStoreListFilters(t *testing.T) {
	b, _ := open(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

	requests := []quota.Request{
		{ID: "r1", Subject: "twin|t1", Name: "llm-tokens", Cap: 10, Reason: "a",
			Status: quota.Approved, RequestedBy: "alice", CreatedAt: base},
		{ID: "r2", Subject: "twin|t2", Name: "llm-tokens", Cap: 20, Reason: "b",
			Status: quota.Pending, RequestedBy: "carol", CreatedAt: base.Add(time.Hour)},
		{ID: "r3", Subject: "twin|t1", Name: "capability", Cap: 30, Reason: "c",
			Status: quota.Pending, RequestedBy: "alice", CreatedAt: base.Add(2 * time.Hour)},
	}
	for _, r := range requests {
		if err := b.PutRequest(ctx, r); err != nil {
			t.Fatalf("PutRequest: %v", err)
		}
	}

	for name, tc := range map[string]struct {
		filter quota.RequestFilter
		want   []string
	}{
		"everything, newest first": {quota.RequestFilter{}, []string{"r3", "r2", "r1"}},
		"pending":                  {quota.RequestFilter{Status: quota.Pending}, []string{"r3", "r2"}},
		"by subject":               {quota.RequestFilter{Subject: "twin|t1"}, []string{"r3", "r1"}},
		"mine":                     {quota.RequestFilter{RequestedBy: "alice"}, []string{"r3", "r1"}},
		"mine and pending":         {quota.RequestFilter{RequestedBy: "alice", Status: quota.Pending}, []string{"r3"}},
		"nothing matches":          {quota.RequestFilter{RequestedBy: "nobody"}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := b.ListRequests(ctx, tc.filter)
			if err != nil {
				t.Fatalf("ListRequests: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("ListRequests = %+v, want %v", got, tc.want)
			}
			for i, id := range tc.want {
				if got[i].ID != id {
					t.Fatalf("ListRequests[%d] = %q, want %q", i, got[i].ID, id)
				}
			}
		})
	}
}

// The workflow over the persistent stores behaves as it does over the in-memory
// ones — which is the only reason the two implementations are interchangeable.
func TestWorkflowOverSQL(t *testing.T) {
	b, _ := open(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 12, 13, 0, 0, 0, time.UTC)

	q := quota.Requests{Store: b, Overrides: b, Resolver: quota.NewResolver(b)}
	if _, err := q.Submit(ctx, quota.Request{
		ID: "r1", Subject: "twin|t1", Name: "llm-tokens", Cap: 5_000_000,
		Reason: "launch week", RequestedBy: "alice", CreatedAt: now,
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if _, err := q.Decide(ctx, "r1", quota.Decisions{By: "bob", At: now, Approved: true}); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	base := quota.Quota{Name: "llm-tokens", Cap: 1_000_000, Period: quota.Daily}
	if got := base.Resolved(ctx, q.Resolver, "twin|t1", now); got.Cap != 5_000_000 {
		t.Fatalf("resolved cap = %d, want the approval in force", got.Cap)
	}
	// And the raised ceiling is what the backend then enforces.
	dec, err := base.Resolved(ctx, q.Resolver, "twin|t1", now).Take(ctx, b, "twin|t1", 2_000_000, now)
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	if !dec.Allowed {
		t.Fatal("a take inside the raised ceiling was refused")
	}
	if dec.Limit != 5_000_000 {
		t.Fatalf("Limit = %d, want the override's", dec.Limit)
	}
}

// Overrides and requests survive a restart, which is the whole reason they are
// not in the memory stores.
func TestStoresSurviveReopen(t *testing.T) {
	b, db := open(t)
	ctx := context.Background()

	if err := b.PutOverride(ctx, quota.Override{Subject: "twin|t1", Name: "llm-tokens", Cap: 42}); err != nil {
		t.Fatalf("PutOverride: %v", err)
	}
	if err := b.PutRequest(ctx, quota.Request{
		ID: "r1", Subject: "twin|t1", Name: "llm-tokens", Cap: 42,
		Reason: "why", Status: quota.Approved, RequestedBy: "alice",
	}); err != nil {
		t.Fatalf("PutRequest: %v", err)
	}

	// A second Backend over the same database is what a restart looks like from
	// here: no shared memory, only what reached the file.
	reopened, err := quotasql.New(db, quotasql.Options{Dialect: quotasql.SQLite})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got, _ := reopened.Overrides(ctx, "twin|t1"); len(got) != 1 || got[0].Cap != 42 {
		t.Fatalf("overrides after reopen = %+v", got)
	}
	if got, err := reopened.GetRequest(ctx, "r1"); err != nil || got.Cap != 42 {
		t.Fatalf("request after reopen = %+v, %v", got, err)
	}
}
