package report

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/digest"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

func seedRefusal(t *testing.T, store storage.StorageAdapter, rowID, objID, capability, reason, payload string, at time.Time) {
	t.Helper()
	err := store.SaveToolEvent(context.Background(), storage.ToolEvent{
		ID: rowID, ObjectiveID: objID, Capability: capability,
		Kind: storage.ToolEventRefusal, BoundsViolation: true,
		EscalationReason: reason, PayloadJSON: payload, CreatedAt: at,
	})
	if err != nil {
		t.Fatalf("seed refusal %s: %v", rowID, err)
	}
}

func refusalReasons(r []digest.Refusal) []string {
	out := make([]string, len(r))
	for i, x := range r {
		out[i] = x.Reason
	}
	return out
}

func blockOf(blocks []digest.Block, kind digest.BlockKind) (digest.Block, bool) {
	for _, b := range blocks {
		if b.Kind == kind {
			return b, true
		}
	}
	return digest.Block{}, false
}

func summaryOf(t *testing.T, d digest.Digest, id string) digest.ObjectiveSummary {
	t.Helper()
	for _, o := range d.Objectives {
		if string(o.ID) == id {
			return o
		}
	}
	t.Fatalf("no summary for %s in %v", id, titles(d))
	return digest.ObjectiveSummary{}
}

