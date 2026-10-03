package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// fakeExportStore answers the three reads the way the real store does: the
// window bounds on tool events, the resolution window on resolved checkpoints.
// It returns rows in the order they were given, so a test can shuffle them.
type fakeExportStore struct {
	events      []storage.ToolEvent
	checkpoints []checkpoint.Checkpoint
	filters     []storage.ToolEventFilter
}

func (f *fakeExportStore) ListToolEvents(_ context.Context, flt storage.ToolEventFilter) ([]storage.ToolEvent, error) {
	f.filters = append(f.filters, flt)
	out := []storage.ToolEvent{}
	for _, e := range f.events {
		if flt.CreatedAtSince != nil && e.CreatedAt.Before(*flt.CreatedAtSince) {
			continue
		}
		if flt.CreatedAtBefore != nil && !e.CreatedAt.Before(*flt.CreatedAtBefore) {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func (f *fakeExportStore) ListPendingCheckpoints(_ context.Context, _ string) ([]checkpoint.Checkpoint, error) {
	out := []checkpoint.Checkpoint{}
	for _, c := range f.checkpoints {
		if c.Status == checkpoint.StatusPending {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeExportStore) ListResolvedCheckpoints(_ context.Context, flt storage.ResolvedCheckpointFilter) ([]checkpoint.Checkpoint, error) {
	out := []checkpoint.Checkpoint{}
	for _, c := range f.checkpoints {
		if c.Status != checkpoint.StatusResolved || c.ResolvedAt == nil {
			continue
		}
		if !flt.Since.IsZero() && c.ResolvedAt.Before(flt.Since) {
			continue
		}
		if !flt.Until.IsZero() && !c.ResolvedAt.Before(flt.Until) {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

var (
	exportFrom = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	exportTo   = time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	exportNow  = time.Date(2026, 6, 17, 9, 41, 23, 0, time.UTC)

	exportRetention = Retention{FloorDays: 183, Days: 0}
)

const decisionPayload = `{"agent_definition_id":"software.agent.reviewer","reasoning_strategy":"react","max_autonomous":3,"confidence_threshold":0.9,"effective_threshold":0.85,"requires_approval_for":["software.cap.merge"],"risk_class":"consequential"}`

func exportTemplates() []objective.Template {
	return []objective.Template{
		{ID: "software.review", Title: "Review", Domain: "software", Risk: objective.RiskConsequential},
		{ID: "finance.close", Title: "Close", Domain: "finance", Risk: objective.RiskHigh},
		{ID: "ops.rotate", Title: "Rotate", Domain: "ops", Risk: objective.RiskRoutine},
	}
}

// everyKind is one row of every kind inside the window, a minute apart.
func everyKind() []storage.ToolEvent {
	at := func(m int) time.Time { return exportFrom.Add(time.Duration(m) * time.Minute) }
	decision := func(id, kind string, m int) storage.ToolEvent {
		return storage.ToolEvent{
			ID: id, ObjectiveID: "obj-1", AgentID: "agent-1", Capability: "software.cap.review", Kind: kind,
			Success: true, Confidence: 0.8, PayloadJSON: decisionPayload,
			Provider: "anthropic", Model: "claude-opus-5-5", TemplateID: "software.review", AutonomyRung: "supervised",
			CreatedAt: at(m),
		}
	}
	return []storage.ToolEvent{
		decision("ev-execute", storage.ToolEventExecute, 1),
		decision("ev-escalation", storage.ToolEventEscalation, 2),
		{ID: "ev-approval", ObjectiveID: "obj-1", Kind: storage.ToolEventApproval, Approver: "alice", Success: true,
			PayloadJSON: `{"checkpoint_id":"cp-approved","choice":"approve","note":"","linked_audit_event":"ev-escalation"}`, CreatedAt: at(3)},
		{ID: "ev-rejection", ObjectiveID: "obj-1", Kind: storage.ToolEventRejection, Approver: "bob", Success: true,
			PayloadJSON: `{"checkpoint_id":"cp-rejected","choice":"reject","note":"no","linked_audit_event":""}`, CreatedAt: at(4)},
		{ID: "ev-modification", ObjectiveID: "obj-1", Kind: storage.ToolEventModification, Approver: "carol", Success: true,
			PayloadJSON: `{"checkpoint_id":"cp-modified","choice":"modify","note":"","linked_audit_event":"","modifications":{"removed_actions":["software.cap.merge"],"added_constraints":["do not merge"]}}`, CreatedAt: at(5)},
		{ID: "ev-promotion", ObjectiveID: "obj-1", Kind: storage.ToolEventPromotion, Success: true,
			PayloadJSON: `{"from":"supervised","to":"autonomous"}`, CreatedAt: at(6)},
		{ID: "ev-demotion", ObjectiveID: "obj-1", Kind: storage.ToolEventDemotion, Success: true,
			PayloadJSON: `{"from":"autonomous","to":"supervised"}`, CreatedAt: at(7)},
		{ID: "ev-denied", ObjectiveID: "", Kind: storage.ToolEventAuthzDenied, Success: false,
			PayloadJSON: `{"subject":"mallory","action":"checkpoint.resolve"}`, CreatedAt: at(8)},
	}
}

func raised(id string, at time.Time) checkpoint.Checkpoint {
	return checkpoint.Checkpoint{ID: id, ObjectiveID: "obj-1", TwinID: "twin-1", Summary: "review " + id, Status: checkpoint.StatusPending, CreatedAt: at}
}

func resolved(id string, at, resolvedAt time.Time) checkpoint.Checkpoint {
	c := raised(id, at)
	c.Status = checkpoint.StatusResolved
	c.ResolvedAt = &resolvedAt
	c.Decision = &checkpoint.Decision{Choice: "approve", Approver: "alice"}
	return c
}

func exportBytes(t *testing.T, store exportStore, r Retention, from, to, now time.Time) []byte {
	t.Helper()
	b, err := NewExporter(store, r, exportTemplates()).Export(context.Background(), from, to, now)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	return b
}

func exportDoc(t *testing.T, store exportStore, r Retention, from, to time.Time) (Export, []byte) {
	t.Helper()
	b := exportBytes(t, store, r, from, to, exportNow)
	var doc Export
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("export is not JSON: %v\n%s", err, b)
	}
	return doc, b
}

func rowByID(rows []ExportRow, id string) (ExportRow, bool) {
	for _, r := range rows {
		if r.ID == id {
			return r, true
		}
	}
	return ExportRow{}, false
}

func pendingIDs(doc Export) []string {
	ids := []string{}
	for _, p := range doc.PendingCheckpoints {
		ids = append(ids, p.ID)
	}
	slices.Sort(ids)
	return ids
}

// The point of the export: a past window is a fixed thing, so asking for it
// again, on another day, from a store that hands the rows back in another
// order, gives the same bytes.
func TestExportIsByteIdenticalForAPastWindow(t *testing.T) {
	cps := []checkpoint.Checkpoint{
		resolved("cp-approved", exportFrom.Add(2*time.Minute), exportFrom.Add(3*time.Minute)),
		raised("cp-open", exportFrom.Add(9*time.Minute)),
	}
	store := &fakeExportStore{events: everyKind(), checkpoints: cps}
	first := exportBytes(t, store, exportRetention, exportFrom, exportTo, exportNow)

	t.Run("exported twice", func(t *testing.T) {
		if again := exportBytes(t, store, exportRetention, exportFrom, exportTo, exportNow); !bytes.Equal(first, again) {
			t.Errorf("second export differs from the first\nfirst:  %s\nsecond: %s", first, again)
		}
	})

	t.Run("store returns the rows in another order", func(t *testing.T) {
		events := everyKind()
		slices.Reverse(events)
		events[0], events[3] = events[3], events[0]
		shuffledCps := slices.Clone(cps)
		slices.Reverse(shuffledCps)
		shuffled := &fakeExportStore{events: events, checkpoints: shuffledCps}
		if got := exportBytes(t, shuffled, exportRetention, exportFrom, exportTo, exportNow); !bytes.Equal(first, got) {
			t.Errorf("export depends on the order the store returned rows in\nfirst:    %s\nshuffled: %s", first, got)
		}
	})

	t.Run("payload keys written in another order", func(t *testing.T) {
		events := everyKind()
		for i := range events {
			switch events[i].ID {
			case "ev-execute", "ev-escalation":
				events[i].PayloadJSON = `{"risk_class":"consequential","requires_approval_for":["software.cap.merge"],"effective_threshold":0.85,"confidence_threshold":0.9,"max_autonomous":3,"reasoning_strategy":"react","agent_definition_id":"software.agent.reviewer"}`
			case "ev-promotion":
				events[i].PayloadJSON = `{"to":"autonomous", "from":"supervised"}`
			}
		}
		reordered := &fakeExportStore{events: events, checkpoints: cps}
		if got := exportBytes(t, reordered, exportRetention, exportFrom, exportTo, exportNow); !bytes.Equal(first, got) {
			t.Errorf("export depends on payload key order\nfirst:     %s\nreordered: %s", first, got)
		}
	})

	t.Run("exported at another time", func(t *testing.T) {
		later := time.Date(2031, 11, 29, 17, 53, 47, 0, time.UTC)
		got := exportBytes(t, store, exportRetention, exportFrom, exportTo, later)
		if !bytes.Equal(first, got) {
			t.Errorf("export depends on when it was generated\nfirst: %s\nlater: %s", first, got)
		}
		for _, b := range [][]byte{first, got} {
			for _, stamp := range []string{"2026-06-17", "09:41:23", "2031-11-29", "17:53:47"} {
				if bytes.Contains(b, []byte(stamp)) {
					t.Errorf("export carries its generation time (%s): %s", stamp, b)
				}
			}
		}
	})
}

// A window that has not ended would export differently tomorrow, so it is
// refused rather than exported.
func TestExportRefusesAWindowThatIsNotClosed(t *testing.T) {
	tests := []struct {
		name     string
		from, to time.Time
		wantIn   string
	}{
		{"to after now", exportFrom, exportNow.Add(time.Second), "ended"},
		{"from equals to", exportFrom, exportFrom, ""},
		{"from after to", exportTo, exportFrom, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeExportStore{events: everyKind()}
			b, err := NewExporter(store, exportRetention, exportTemplates()).Export(context.Background(), tt.from, tt.to, exportNow)
			if !errors.Is(err, ErrWindow) {
				t.Fatalf("Export error = %v, want one wrapping ErrWindow", err)
			}
			if b != nil {
				t.Errorf("a refused window returned %d bytes, want none", len(b))
			}
			if !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("error %q does not say the window must have %s", err, tt.wantIn)
			}
			if len(store.filters) != 0 {
				t.Errorf("a refused window still read the store: %+v", store.filters)
			}
		})
	}
}

func TestExportAllowsAWindowEndingNow(t *testing.T) {
	store := &fakeExportStore{events: everyKind()}
	if _, err := NewExporter(store, exportRetention, exportTemplates()).Export(context.Background(), exportFrom, exportNow, exportNow); err != nil {
		t.Fatalf("Export with to == now: %v, want nil", err)
	}
}

func TestExportCarriesEverySection(t *testing.T) {
	store := &fakeExportStore{events: everyKind(), checkpoints: []checkpoint.Checkpoint{
		resolved("cp-approved", exportFrom.Add(2*time.Minute), exportFrom.Add(3*time.Minute)),
	}}
	doc, b := exportDoc(t, store, exportRetention, exportFrom, exportTo)

	if doc.SchemaVersion != ExportSchemaVersion {
		t.Errorf("schema_version = %d, want %d", doc.SchemaVersion, ExportSchemaVersion)
	}
	if doc.Window.From != "2026-03-01T00:00:00Z" || doc.Window.To != "2026-04-01T00:00:00Z" {
		t.Errorf("window = %+v, want the UTC RFC3339Nano bounds", doc.Window)
	}

	t.Run("top-level order", func(t *testing.T) {
		keys := []string{`"schema_version"`, `"window"`, `"retention"`, `"templates"`, `"decisions"`, `"oversight"`,
			`"autonomy_changes"`, `"counts_by_kind"`, `"pending_checkpoints"`, `"not_a_certification"`}
		last := -1
		for _, k := range keys {
			i := bytes.Index(b, []byte(k))
			if i < 0 {
				t.Fatalf("export has no %s", k)
			}
			if i < last {
				t.Errorf("%s is out of order", k)
			}
			last = i
		}
	})

	t.Run("decisions", func(t *testing.T) {
		if len(doc.Decisions.Rows) != 2 {
			t.Fatalf("decisions.rows = %d rows, want 2 (execute and escalation)", len(doc.Decisions.Rows))
		}
		for _, want := range []struct{ id, kind string }{{"ev-execute", storage.ToolEventExecute}, {"ev-escalation", storage.ToolEventEscalation}} {
			r, ok := rowByID(doc.Decisions.Rows, want.id)
			if !ok {
				t.Errorf("decisions.rows has no %s", want.id)
				continue
			}
			if r.Kind != want.kind {
				t.Errorf("%s kind = %q, want %q", want.id, r.Kind, want.kind)
			}
			if r.Provider != "anthropic" || r.Model != "claude-opus-5-5" {
				t.Errorf("%s provider, model = %q, %q", want.id, r.Provider, r.Model)
			}
			if r.AgentDefinitionID != "software.agent.reviewer" || r.ReasoningStrategy != "react" {
				t.Errorf("%s agent definition, strategy = %q, %q", want.id, r.AgentDefinitionID, r.ReasoningStrategy)
			}
			if r.TemplateID != "software.review" || r.RiskClass != "consequential" || r.AutonomyRung != "supervised" {
				t.Errorf("%s template, risk, rung = %q, %q, %q", want.id, r.TemplateID, r.RiskClass, r.AutonomyRung)
			}
			if r.CreatedAt == "" || r.ObjectiveID != "obj-1" {
				t.Errorf("%s created_at, objective = %q, %q", want.id, r.CreatedAt, r.ObjectiveID)
			}
			if r.Bounds == nil {
				t.Errorf("%s carries no bounds", want.id)
				continue
			}
			if r.Bounds.MaxAutonomous != 3 || r.Bounds.ConfidenceThreshold != 0.9 || r.Bounds.EffectiveThreshold != 0.85 ||
				!slices.Equal(r.Bounds.RequiresApprovalFor, []string{"software.cap.merge"}) {
				t.Errorf("%s bounds = %+v", want.id, *r.Bounds)
			}
		}
		if doc.Decisions.WithoutProvenance != 0 {
			t.Errorf("without_provenance = %d, want 0", doc.Decisions.WithoutProvenance)
		}
	})

	t.Run("interventions", func(t *testing.T) {
		if len(doc.Oversight.Interventions) != 3 {
			t.Fatalf("interventions = %d rows, want 3", len(doc.Oversight.Interventions))
		}
		for _, want := range []struct{ id, kind, approver, cp string }{
			{"ev-approval", storage.ToolEventApproval, "alice", "cp-approved"},
			{"ev-rejection", storage.ToolEventRejection, "bob", "cp-rejected"},
			{"ev-modification", storage.ToolEventModification, "carol", "cp-modified"},
		} {
			r, ok := rowByID(doc.Oversight.Interventions, want.id)
			if !ok {
				t.Errorf("interventions has no %s", want.id)
				continue
			}
			if r.Kind != want.kind || r.Approver != want.approver || r.CheckpointID != want.cp {
				t.Errorf("%s = kind %q approver %q checkpoint %q, want %q %q %q", want.id, r.Kind, r.Approver, r.CheckpointID, want.kind, want.approver, want.cp)
			}
		}
		mod, _ := rowByID(doc.Oversight.Interventions, "ev-modification")
		var m checkpoint.Modifications
		if err := json.Unmarshal(mod.Modifications, &m); err != nil {
			t.Fatalf("modification row does not say what was changed: %v (%s)", err, mod.Modifications)
		}
		if !slices.Equal(m.RemovedActions, []string{"software.cap.merge"}) || !slices.Equal(m.AddedConstraints, []string{"do not merge"}) {
			t.Errorf("modifications = %+v", m)
		}
	})

	t.Run("autonomy changes", func(t *testing.T) {
		if len(doc.AutonomyChanges) != 2 {
			t.Fatalf("autonomy_changes = %d rows, want 2", len(doc.AutonomyChanges))
		}
		if doc.AutonomyChanges[0].ID != "ev-promotion" || doc.AutonomyChanges[0].Kind != storage.ToolEventPromotion {
			t.Errorf("first autonomy change = %+v, want the promotion", doc.AutonomyChanges[0])
		}
		if doc.AutonomyChanges[1].ID != "ev-demotion" || doc.AutonomyChanges[1].Kind != storage.ToolEventDemotion {
			t.Errorf("second autonomy change = %+v, want the demotion", doc.AutonomyChanges[1])
		}
		if got := string(doc.AutonomyChanges[0].Payload); got != `{"from":"supervised","to":"autonomous"}` {
			t.Errorf("promotion payload = %s", got)
		}
	})

	t.Run("counts by kind", func(t *testing.T) {
		want := []ExportKindCount{
			{storage.ToolEventApproval, 1}, {storage.ToolEventAuthzDenied, 1}, {storage.ToolEventDemotion, 1},
			{storage.ToolEventEscalation, 1}, {storage.ToolEventExecute, 1}, {storage.ToolEventModification, 1},
			{storage.ToolEventPromotion, 1}, {storage.ToolEventRejection, 1},
		}
		if !slices.Equal(doc.CountsByKind, want) {
			t.Errorf("counts_by_kind = %+v, want %+v", doc.CountsByKind, want)
		}
		if !bytes.Contains(b, []byte(`"counts_by_kind":[`)) {
			t.Errorf("counts_by_kind is not a JSON array: %s", b)
		}
	})

	t.Run("templates", func(t *testing.T) {
		want := []ExportTemplate{
			{"finance.close", "finance", "high"},
			{"ops.rotate", "ops", "routine"},
			{"software.review", "software", "consequential"},
		}
		if !slices.Equal(doc.Templates, want) {
			t.Errorf("templates = %+v, want %+v", doc.Templates, want)
		}
	})

	t.Run("not a certification", func(t *testing.T) {
		n := doc.NotACertification
		if n == "" {
			t.Fatal("not_a_certification is empty")
		}
		for _, want := range []string{"record of what this deployment logged", "not a compliance certification"} {
			if !strings.Contains(n, want) {
				t.Errorf("not_a_certification %q does not say %q", n, want)
			}
		}
		for _, banned := range []string{"Article", "AI Act", "compliant"} {
			if strings.Contains(n, banned) {
				t.Errorf("not_a_certification %q names %q", n, banned)
			}
		}
	})
}

func TestExportAsksTheStoreForAClosedWindow(t *testing.T) {
	store := &fakeExportStore{events: everyKind()}
	// Bounds given in another zone are the same instants.
	zone := time.FixedZone("east", 5*60*60)
	exportBytes(t, store, exportRetention, exportFrom.In(zone), exportTo.In(zone), exportNow)

	if len(store.filters) == 0 {
		t.Fatal("the store was never asked for tool events")
	}
	for _, f := range store.filters {
		if f.CreatedAtSince == nil || !f.CreatedAtSince.Equal(exportFrom) {
			t.Errorf("CreatedAtSince = %v, want %v", f.CreatedAtSince, exportFrom)
		}
		if f.CreatedAtBefore == nil || !f.CreatedAtBefore.Equal(exportTo) {
			t.Errorf("CreatedAtBefore = %v, want %v", f.CreatedAtBefore, exportTo)
		}
		if !f.OldestFirst {
			t.Error("OldestFirst = false, want true")
		}
		if f.Limit != 0 {
			t.Errorf("Limit = %d, want 0 (an export is never a page)", f.Limit)
		}
	}
}

// "Nobody was asked" and "somebody was asked and has not answered" are
// different facts and must not read the same.
func TestExportSaysWhetherAPersonWasConsulted(t *testing.T) {
	onlyExecute := everyKind()[:1]

	t.Run("no checkpoint raised and none resolved", func(t *testing.T) {
		doc, b := exportDoc(t, &fakeExportStore{events: onlyExecute}, exportRetention, exportFrom, exportTo)
		o := doc.Oversight
		if o.PersonConsulted || o.CheckpointsRaised != 0 || o.CheckpointsResolved != 0 {
			t.Errorf("oversight = %+v, want nobody consulted and no checkpoints", o)
		}
		want := "No person was consulted in this window: no checkpoint was raised and none was resolved."
		if o.Statement != want {
			t.Errorf("statement = %q, want %q", o.Statement, want)
		}
		if !bytes.Contains(b, []byte(`"interventions":[]`)) {
			t.Errorf("empty interventions is not []: %s", b)
		}
		if !bytes.Contains(b, []byte(`"pending_checkpoints":[]`)) {
			t.Errorf("empty pending_checkpoints is not []: %s", b)
		}
	})

	t.Run("checkpoints raised but none resolved in the window", func(t *testing.T) {
		store := &fakeExportStore{events: onlyExecute, checkpoints: []checkpoint.Checkpoint{
			raised("cp-open-1", exportFrom.Add(time.Hour)),
			raised("cp-open-2", exportFrom.Add(2*time.Hour)),
		}}
		doc, b := exportDoc(t, store, exportRetention, exportFrom, exportTo)
		o := doc.Oversight
		if o.PersonConsulted {
			t.Error("person_consulted = true, want false: nothing was resolved in the window")
		}
		if o.CheckpointsRaised != 2 || o.CheckpointsResolved != 0 {
			t.Errorf("raised, resolved = %d, %d; want 2, 0", o.CheckpointsRaised, o.CheckpointsResolved)
		}
		none := "No person was consulted in this window: no checkpoint was raised and none was resolved."
		if o.Statement == "" || o.Statement == none {
			t.Errorf("statement = %q, want one that differs from the no-checkpoint case", o.Statement)
		}
		if !strings.Contains(o.Statement, "raised") || !strings.Contains(o.Statement, "none was resolved") {
			t.Errorf("statement %q does not say checkpoints were raised and none was resolved in the window", o.Statement)
		}
		if strings.Contains(o.Statement, "no checkpoint was raised") {
			t.Errorf("statement %q denies that a checkpoint was raised", o.Statement)
		}
		if got := pendingIDs(doc); !slices.Equal(got, []string{"cp-open-1", "cp-open-2"}) {
			t.Errorf("pending_checkpoints = %v, want both raised checkpoints", got)
		}
		if !bytes.Contains(b, []byte(`"interventions":[]`)) {
			t.Errorf("empty interventions is not []: %s", b)
		}
	})

	t.Run("a checkpoint resolved in the window", func(t *testing.T) {
		store := &fakeExportStore{events: everyKind()[:3], checkpoints: []checkpoint.Checkpoint{
			resolved("cp-approved", exportFrom.Add(2*time.Minute), exportFrom.Add(3*time.Minute)),
		}}
		doc, _ := exportDoc(t, store, exportRetention, exportFrom, exportTo)
		o := doc.Oversight
		if !o.PersonConsulted {
			t.Error("person_consulted = false, want true")
		}
		if o.CheckpointsRaised != 1 || o.CheckpointsResolved != 1 {
			t.Errorf("raised, resolved = %d, %d; want 1, 1", o.CheckpointsRaised, o.CheckpointsResolved)
		}
		if strings.HasPrefix(o.Statement, "No person was consulted") || o.Statement == "" {
			t.Errorf("statement = %q, want one saying a person was consulted", o.Statement)
		}
		if len(o.Interventions) != 1 || o.Interventions[0].ID != "ev-approval" {
			t.Errorf("interventions = %+v, want the approval", o.Interventions)
		}
	})

	t.Run("empty window has no nulls", func(t *testing.T) {
		b := exportBytes(t, &fakeExportStore{}, exportRetention, exportFrom, exportTo, exportNow)
		if bytes.Contains(b, []byte("null")) {
			t.Errorf("an empty window encodes a null: %s", b)
		}
		for _, list := range []string{`"rows":[]`, `"interventions":[]`, `"autonomy_changes":[]`, `"counts_by_kind":[]`, `"pending_checkpoints":[]`} {
			if !bytes.Contains(b, []byte(list)) {
				t.Errorf("an empty window does not encode %s: %s", list, b)
			}
		}
	})
}

// Pending means pending when the window closed, which is a fact about the past
// and so cannot be changed by what somebody does afterwards.
func TestExportListsCheckpointsPendingAtTheEndOfTheWindow(t *testing.T) {
	stillOpen := raised("cp-still-open", exportFrom.Add(time.Hour))
	resolvedLater := resolved("cp-resolved-later", exportFrom.Add(2*time.Hour), exportTo.Add(48*time.Hour))
	beforeWindow := raised("cp-before-window", exportFrom.Add(-time.Hour))
	resolvedInside := resolved("cp-resolved-inside", exportFrom.Add(3*time.Hour), exportFrom.Add(4*time.Hour))
	raisedAfter := raised("cp-raised-after", exportTo.Add(time.Hour))

	store := &fakeExportStore{checkpoints: []checkpoint.Checkpoint{stillOpen, resolvedLater, beforeWindow, resolvedInside, raisedAfter}}
	doc, before := exportDoc(t, store, exportRetention, exportFrom, exportTo)

	if got := pendingIDs(doc); !slices.Equal(got, []string{"cp-resolved-later", "cp-still-open"}) {
		t.Errorf("pending_checkpoints = %v, want [cp-resolved-later cp-still-open]", got)
	}
	if doc.Oversight.CheckpointsRaised != 3 || doc.Oversight.CheckpointsResolved != 1 {
		t.Errorf("raised, resolved = %d, %d; want 3, 1", doc.Oversight.CheckpointsRaised, doc.Oversight.CheckpointsResolved)
	}
	for _, p := range doc.PendingCheckpoints {
		if p.ID == "cp-still-open" && (p.ObjectiveID != "obj-1" || p.RaisedAt != "2026-03-01T01:00:00Z") {
			t.Errorf("cp-still-open = %+v, want objective obj-1 raised at 2026-03-01T01:00:00Z", p)
		}
	}
	// Only what cannot change later: no status, decision or resolution time.
	var raw struct {
		Pending []map[string]any `json:"pending_checkpoints"`
	}
	if err := json.Unmarshal(before, &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, p := range raw.Pending {
		for k := range p {
			if k != "id" && k != "objective_id" && k != "raised_at" {
				t.Errorf("pending checkpoint carries %q, which can change after the window closed", k)
			}
		}
	}

	// cp-still-open is resolved long after the window closed.
	after := &fakeExportStore{checkpoints: []checkpoint.Checkpoint{
		resolved("cp-still-open", stillOpen.CreatedAt, exportTo.Add(72*time.Hour)),
		resolvedLater, beforeWindow, resolvedInside, raisedAfter,
	}}
	if got := exportBytes(t, after, exportRetention, exportFrom, exportTo, exportNow); !bytes.Equal(before, got) {
		t.Errorf("resolving a checkpoint after the window closed changed the export\nbefore: %s\nafter:  %s", before, got)
	}
}

// Rows written before provenance was recorded are carried as they are, and
// counted, so their empty fields are not read as "no model was involved".
func TestExportCountsDecisionRowsWithoutProvenance(t *testing.T) {
	events := everyKind()[:2]
	events = append(events,
		storage.ToolEvent{ID: "ev-old-execute", ObjectiveID: "obj-0", Kind: storage.ToolEventExecute, Success: true, CreatedAt: exportFrom.Add(20 * time.Minute)},
		storage.ToolEvent{ID: "ev-old-escalation", ObjectiveID: "obj-0", Kind: storage.ToolEventEscalation, PayloadJSON: `{"reason":"low confidence"}`, CreatedAt: exportFrom.Add(21 * time.Minute)},
		// An intervention never carries provenance and is not a decision row.
		storage.ToolEvent{ID: "ev-approval", ObjectiveID: "obj-0", Kind: storage.ToolEventApproval, Approver: "alice", CreatedAt: exportFrom.Add(22 * time.Minute)},
	)
	doc, _ := exportDoc(t, &fakeExportStore{events: events}, exportRetention, exportFrom, exportTo)

	if len(doc.Decisions.Rows) != 4 {
		t.Fatalf("decisions.rows = %d rows, want 4", len(doc.Decisions.Rows))
	}
	if doc.Decisions.WithoutProvenance != 2 {
		t.Errorf("without_provenance = %d, want 2", doc.Decisions.WithoutProvenance)
	}
	for _, id := range []string{"ev-old-execute", "ev-old-escalation"} {
		r, ok := rowByID(doc.Decisions.Rows, id)
		if !ok {
			t.Errorf("decisions.rows has no %s", id)
			continue
		}
		if r.Provider != "" || r.Model != "" || r.TemplateID != "" {
			t.Errorf("%s provider, model, template = %q, %q, %q; want all empty", id, r.Provider, r.Model, r.TemplateID)
		}
	}
}

// One rule for embedded payloads: valid JSON is re-encoded with sorted keys,
// anything else is carried as the stored string.
func TestExportEmbedsPayloadsCanonically(t *testing.T) {
	at := exportFrom.Add(time.Minute)
	events := []storage.ToolEvent{
		{ID: "ev-1-json", Kind: storage.ToolEventExecute, PayloadJSON: `{ "zeta": 1, "alpha": {"b": 2, "a": [3, 1]} }`, CreatedAt: at},
		{ID: "ev-2-text", Kind: storage.ToolEventExecute, PayloadJSON: `not json {`, CreatedAt: at.Add(time.Minute)},
		{ID: "ev-3-empty", Kind: storage.ToolEventExecute, PayloadJSON: "", CreatedAt: at.Add(2 * time.Minute)},
	}
	doc, _ := exportDoc(t, &fakeExportStore{events: events}, exportRetention, exportFrom, exportTo)

	want := map[string]string{
		"ev-1-json":  `{"alpha":{"a":[3,1],"b":2},"zeta":1}`,
		"ev-2-text":  `"not json {"`,
		"ev-3-empty": `""`,
	}
	for id, w := range want {
		r, ok := rowByID(doc.Decisions.Rows, id)
		if !ok {
			t.Errorf("decisions.rows has no %s", id)
			continue
		}
		if string(r.Payload) != w {
			t.Errorf("%s payload = %s, want %s", id, r.Payload, w)
		}
	}
}

// Rows come out oldest first, and by id within one instant.
func TestExportOrdersRowsByTimeThenID(t *testing.T) {
	at := exportFrom.Add(time.Hour)
	events := []storage.ToolEvent{
		{ID: "ev-c", Kind: storage.ToolEventExecute, CreatedAt: at.Add(time.Minute)},
		{ID: "ev-b", Kind: storage.ToolEventExecute, CreatedAt: at},
		{ID: "ev-a", Kind: storage.ToolEventExecute, CreatedAt: at},
		{ID: "ev-z", Kind: storage.ToolEventExecute, CreatedAt: at.Add(-time.Minute)},
	}
	doc, _ := exportDoc(t, &fakeExportStore{events: events}, exportRetention, exportFrom, exportTo)
	got := []string{}
	for _, r := range doc.Decisions.Rows {
		got = append(got, r.ID)
	}
	if want := []string{"ev-z", "ev-a", "ev-b", "ev-c"}; !slices.Equal(got, want) {
		t.Errorf("decisions.rows order = %v, want %v", got, want)
	}
}

// The rule is from < to - retention_days: a window longer than the retention
// period reaches back past what a prune at the window's own end would have
// kept. It is measured from the window, never from now.
func TestExportRetentionSection(t *testing.T) {
	to := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	tests := []struct {
		name      string
		r         Retention
		from      time.Time
		precedes  bool
		noteHas   []string
		noteEmpty bool
	}{
		{"window reaches back past the retention period", Retention{FloorDays: 183, Days: 200}, to.Add(-201 * day), true,
			[]string{"may have been pruned", "not inactivity"}, false},
		{"window exactly the retention period", Retention{FloorDays: 183, Days: 200}, to.Add(-200 * day), false, nil, false},
		{"window inside the retention period", Retention{FloorDays: 183, Days: 200}, to.Add(-30 * day), false, nil, false},
		{"never pruned", Retention{FloorDays: 183, Days: 0}, to.Add(-900 * day), false, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeExportStore{}
			doc, first := exportDoc(t, store, tt.r, tt.from, to)
			ret := doc.Retention
			if ret.FloorDays != tt.r.FloorDays || ret.RetentionDays != tt.r.Days {
				t.Errorf("floor_days, retention_days = %d, %d; want %d, %d", ret.FloorDays, ret.RetentionDays, tt.r.FloorDays, tt.r.Days)
			}
			if ret.WindowPrecedesRetention != tt.precedes {
				t.Errorf("window_precedes_retention = %v, want %v", ret.WindowPrecedesRetention, tt.precedes)
			}
			for _, want := range tt.noteHas {
				if !strings.Contains(ret.Note, want) {
					t.Errorf("note %q does not say %q", ret.Note, want)
				}
			}
			if tt.noteEmpty && ret.Note != "" && !strings.Contains(ret.Note, "never pruned") {
				t.Errorf("note = %q, want empty or one saying the log is never pruned", ret.Note)
			}
			// The flag is a fact about the window, not about when it was asked for.
			later := exportBytes(t, store, tt.r, tt.from, to, exportNow.Add(5000*day))
			if !bytes.Equal(first, later) {
				t.Errorf("retention section depends on now\nfirst: %s\nlater: %s", first, later)
			}
		})
	}
}
