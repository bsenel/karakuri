package quota_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bsenel/karakuri/quota"
)

func newRequests(t *testing.T) (quota.Requests, *quota.MemoryOverrideStore) {
	t.Helper()
	overrides := quota.NewMemoryOverrideStore()
	return quota.Requests{
		Store:     quota.NewMemoryRequestStore(),
		Overrides: overrides,
		Resolver:  quota.NewResolver(overrides),
	}, overrides
}

func pending(id string) quota.Request {
	return quota.Request{
		ID: id, Subject: "twin|t1", Name: "llm-tokens", Cap: 5_000_000,
		Reason: "launch week", RequestedBy: "alice",
		CreatedAt: time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC),
	}
}

// The whole point of the phase: approving changes what the limit is.
func TestApprovalWritesTheOverride(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q, overrides := newRequests(t)
	now := time.Date(2026, 8, 12, 13, 0, 0, 0, time.UTC)

	submitted, err := q.Submit(ctx, pending("r1"))
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if submitted.Status != quota.Pending {
		t.Fatalf("status = %q, want pending", submitted.Status)
	}
	// Nothing is in force yet.
	if got, _ := overrides.Overrides(ctx, "twin|t1"); len(got) != 0 {
		t.Fatalf("a submitted request already changed a limit: %+v", got)
	}

	decided, err := q.Decide(ctx, "r1", quota.Decisions{By: "bob", At: now, Approved: true, Note: "one week"})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if decided.Status != quota.Approved || decided.DecidedBy != "bob" || !decided.DecidedAt.Equal(now) {
		t.Fatalf("decided = %+v", decided)
	}

	got, err := overrides.Overrides(ctx, "twin|t1")
	if err != nil {
		t.Fatalf("Overrides: %v", err)
	}
	if len(got) != 1 || got[0].Cap != 5_000_000 || got[0].Name != "llm-tokens" {
		t.Fatalf("overrides = %+v, want the approved cap in force", got)
	}
	// The reason travels with it, so reading the overrides answers "why is this
	// one different" without a second query.
	if got[0].Reason != "launch week" {
		t.Errorf("Reason = %q, want the requester's", got[0].Reason)
	}

	// And the resolver sees it at once rather than a TTL later.
	base := quota.Quota{Name: "llm-tokens", Cap: 1_000_000, Period: quota.Daily}
	if resolved := base.Resolved(ctx, q.Resolver, "twin|t1", now); resolved.Cap != 5_000_000 {
		t.Fatalf("resolved cap = %d, want the approval visible immediately", resolved.Cap)
	}
}

func TestRejectionChangesNothing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q, overrides := newRequests(t)
	now := time.Date(2026, 8, 12, 13, 0, 0, 0, time.UTC)

	if _, err := q.Submit(ctx, pending("r1")); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	decided, err := q.Decide(ctx, "r1", quota.Decisions{By: "bob", At: now, Note: "use the shared pool"})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if decided.Status != quota.Rejected {
		t.Fatalf("status = %q, want rejected", decided.Status)
	}
	// A rejection is worth recording *why* — "no" on its own is the least
	// useful answer an operator can give.
	if decided.DecisionNote != "use the shared pool" {
		t.Errorf("note = %q, want the approver's words", decided.DecisionNote)
	}
	if got, _ := overrides.Overrides(ctx, "twin|t1"); len(got) != 0 {
		t.Fatalf("a rejection wrote an override: %+v", got)
	}
}

// Deciding twice is refused rather than ignored: the second decision is usually
// a duplicate submission, and silently keeping the first leaves that approver
// believing they did something.
func TestDecideIsOnceOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q, _ := newRequests(t)
	now := time.Now()

	if _, err := q.Submit(ctx, pending("r1")); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if _, err := q.Decide(ctx, "r1", quota.Decisions{By: "bob", At: now, Approved: true}); err != nil {
		t.Fatalf("first Decide: %v", err)
	}
	_, err := q.Decide(ctx, "r1", quota.Decisions{By: "carol", At: now})
	if !errors.Is(err, quota.ErrRequestDecided) {
		t.Fatalf("second Decide = %v, want ErrRequestDecided", err)
	}
}

func TestDecideRejects(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q, _ := newRequests(t)

	if _, err := q.Decide(ctx, "nope", quota.Decisions{By: "bob"}); !errors.Is(err, quota.ErrRequestNotFound) {
		t.Fatalf("unknown id = %v, want ErrRequestNotFound", err)
	}
	if _, err := q.Submit(ctx, pending("r1")); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	// An anonymous decision is refused: who approved a limit raise is the first
	// thing anybody reviewing it needs.
	if _, err := q.Decide(ctx, "r1", quota.Decisions{}); !errors.Is(err, quota.ErrInvalidPolicy) {
		t.Fatalf("decision with no approver = %v, want ErrInvalidPolicy", err)
	}
}

func TestSubmitValidates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q, _ := newRequests(t)

	cases := map[string]quota.Request{
		"no id":        {Subject: "twin|t1", Name: "llm-tokens", Cap: 10, Reason: "why", RequestedBy: "alice"},
		"no requester": {ID: "r", Subject: "twin|t1", Name: "llm-tokens", Cap: 10, Reason: "why"},
		"no reason":    {ID: "r", Subject: "twin|t1", Name: "llm-tokens", Cap: 10, RequestedBy: "alice"},
		"blank reason": {ID: "r", Subject: "twin|t1", Name: "llm-tokens", Cap: 10, Reason: "  ", RequestedBy: "alice"},
		"no subject":   {ID: "r", Name: "llm-tokens", Cap: 10, Reason: "why", RequestedBy: "alice"},
		"no cap":       {ID: "r", Subject: "twin|t1", Name: "llm-tokens", Reason: "why", RequestedBy: "alice"},
	}
	for name, r := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := q.Submit(ctx, r); !errors.Is(err, quota.ErrInvalidPolicy) {
				t.Fatalf("err = %v, want ErrInvalidPolicy", err)
			}
		})
	}
}

