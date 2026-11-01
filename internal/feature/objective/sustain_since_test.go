package objective

import (
	"context"
	"math"
	"testing"
	"time"

	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 41 step 8 part C: targets, baselines and sustain budgets are fixed at
// create time; a change goes through the API by a person, is audited, and
// restarts the criterion's sustain window (docs/roadmap.md, Phase 41 section 6
// 'Targets belong to their author').
//
// The restart is recorded on the criterion as SustainSince (sustain_since), a
// timestamp SetBaseline writes beside the value it saves, rather than read
// back from the baseline audit row: SetBaseline already holds the criterion
// and saves the objective, and reconcile already reads the objective, so the
// window start travels with the criterion and needs no audit query on every
// pass. The audit row stays the record of who changed what.

// sustainedRelativeLatency is relativeLatency (baseline 200 ms fixed by hand,
// +10%) held with step 8's sustain.
func sustainedRelativeLatency() coreobjective.Criterion {
	return relativeLatency(func(c *coreobjective.Criterion) { c.Sustain = acceptanceSustain() })
}

// A new criterion's window was never restarted.
func TestCreateLeavesSustainSinceUnset(t *testing.T) {
	c := createOne(t, sustainedRelativeLatency())
	if c.SustainSince != nil {
		t.Errorf("stored sustain_since = %v at create time, want none: nothing has restarted the window", c.SustainSince)
	}
}

// A request does not declare when the window restarted: that is the server's
// record of a person's audited change, and a request that could set it could
// exclude passes it did not like.
func TestCreateRejectsARequestSustainSince(t *testing.T) {
	svc := newCriteriaService(t)
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	c := sustainedRelativeLatency()
	c.SustainSince = &since
	assertRejected(t, svc, CreateRequest{
		Title: "standing", Domain: "software",
		SuccessCriteria: []coreobjective.Criterion{
			{ID: "tested", Description: "the tests pass", Kind: coreobjective.KindVerified, Verifier: testsVerifier},
			c,
		},
	}, "success_criteria[1].sustain_since")
}

// A person setting the baseline restarts the window: the stored criterion
// carries sustain_since, the time of the set, and the audited baseline row is
// written as before. The sustain itself is unchanged.
func TestSetBaselineRestartsTheSustainWindow(t *testing.T) {
	svc := newCriteriaService(t)
	o, err := svc.Create(context.Background(), CreateRequest{
		Title: "standing", Domain: "software",
		SuccessCriteria: []coreobjective.Criterion{sustainedRelativeLatency()},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	before := time.Now().UTC()
	answered, err := svc.SetBaseline(context.Background(), o.ID, "latency-relative", 180, baselineSetter)
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("set baseline: %v", err)
	}

	for name, c := range map[string]coreobjective.Criterion{
		"answered": criterionByID(t, answered.SuccessCriteria, "latency-relative"),
		"stored":   storedCriterion(t, svc, o.ID, "latency-relative"),
	} {
		if c.SustainSince == nil {
			t.Errorf("%s sustain_since is unset after a person set the baseline; the window must restart", name)
			continue
		}
		if c.SustainSince.Before(before) || c.SustainSince.After(after) {
			t.Errorf("%s sustain_since = %v, want the time of the set, between %v and %v", name, *c.SustainSince, before, after)
		}
		if c.Sustain == nil || *c.Sustain != *acceptanceSustain() {
			t.Errorf("%s sustain = %+v after the set, want it unchanged %+v", name, c.Sustain, acceptanceSustain())
		}
	}
	if rows := baselineRows(t, svc, o.ID); len(rows) != 1 || rows[0].SetBy != baselineSetter {
		t.Errorf("baseline rows = %+v, want the one person's row", rows)
	}
}

// A second set restarts it again, later than the first.
func TestASecondSetBaselineRestartsTheSustainWindowAgain(t *testing.T) {
	svc := newCriteriaService(t)
	o, err := svc.Create(context.Background(), CreateRequest{
		Title: "standing", Domain: "software",
		SuccessCriteria: []coreobjective.Criterion{sustainedRelativeLatency()},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.SetBaseline(context.Background(), o.ID, "latency-relative", 180, baselineSetter); err != nil {
		t.Fatalf("first set: %v", err)
	}
	first := storedCriterion(t, svc, o.ID, "latency-relative").SustainSince
	if first == nil {
		t.Fatal("sustain_since is unset after the first set")
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := svc.SetBaseline(context.Background(), o.ID, "latency-relative", 170, baselineSetter); err != nil {
		t.Fatalf("second set: %v", err)
	}
	second := storedCriterion(t, svc, o.ID, "latency-relative").SustainSince
	if second == nil || !second.After(*first) {
		t.Errorf("sustain_since after the second set = %v, want later than the first %v", second, *first)
	}
}

// A refused set changes nothing, so it restarts nothing.
func TestARefusedSetBaselineLeavesTheSustainWindow(t *testing.T) {
	svc := newCriteriaService(t)
	o, err := svc.Create(context.Background(), CreateRequest{
		Title: "standing", Domain: "software",
		SuccessCriteria: []coreobjective.Criterion{sustainedRelativeLatency()},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.SetBaseline(context.Background(), o.ID, "latency-relative", math.NaN(), baselineSetter); err == nil {
		t.Fatal("a NaN baseline was accepted")
	}
	if c := storedCriterion(t, svc, o.ID, "latency-relative"); c.SustainSince != nil {
		t.Errorf("sustain_since = %v after a refused set, want none", *c.SustainSince)
	}
}