// Phase 40 step 14 acceptance: from a seeded audit log and store, the digest
// for a window lists exactly the seeded refusals and blocks per stream and
// nothing outside the window, the passes keep appearing, and assembling the
// same window twice gives identical content.
func TestDigestListsEachStreamsRefusalsAndBlocks(t *testing.T) {
	svc, store, clock := newService(t)
	ctx := context.Background()
	seedTwin(t, store, "twin-1", "CTO twin")
	a := seedStanding(t, store, "twin-1", "obj-a", "stream a")
	b := seedStanding(t, store, "twin-1", "obj-b", "stream b")
	seedStanding(t, store, "twin-1", "obj-c", "stream c, quiet")

	until := *clock
	since := until.Add(-24 * time.Hour)
	in := func(h int) time.Time { return since.Add(time.Duration(h) * time.Hour) }
	quotaBack := until.Add(3 * time.Hour)

	// Stream a: two refused plans and one pushed-refs violation in the
	// window; one refusal before it and one after it, which must not appear.
	seedRefusal(t, store, "a-plan-1", string(a.ID), "", "plan 1: action outside limits", `{"actions":[{"capability_id":"code.write"}]}`, in(2))
	seedRefusal(t, store, "a-push", string(a.ID), "git.push", "push to ref it never named", `{"capability":"git.push","refs":["refs/heads/main"]}`, in(5))
	seedRefusal(t, store, "a-plan-2", string(a.ID), "", "plan 2: action outside limits", `{"actions":[{"capability_id":"code.write"}]}`, in(9))
	seedRefusal(t, store, "a-old", string(a.ID), "", "before the window", `{}`, since.Add(-time.Hour))
	seedRefusal(t, store, "a-late", string(a.ID), "", "after the window", `{}`, until.Add(time.Hour))
	// Stream b: one refused plan, in the window.
	seedRefusal(t, store, "b-plan", string(b.ID), "", "plan b: action outside limits", `{}`, in(4))

	// Passes. a's latest pass ended out of quota until quotaBack; b's last
	// pass could see no environment (an unreadable remote).
	mustOutcome(t, store, reconcile.Outcome{
		ID: "a-pass-1", ObjectiveID: a.ID, LoopID: "loop-a1", Trigger: reconcile.TriggerDrift,
		Drift: reconcile.Drift{Changed: true}, Converged: true, CriteriaMet: 1, StartedAt: in(1),
	})
	mustOutcome(t, store, reconcile.Outcome{
		ID: "a-pass-2", ObjectiveID: a.ID, LoopID: "loop-a2", Trigger: reconcile.TriggerDrift,
		Deferred: "waiting_for_quota", DeferredUntil: quotaBack, StartedAt: in(10),
	})
	mustOutcome(t, store, reconcile.Outcome{
		ID: "a-old-pass", ObjectiveID: a.ID, LoopID: "loop-a0", Trigger: reconcile.TriggerDrift,
		Deferred: "waiting_for_quota", DeferredUntil: since.Add(-time.Hour), StartedAt: since.Add(-3 * time.Hour),
	})
	mustOutcome(t, store, reconcile.Outcome{
		ID: "b-sense", ObjectiveID: b.ID, Trigger: reconcile.TriggerSchedule,
		Drift: reconcile.Drift{Blind: true}, StartedAt: in(6),
	})

	d, err := svc.Assemble(ctx, "twin-1", since, until)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	sa, sb, sc := summaryOf(t, d, "obj-a"), summaryOf(t, d, "obj-b"), summaryOf(t, d, "obj-c")

	// Refusals: exactly the seeded ones in the window, oldest first.
	want := []string{"plan 1: action outside limits", "push to ref it never named", "plan 2: action outside limits"}
	if got := refusalReasons(sa.Refusals); !equalStrings(got, want) {
		t.Errorf("stream a refusals = %q, want %q", got, want)
	}
	if got := refusalReasons(sb.Refusals); !equalStrings(got, []string{"plan b: action outside limits"}) {
		t.Errorf("stream b refusals = %q, want its one plan refusal and none of a's", got)
	}
	if len(sc.Refusals) != 0 || len(sc.Blocks) != 0 {
		t.Errorf("quiet stream c has refusals %v blocks %v, want none", sc.Refusals, sc.Blocks)
	}
	if len(sa.Refusals) > 1 && sa.Refusals[1].Capability != "git.push" {
		t.Errorf("the pushed-refs refusal lost its capability: %+v", sa.Refusals[1])
	}

	// Blocks: a is out of quota until quotaBack and violated its bounds
	// with a push; b cannot read its remote. The deferral before the window
	// is not a block.
	if len(sa.Blocks) != 2 {
		t.Fatalf("stream a blocks = %+v, want quota and bounds_violation", sa.Blocks)
	}
	if q, ok := blockOf(sa.Blocks, digest.BlockQuota); !ok || !q.Until.Equal(quotaBack) {
		t.Errorf("stream a quota block = %+v (found %v), want until %s", q, ok, quotaBack)
	}
	if v, ok := blockOf(sa.Blocks, digest.BlockBoundsViolation); !ok || v.Reason != "push to ref it never named" {
		t.Errorf("stream a violation block = %+v (found %v), want the push's reason", v, ok)
	}
	if len(sb.Blocks) != 1 {
		t.Fatalf("stream b blocks = %+v, want only remote_unreadable", sb.Blocks)
	}
	if _, ok := blockOf(sb.Blocks, digest.BlockRemoteUnreadable); !ok {
		t.Errorf("stream b blocks = %+v, want remote_unreadable", sb.Blocks)
	}

	// The passes already in the digest keep appearing.
	if sa.Reconciles != 2 || sa.Converged != 1 || sa.DriftDetected != 1 {
		t.Errorf("stream a passes: reconciles %d converged %d drift %d, want 2/1/1", sa.Reconciles, sa.Converged, sa.DriftDetected)
	}
	if sb.Senses != 1 {
		t.Errorf("stream b senses = %d, want 1", sb.Senses)
	}

	// The same window twice is the same content.
	again, err := svc.Assemble(ctx, "twin-1", since, until)
	if err != nil {
		t.Fatalf("assemble again: %v", err)
	}
	first, _ := json.Marshal(d)
	second, _ := json.Marshal(again)
	if string(first) != string(second) {
		t.Errorf("assembling the same window twice differs:\n%s\n%s", first, second)
	}
}

// A refusal and a block outside the window are not in a digest for it; the
// window moves what is listed, not the log.
func TestDigestRefusalsFollowTheWindow(t *testing.T) {
	svc, store, clock := newService(t)
	seedTwin(t, store, "twin-1", "CTO twin")
	a := seedStanding(t, store, "twin-1", "obj-a", "stream a")
	seedRefusal(t, store, "r1", string(a.ID), "", "yesterday", `{}`, clock.Add(-30*time.Hour))
	seedRefusal(t, store, "r2", string(a.ID), "", "today", `{}`, clock.Add(-2*time.Hour))

	d, err := svc.Assemble(context.Background(), "twin-1", clock.Add(-48*time.Hour), clock.Add(-26*time.Hour))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if got := refusalReasons(summaryOf(t, d, "obj-a").Refusals); !equalStrings(got, []string{"yesterday"}) {
		t.Errorf("refusals = %q, want only the one inside the window", got)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

