package objective

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"

	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Phase 41 step 7 part D2: a person sets a measured criterion's baseline
// through the API. The value is written as the criterion's BaselineValue,
// replacing a frozen one, persisted, and each change is audited in one
// baseline row that names the person, so it can be told apart from the loop's
// at_start row (docs/roadmap.md, Phase 41 decision 3 'Baselines', decision 6
// 'Targets belong to their author', and step 7's acceptance).

const baselineSetter = "alice@example.com"

// baselineObjective is created from the delivery template, so it has a
// verified criterion (tested) and a measured one with no change (coverage),
// plus two relative latency criteria from the request: one at_start and not
// measured yet, one fixed by hand at 200 and so frozen at create time.
func baselineObjective(t *testing.T) (*Service, coreobjective.Objective) {
	t.Helper()
	svc := newCriteriaService(t)
	o, err := svc.Create(context.Background(), CreateRequest{
		Title: "keep latency where it started", Domain: "software", TemplateID: "tmpl.delivery",
		SuccessCriteria: []coreobjective.Criterion{
			relativeLatency(func(c *coreobjective.Criterion) {
				c.ID, c.Baseline = "latency-at-start", &coreobjective.Baseline{AtStart: true}
			}),
			relativeLatency(func(c *coreobjective.Criterion) { c.ID = "latency-fixed" }),
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return svc, o
}

// personBaselineRow is the payload of a baseline row a person's set writes.
type personBaselineRow struct {
	Criterion string   `json:"criterion"`
	Previous  *float64 `json:"previous"`
	Value     *float64 `json:"value"`
	Unit      string   `json:"unit"`
	SetBy     string   `json:"set_by"`
	LoopID    *string  `json:"loop_id"`
}

func baselineRows(t *testing.T, svc *Service, id coreobjective.ObjectiveID) []personBaselineRow {
	t.Helper()
	rows, err := svc.store.ListToolEvents(context.Background(), storage.ToolEventFilter{ObjectiveID: string(id)})
	if err != nil {
		t.Fatalf("list tool events: %v", err)
	}
	var out []personBaselineRow
	for _, r := range rows {
		if r.Kind != storage.ToolEventBaseline {
			continue
		}
		var p personBaselineRow
		if err := json.Unmarshal([]byte(r.PayloadJSON), &p); err != nil {
			t.Fatalf("baseline row %s: payload is not JSON: %v (%q)", r.ID, err, r.PayloadJSON)
		}
		out = append(out, p)
	}
	return out
}

func storedCriterion(t *testing.T, svc *Service, id coreobjective.ObjectiveID, criterionID string) coreobjective.Criterion {
	t.Helper()
	o, err := svc.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	return criterionByID(t, o.SuccessCriteria, criterionID)
}

func valueOf(v *float64) string {
	if v == nil {
		return "none"
	}
	b, _ := json.Marshal(*v)
	return string(b)
}

// assertPersonRow checks one row a person's set wrote: it names the criterion,
// the value it replaced (or none), the new value, the unit and the person, and
// carries no loop ID, because no run took it.
func assertPersonRow(t *testing.T, r personBaselineRow, criterion string, previous *float64, value float64) {
	t.Helper()
	if r.Criterion != criterion {
		t.Errorf("row criterion = %q, want %q (row %+v)", r.Criterion, criterion, r)
	}
	if valueOf(r.Previous) != valueOf(previous) {
		t.Errorf("row previous = %s, want %s (row %+v)", valueOf(r.Previous), valueOf(previous), r)
	}
	if r.Value == nil || *r.Value != value {
		t.Errorf("row value = %s, want %v (row %+v)", valueOf(r.Value), value, r)
	}
	if r.Unit != "ms" {
		t.Errorf("row unit = %q, want ms (row %+v)", r.Unit, r)
	}
	if r.SetBy != baselineSetter {
		t.Errorf("row set_by = %q, want %q: the row names the person who set it", r.SetBy, baselineSetter)
	}
	if r.LoopID != nil {
		t.Errorf("row loop_id = %q: a person's set is no run's, and must not read as the loop's at_start row", *r.LoopID)
	}
}

// A person sets an at_start baseline that was never measured: the value is
// returned, persisted, and audited once with no previous value.
func TestSetBaselineWritesThePersonsValueAndAuditsIt(t *testing.T) {
	svc, o := baselineObjective(t)

	updated, err := svc.SetBaseline(context.Background(), o.ID, "latency-at-start", 200, baselineSetter)
	if err != nil {
		t.Fatalf("set baseline: %v", err)
	}
	if got := criterionByID(t, updated.SuccessCriteria, "latency-at-start"); got.BaselineValue == nil || *got.BaselineValue != 200 {
		t.Errorf("returned baseline_value = %s, want 200", valueOf(got.BaselineValue))
	}
	if got := storedCriterion(t, svc, o.ID, "latency-at-start"); got.BaselineValue == nil || *got.BaselineValue != 200 {
		t.Errorf("stored baseline_value = %s, want 200, persisted", valueOf(got.BaselineValue))
	}
	if got := storedCriterion(t, svc, o.ID, "latency-fixed"); got.BaselineValue == nil || *got.BaselineValue != 200 {
		t.Errorf("another criterion's baseline_value = %s, want its own 200 untouched", valueOf(got.BaselineValue))
	}

	rows := baselineRows(t, svc, o.ID)
	if len(rows) != 1 {
		t.Fatalf("%d baseline audit rows, want exactly 1: %+v", len(rows), rows)
	}
	assertPersonRow(t, rows[0], "latency-at-start", nil, 200)
}

// A person may replace a frozen baseline, one fixed at create time or one they
// set before. Each change is audited with the value it replaced.
func TestSetBaselineReplacesAFrozenValueAndAuditsEachChange(t *testing.T) {
	svc, o := baselineObjective(t)

	for _, value := range []float64{180, 170} {
		if _, err := svc.SetBaseline(context.Background(), o.ID, "latency-fixed", value, baselineSetter); err != nil {
			t.Fatalf("set baseline %v: %v", value, err)
		}
		if got := storedCriterion(t, svc, o.ID, "latency-fixed"); got.BaselineValue == nil || *got.BaselineValue != value {
			t.Errorf("stored baseline_value = %s, want %v: a person may replace a frozen baseline", valueOf(got.BaselineValue), value)
		}
	}

	rows := baselineRows(t, svc, o.ID)
	if len(rows) != 2 {
		t.Fatalf("%d baseline audit rows after two changes, want 2, one per change: %+v", len(rows), rows)
	}
	// The rows' order in the store is not part of the contract; each change is
	// identified by the value it set.
	byValue := map[float64]personBaselineRow{}
	for _, r := range rows {
		if r.Value != nil {
			byValue[*r.Value] = r
		}
	}
	assertPersonRow(t, byValue[180], "latency-fixed", target(200), 180)
	assertPersonRow(t, byValue[170], "latency-fixed", target(180), 170)
}

// An unknown objective or criterion is not found, and nothing is written.
func TestSetBaselineRefusesAnUnknownObjectiveOrCriterion(t *testing.T) {
	svc, o := baselineObjective(t)

	if _, err := svc.SetBaseline(context.Background(), "no-such-objective", "latency-at-start", 200, baselineSetter); !errors.Is(err, coreerrors.ErrObjectiveNotFound) {
		t.Errorf("unknown objective: err = %v, want ErrObjectiveNotFound", err)
	}
	if _, err := svc.SetBaseline(context.Background(), o.ID, "no-such-criterion", 200, baselineSetter); !errors.Is(err, ErrCriterionNotFound) {
		t.Errorf("unknown criterion: err = %v, want ErrCriterionNotFound", err)
	}
	if rows := baselineRows(t, svc, o.ID); len(rows) != 0 {
		t.Errorf("a refused set wrote %d baseline audit rows: %+v", len(rows), rows)
	}
}

// A value that is not finite, a criterion that is not measured, or one with no
// change is refused for the field the person has to look at, and nothing is
// written: a baseline only means something to a measured criterion with a
// change (validateBaseline and refuseBaseline say the same at create time).
func TestSetBaselineRefusesWhatABaselineCannotMean(t *testing.T) {
	for _, tc := range []struct {
		name      string
		criterion string
		value     float64
		field     string
	}{
		{name: "NaN", criterion: "latency-at-start", value: math.NaN(), field: "value"},
		{name: "+Inf", criterion: "latency-at-start", value: math.Inf(1), field: "value"},
		{name: "-Inf", criterion: "latency-at-start", value: math.Inf(-1), field: "value"},
		{name: "a verified criterion", criterion: "tested", value: 200, field: "kind"},
		{name: "a measured criterion with no change", criterion: "coverage", value: 80, field: "change"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, o := baselineObjective(t)
			before := storedCriterion(t, svc, o.ID, tc.criterion)

			_, err := svc.SetBaseline(context.Background(), o.ID, tc.criterion, tc.value, baselineSetter)
			var invalid *ValidationError
			if !errors.As(err, &invalid) {
				t.Fatalf("err = %v, want a *ValidationError naming %s", err, tc.field)
			}
			if invalid.Field != tc.field {
				t.Errorf("field = %q, want %q (%v)", invalid.Field, tc.field, err)
			}
			if after := storedCriterion(t, svc, o.ID, tc.criterion); valueOf(after.BaselineValue) != valueOf(before.BaselineValue) {
				t.Errorf("stored baseline_value = %s after a refused set, want %s unchanged",
					valueOf(after.BaselineValue), valueOf(before.BaselineValue))
			}
			if rows := baselineRows(t, svc, o.ID); len(rows) != 0 {
				t.Errorf("a refused set wrote %d baseline audit rows: %+v", len(rows), rows)
			}
		})
	}
}