// Submitting cannot smuggle in a decision: whatever a caller sets, a new
// request is pending and undecided.
func TestSubmitIsAlwaysPending(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q, overrides := newRequests(t)

	r := pending("r1")
	r.Status = quota.Approved
	r.DecidedBy = "alice-pretending-to-be-bob"
	r.DecidedAt = time.Now()
	r.DecisionNote = "I approve of myself"

	got, err := q.Submit(ctx, r)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if got.Status != quota.Pending || got.DecidedBy != "" || !got.DecidedAt.IsZero() || got.DecisionNote != "" {
		t.Fatalf("a self-approved submission survived: %+v", got)
	}
	if in, _ := overrides.Overrides(ctx, "twin|t1"); len(in) != 0 {
		t.Fatal("submitting an approved-looking request changed a limit")
	}
}

func TestListRequests(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q, _ := newRequests(t)
	base := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

	first := pending("r1")
	second := pending("r2")
	second.CreatedAt = base.Add(time.Hour)
	second.RequestedBy = "carol"
	second.Subject = "twin|t2"
	for _, r := range []quota.Request{first, second} {
		if _, err := q.Submit(ctx, r); err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}
	if _, err := q.Decide(ctx, "r1", quota.Decisions{By: "bob", At: base.Add(2 * time.Hour), Approved: true}); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	all, err := q.List(ctx, quota.RequestFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 || all[0].ID != "r2" {
		t.Fatalf("List = %+v, want both, newest first", all)
	}

	for name, tc := range map[string]struct {
		filter quota.RequestFilter
		want   []string
	}{
		"pending":    {quota.RequestFilter{Status: quota.Pending}, []string{"r2"}},
		"approved":   {quota.RequestFilter{Status: quota.Approved}, []string{"r1"}},
		"by subject": {quota.RequestFilter{Subject: "twin|t1"}, []string{"r1"}},
		"mine":       {quota.RequestFilter{RequestedBy: "carol"}, []string{"r2"}},
		"none match": {quota.RequestFilter{RequestedBy: "nobody"}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := q.List(ctx, tc.filter)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("List = %+v, want %v", got, tc.want)
			}
			for i, id := range tc.want {
				if got[i].ID != id {
					t.Fatalf("List[%d] = %q, want %q", i, got[i].ID, id)
				}
			}
		})
	}
}

// Two requests submitted in the same instant still list in a stable order, so a
// paged listing cannot show one twice and skip another.
func TestListRequestsIsStable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	q, _ := newRequests(t)

	for _, id := range []string{"r3", "r1", "r2"} {
		if _, err := q.Submit(ctx, pending(id)); err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}
	for range 5 {
		got, err := q.List(ctx, quota.RequestFilter{})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 3 || got[0].ID != "r1" || got[2].ID != "r3" {
			t.Fatalf("List = %+v, want a stable order", got)
		}
	}
}

// A workflow missing a store must say so rather than silently doing nothing.
func TestRequestsWithoutStores(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var empty quota.Requests
	if _, err := empty.Submit(ctx, pending("r1")); err == nil {
		t.Error("Submit with no store succeeded")
	}
	if _, err := empty.Decide(ctx, "r1", quota.Decisions{By: "bob"}); err == nil {
		t.Error("Decide with no store succeeded")
	}
	if _, err := empty.List(ctx, quota.RequestFilter{}); err == nil {
		t.Error("List with no store succeeded")
	}

	// A request store with no override store: approving would change nothing,
	// so it is refused rather than recorded as an approval that did not apply.
	noOverrides := quota.Requests{Store: quota.NewMemoryRequestStore()}
	if _, err := noOverrides.Submit(ctx, pending("r1")); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if _, err := noOverrides.Decide(ctx, "r1", quota.Decisions{By: "bob", Approved: true}); err == nil {
		t.Error("approving with no override store succeeded, so the approval changed nothing")
	}
	// The request is still pending, so somebody can approve it once the
	// deployment is fixed.
	got, err := noOverrides.Store.GetRequest(ctx, "r1")
	if err != nil || got.Status != quota.Pending {
		t.Fatalf("request = %+v, %v — want it still pending", got, err)
	}
	// Rejecting needs no override store.
	if _, err := noOverrides.Decide(ctx, "r1", quota.Decisions{By: "bob"}); err != nil {
		t.Errorf("rejecting with no override store: %v", err)
	}
}

func TestRequestOverride(t *testing.T) {
	t.Parallel()
	expires := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	r := quota.Request{
		Subject: "principal|alice", Name: "request", Cap: 600,
		Window: time.Minute, ExpiresAt: expires, Reason: "launch",
	}
	got := r.Override()
	want := quota.Override{
		Subject: "principal|alice", Name: "request", Cap: 600,
		Window: time.Minute, ExpiresAt: expires, Reason: "launch",
	}
	if got != want {
		t.Fatalf("Override = %+v, want %+v", got, want)
	}
}
