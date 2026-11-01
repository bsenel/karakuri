package objective

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Phase 41 step 8, the acceptance's last open clause: "A person's target
// change restarts the window." Targets are fixed at create time and the loop
// has no capability that writes them; a change goes through the API by a
// person, is audited, and restarts the criterion's sustain window
// (docs/roadmap.md, Phase 41 section 6 'Targets belong to their author').
//
// The restart is recorded the way SetBaseline records it, as the criterion's
// SustainSince, which reconcile already reads (TestPassesBeforeARestartAreOut
// OfTheSustainCounts), so a target change needs no new reconcile path. The
// change is audited in one target row naming the person.

// personTargetRow is the payload of the target row a person's change writes.
type personTargetRow struct {
	Criterion    string     `json:"criterion"`
	Previous     *float64   `json:"previous"`
	Value        *float64   `json:"value"`
	Unit         string     `json:"unit"`
	SetBy        string     `json:"set_by"`
	SustainSince *time.Time `json:"sustain_since"`
	LoopID       *string    `json:"loop_id"`
}

func targetRows(t *testing.T, svc *Service, id coreobjective.ObjectiveID) []personTargetRow {
	t.Helper()
	rows, err := svc.store.ListToolEvents(context.Background(), storage.ToolEventFilter{ObjectiveID: string(id)})
	if err != nil {
		t.Fatalf("list tool events: %v", err)
	}
	var out []personTargetRow
	for _, r := range rows {
		if r.Kind != storage.ToolEventTarget {
			continue
		}
		var p personTargetRow
		if err := json.Unmarshal([]byte(r.PayloadJSON), &p); err != nil {
			t.Fatalf("target row %s: payload is not JSON: %v (%q)", r.ID, err, r.PayloadJSON)
		}
		out = append(out, p)
	}
	return out
}

