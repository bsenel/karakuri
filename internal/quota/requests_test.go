package quota_test

import (
	"context"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/config"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	"github.com/bsenel/karakuri/quota"
)

func deps(t *testing.T) karakuriquota.Deps {
	t.Helper()
	overrides := quota.NewMemoryOverrideStore()
	return karakuriquota.Deps{
		Backend:       quota.NewMemoryBackend(),
		TierSet:       karakuriquota.NewTierResolver(karakuriquota.DefaultTiers(config.QuotaConfig{}), nil),
		OverrideStore: overrides,
		RequestStore:  quota.NewMemoryRequestStore(),
		Resolver:      quota.NewResolver(overrides),
		Costs:         &karakuriquota.Recorder{},
	}
}

// The subject a tier is counted under has to match how it is enforced, or an
// approval writes an override nothing ever reads.
func TestSubjectForMatchesEnforcement(t *testing.T) {
	// The request tier throttles whoever is calling, so it is per principal.
	got, err := karakuriquota.SubjectFor(karakuriquota.TierRequest, "alice", "")
	if err != nil {
		t.Fatalf("SubjectFor: %v", err)
	}
	if got != quota.JoinKey("principal", "alice") {
		t.Fatalf("request subject = %q, want the principal", got)
	}

	// The twin quotas bound a twin's work, so they are per twin — and the key
	// is the one TwinKey builds, which is what the enforcement path reads.
	for _, tier := range []string{
		karakuriquota.TierCapability, karakuriquota.TierLLMTokens, karakuriquota.TierAdapter,
	} {
		got, err := karakuriquota.SubjectFor(tier, "", "t1")
		if err != nil {
			t.Fatalf("SubjectFor(%s): %v", tier, err)
		}
		if got != karakuriquota.TwinKey("t1") {
			t.Fatalf("%s subject = %q, want the twin key", tier, got)
		}
	}

	// A tier with nothing to name it is refused rather than keyed on an empty
	// string, which would be one shared bucket for everybody.
	if _, err := karakuriquota.SubjectFor(karakuriquota.TierRequest, "", ""); err == nil {
		t.Error("a request override with no principal was accepted")
	}
	if _, err := karakuriquota.SubjectFor(karakuriquota.TierLLMTokens, "alice", ""); err == nil {
		t.Error("a twin override with no twin was accepted")
	}
	if _, err := karakuriquota.SubjectFor("wishes", "alice", "t1"); err == nil {
		t.Error("an unknown tier was accepted")
	}
}

// The end of the phase's acceptance criterion, at this layer: submit, approve,
// and the resolved tier is the new one.
func TestSubmitApproveRaisesTheLimit(t *testing.T) {
	ctx := context.Background()
	d := deps(t)

	req, err := d.Submit(ctx, karakuriquota.SubmitRequest{
		Tier: karakuriquota.TierLLMTokens, TwinID: "t1",
		Cap: 5_000_000, Reason: "launch week", RequestedBy: "alice",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !strings.HasPrefix(req.ID, "qr_") || req.Status != quota.Pending {
		t.Fatalf("submitted = %+v", req)
	}
	// Before approval the configured cap applies.
	if got := d.Tiers(ctx).LLMTokens.Resolved(ctx, d.Resolver, karakuriquota.TwinKey("t1"), req.CreatedAt); got.Cap != 1_000_000 {
		t.Fatalf("cap before approval = %d, want the configured default", got.Cap)
	}

	if _, err := d.Decide(ctx, req.ID, "bob", "one week", true); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if got := d.Tiers(ctx).LLMTokens.Resolved(ctx, d.Resolver, karakuriquota.TwinKey("t1"), req.CreatedAt); got.Cap != 5_000_000 {
		t.Fatalf("cap after approval = %d, want the approved 5,000,000", got.Cap)
	}

	pending, err := d.ListRequests(ctx, quota.RequestFilter{Status: quota.Pending})
	if err != nil {
		t.Fatalf("ListRequests: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %+v, want none left", pending)
	}
}

// A tier nothing resolves would write an override nobody reads, so it is
// refused at submission.
func TestSubmitRejectsAnUnknownTier(t *testing.T) {
	d := deps(t)
	_, err := d.Submit(context.Background(), karakuriquota.SubmitRequest{
		Tier: "wishes", TwinID: "t1", Cap: 10, Reason: "why", RequestedBy: "alice",
	})
	if err == nil {
		t.Fatal("an unknown tier was accepted")
	}
	// The error names what is allowed, because the next thing the operator does
	// is retype the command.
	for _, tier := range karakuriquota.RequestableTiers() {
		if !strings.Contains(err.Error(), tier) {
			t.Errorf("err = %v, want it to list %q", err, tier)
		}
	}
}

// IDs appear in approval links, so a guessable one invites somebody to try the
// next number up.
func TestRequestIDsAreNotSequential(t *testing.T) {
	ctx := context.Background()
	d := deps(t)
	seen := map[string]bool{}
	for range 20 {
		req, err := d.Submit(ctx, karakuriquota.SubmitRequest{
			Tier: karakuriquota.TierLLMTokens, TwinID: "t1",
			Cap: 10, Reason: "why", RequestedBy: "alice",
		})
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
		if seen[req.ID] {
			t.Fatalf("duplicate request id %q", req.ID)
		}
		seen[req.ID] = true
	}
}