// A person changing the target writes the new target, restarts the window at
// the time of the change, leaves the sustain and the baseline as they were,
// and audits the change once with the target it replaced.
func TestSetTargetRestartsTheSustainWindow(t *testing.T) {
	svc := newCriteriaService(t)
	o, err := svc.Create(context.Background(), CreateRequest{
		Title: "standing", Domain: "software",
		SuccessCriteria: []coreobjective.Criterion{sustainedRelativeLatency()},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	before := time.Now().UTC()
	answered, err := svc.SetTarget(context.Background(), o.ID, "latency-relative", 5, baselineSetter)
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("set target: %v", err)
	}

	for name, c := range map[string]coreobjective.Criterion{
		"answered": criterionByID(t, answered.SuccessCriteria, "latency-relative"),
		"stored":   storedCriterion(t, svc, o.ID, "latency-relative"),
	} {
		if c.Target == nil || *c.Target != 5 {
			t.Errorf("%s target = %s, want the person's 5", name, valueOf(c.Target))
		}
		if c.SustainSince == nil {
			t.Errorf("%s sustain_since is unset after a person changed the target; the window must restart", name)
		} else if c.SustainSince.Before(before) || c.SustainSince.After(after) {
			t.Errorf("%s sustain_since = %v, want the time of the change, between %v and %v", name, *c.SustainSince, before, after)
		}
		if c.Sustain == nil || *c.Sustain != *acceptanceSustain() {
			t.Errorf("%s sustain = %+v after the change, want it unchanged %+v", name, c.Sustain, acceptanceSustain())
		}
		if c.BaselineValue == nil || *c.BaselineValue != 200 {
			t.Errorf("%s baseline_value = %s after a target change, want the frozen 200 untouched", name, valueOf(c.BaselineValue))
		}
	}

	rows := targetRows(t, svc, o.ID)
	if len(rows) != 1 {
		t.Fatalf("%d target audit rows, want exactly 1: %+v", len(rows), rows)
	}
	r := rows[0]
	if r.Criterion != "latency-relative" {
		t.Errorf("row criterion = %q, want latency-relative (row %+v)", r.Criterion, r)
	}
	if valueOf(r.Previous) != "10" {
		t.Errorf("row previous = %s, want the replaced target 10 (row %+v)", valueOf(r.Previous), r)
	}
	if r.Value == nil || *r.Value != 5 {
		t.Errorf("row value = %s, want 5 (row %+v)", valueOf(r.Value), r)
	}
	if r.Unit != "ms" {
		t.Errorf("row unit = %q, want ms (row %+v)", r.Unit, r)
	}
	if r.SetBy != baselineSetter {
		t.Errorf("row set_by = %q, want %q: the row names the person who changed it", r.SetBy, baselineSetter)
	}
	if r.LoopID != nil {
		t.Errorf("row loop_id = %q: a person's change is no run's", *r.LoopID)
	}
	if stored := storedCriterion(t, svc, o.ID, "latency-relative").SustainSince; r.SustainSince == nil || stored == nil || !r.SustainSince.Equal(*stored) {
		t.Errorf("row sustain_since = %v, want the stored restart %v", r.SustainSince, stored)
	}
	if rows := baselineRows(t, svc, o.ID); len(rows) != 0 {
		t.Errorf("a target change wrote %d baseline rows, want none: it is not a baseline change: %+v", len(rows), rows)
	}
}

// A second change restarts the window again, later than the first.
func TestASecondSetTargetRestartsTheSustainWindowAgain(t *testing.T) {
	svc := newCriteriaService(t)
	o, err := svc.Create(context.Background(), CreateRequest{
		Title: "standing", Domain: "software",
		SuccessCriteria: []coreobjective.Criterion{sustainedRelativeLatency()},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.SetTarget(context.Background(), o.ID, "latency-relative", 5, baselineSetter); err != nil {
		t.Fatalf("first change: %v", err)
	}
	first := storedCriterion(t, svc, o.ID, "latency-relative").SustainSince
	if first == nil {
		t.Fatal("sustain_since is unset after the first change")
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := svc.SetTarget(context.Background(), o.ID, "latency-relative", 8, baselineSetter); err != nil {
		t.Fatalf("second change: %v", err)
	}
	second := storedCriterion(t, svc, o.ID, "latency-relative").SustainSince
	if second == nil || !second.After(*first) {
		t.Errorf("sustain_since after the second change = %v, want later than the first %v", second, *first)
	}
	if rows := targetRows(t, svc, o.ID); len(rows) != 2 {
		t.Errorf("%d target audit rows after two changes, want 2, one per change: %+v", len(rows), rows)
	}
}

// A value that is not finite, or a criterion with no target to change, is
// refused for the field the person has to look at; an unknown criterion is not
// found. A refused change writes nothing and restarts nothing.
func TestARefusedSetTargetLeavesTheTargetAndTheSustainWindow(t *testing.T) {
	for _, tc := range []struct {
		name      string
		criterion string
		value     float64
		field     string
		notFound  bool
	}{
		{name: "NaN", criterion: "latency-fixed", value: math.NaN(), field: "value"},
		{name: "+Inf", criterion: "latency-fixed", value: math.Inf(1), field: "value"},
		{name: "-Inf", criterion: "latency-fixed", value: math.Inf(-1), field: "value"},
		{name: "a verified criterion", criterion: "tested", value: 5, field: "kind"},
		{name: "an unknown criterion", criterion: "no-such-criterion", value: 5, notFound: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, o := baselineObjective(t)

			_, err := svc.SetTarget(context.Background(), o.ID, tc.criterion, tc.value, baselineSetter)
			if tc.notFound {
				if !errors.Is(err, ErrCriterionNotFound) {
					t.Fatalf("err = %v, want ErrCriterionNotFound", err)
				}
			} else {
				var invalid *ValidationError
				if !errors.As(err, &invalid) {
					t.Fatalf("err = %v, want a *ValidationError naming %s", err, tc.field)
				}
				if invalid.Field != tc.field {
					t.Errorf("field = %q, want %q (%v)", invalid.Field, tc.field, err)
				}
			}

			for _, c := range []string{"latency-fixed", "tested"} {
				before := criterionByID(t, o.SuccessCriteria, c)
				after := storedCriterion(t, svc, o.ID, c)
				if valueOf(after.Target) != valueOf(before.Target) {
					t.Errorf("%s target = %s after a refused change, want %s unchanged", c, valueOf(after.Target), valueOf(before.Target))
				}
				if after.SustainSince != nil {
					t.Errorf("%s sustain_since = %v after a refused change, want none", c, *after.SustainSince)
				}
			}
			if rows := targetRows(t, svc, o.ID); len(rows) != 0 {
				t.Errorf("a refused change wrote %d target audit rows: %+v", len(rows), rows)
			}
		})
	}
}
