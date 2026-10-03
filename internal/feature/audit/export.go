package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// ExportSchemaVersion is the version of the export document's shape. It
// changes when a field is renamed, removed or reordered.
const ExportSchemaVersion = 1

// ErrWindow is wrapped by every refusal of an export window.
var ErrWindow = errors.New("audit export: invalid window")

// exportStore is the reads the export needs. It writes nothing: like the
// report, an export is a read that accumulates nothing.
type exportStore interface {
	ListToolEvents(ctx context.Context, f storage.ToolEventFilter) ([]storage.ToolEvent, error)
	ListPendingCheckpoints(ctx context.Context, twinID string) ([]checkpoint.Checkpoint, error)
	ListResolvedCheckpoints(ctx context.Context, f storage.ResolvedCheckpointFilter) ([]checkpoint.Checkpoint, error)
}

// Export is the document an export encodes. Field order is the byte order.
type Export struct {
	SchemaVersion      int                       `json:"schema_version"`
	Window             ExportWindow              `json:"window"`
	Retention          ExportRetention           `json:"retention"`
	Templates          []ExportTemplate          `json:"templates"`
	Decisions          ExportDecisions           `json:"decisions"`
	Oversight          ExportOversight           `json:"oversight"`
	AutonomyChanges    []ExportRow               `json:"autonomy_changes"`
	CountsByKind       []ExportKindCount         `json:"counts_by_kind"`
	PendingCheckpoints []ExportPendingCheckpoint `json:"pending_checkpoints"`
	NotACertification  string                    `json:"not_a_certification"`
}

// ExportWindow is the closed-open window [From, To), as UTC RFC3339Nano.
type ExportWindow struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type ExportRetention struct {
	FloorDays int `json:"floor_days"`
	// RetentionDays is 0 when the log is never pruned.
	RetentionDays           int    `json:"retention_days"`
	WindowPrecedesRetention bool   `json:"window_precedes_retention"`
	Note                    string `json:"note"`
}

type ExportTemplate struct {
	ID        string `json:"id"`
	Domain    string `json:"domain"`
	RiskClass string `json:"risk_class"`
}

type ExportDecisions struct {
	Rows []ExportRow `json:"rows"`
	// WithoutProvenance counts the rows with no provider, model or template.
	WithoutProvenance int `json:"without_provenance"`
}

type ExportOversight struct {
	PersonConsulted     bool        `json:"person_consulted"`
	Statement           string      `json:"statement"`
	CheckpointsRaised   int         `json:"checkpoints_raised"`
	CheckpointsResolved int         `json:"checkpoints_resolved"`
	Interventions       []ExportRow `json:"interventions"`
}

// ExportBounds is the authority a decision was taken under, lifted from the
// row's payload.
type ExportBounds struct {
	MaxAutonomous       int      `json:"max_autonomous"`
	ConfidenceThreshold float64  `json:"confidence_threshold"`
	EffectiveThreshold  float64  `json:"effective_threshold"`
	RequiresApprovalFor []string `json:"requires_approval_for"`
}

// ExportRow is one audit row. The fields after AutonomyRung are lifted from
// the payload where it carries them. Payload is the stored payload_json
// re-encoded canonically when it is valid JSON, and the stored text as a JSON
// string when it is not.
type ExportRow struct {
	ID                string          `json:"id"`
	CreatedAt         string          `json:"created_at"`
	Kind              string          `json:"kind"`
	ObjectiveID       string          `json:"objective_id"`
	AgentID           string          `json:"agent_id"`
	Capability        string          `json:"capability"`
	Adapter           string          `json:"adapter"`
	Success           bool            `json:"success"`
	Confidence        float64         `json:"confidence"`
	EscalationReason  string          `json:"escalation_reason"`
	Approver          string          `json:"approver"`
	BoundsViolation   bool            `json:"bounds_violation"`
	Provider          string          `json:"provider"`
	Model             string          `json:"model"`
	TemplateID        string          `json:"template_id"`
	AutonomyRung      string          `json:"autonomy_rung"`
	AgentDefinitionID string          `json:"agent_definition_id"`
	ReasoningStrategy string          `json:"reasoning_strategy"`
	RiskClass         string          `json:"risk_class"`
	Bounds            *ExportBounds   `json:"bounds,omitempty"`
	CheckpointID      string          `json:"checkpoint_id"`
	Modifications     json.RawMessage `json:"modifications,omitempty"`
	Payload           json.RawMessage `json:"payload"`
}

type ExportKindCount struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

// ExportPendingCheckpoint carries only what cannot change after the window
// closed, so a later resolution does not change the bytes.
type ExportPendingCheckpoint struct {
	ID          string `json:"id"`
	ObjectiveID string `json:"objective_id"`
	RaisedAt    string `json:"raised_at"`
}

// Exporter writes the audit record of a past window.
type Exporter struct {
	store     exportStore
	retention Retention
	templates []objective.Template
}

func NewExporter(store exportStore, r Retention, templates []objective.Template) *Exporter {
	return &Exporter{store: store, retention: r, templates: templates}
}

// Export returns the document for [from, to) as JSON. now is used only to
// refuse a window that has not ended; it never appears in the bytes.
//
// It is a pure read, like the report: it writes nothing and caches nothing,
// so the same past window gives the same bytes whenever it is asked for.
func (e *Exporter) Export(ctx context.Context, from, to, now time.Time) ([]byte, error) {
	from, to = from.UTC(), to.UTC()
	if to.After(now) {
		return nil, fmt.Errorf("%w: a window must have ended to be exported, because an open window cannot produce the same bytes tomorrow (to %s is after now)", ErrWindow, formatTime(to))
	}
	if !from.Before(to) {
		return nil, fmt.Errorf("%w: from %s is not before to %s", ErrWindow, formatTime(from), formatTime(to))
	}

	events, err := e.store.ListToolEvents(ctx, storage.ToolEventFilter{CreatedAtSince: &from, CreatedAtBefore: &to, OldestFirst: true})
	if err != nil {
		return nil, fmt.Errorf("audit export: list tool events: %w", err)
	}
	// Sorted again here so the order never depends on the store.
	events = slices.Clone(events)
	slices.SortFunc(events, func(a, b storage.ToolEvent) int {
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})

	doc := Export{
		SchemaVersion:      ExportSchemaVersion,
		Window:             ExportWindow{From: formatTime(from), To: formatTime(to)},
		Retention:          e.retentionSection(from, to),
		Templates:          make([]ExportTemplate, 0, len(e.templates)),
		Decisions:          ExportDecisions{Rows: []ExportRow{}},
		Oversight:          ExportOversight{Interventions: []ExportRow{}},
		AutonomyChanges:    []ExportRow{},
		CountsByKind:       []ExportKindCount{},
		PendingCheckpoints: []ExportPendingCheckpoint{},
		NotACertification:  notACertification,
	}

	for _, t := range e.templates {
		doc.Templates = append(doc.Templates, ExportTemplate{ID: t.ID, Domain: t.Domain, RiskClass: t.Risk.String()})
	}
	slices.SortFunc(doc.Templates, func(a, b ExportTemplate) int { return strings.Compare(a.ID, b.ID) })

	counts := map[string]int{}
	for _, ev := range events {
		counts[ev.Kind]++
		row := exportRow(ev)
		switch ev.Kind {
		case storage.ToolEventExecute, storage.ToolEventEscalation:
			doc.Decisions.Rows = append(doc.Decisions.Rows, row)
			if ev.Provider == "" && ev.Model == "" && ev.TemplateID == "" {
				doc.Decisions.WithoutProvenance++
			}
		case storage.ToolEventApproval, storage.ToolEventRejection, storage.ToolEventModification:
			doc.Oversight.Interventions = append(doc.Oversight.Interventions, row)
		case storage.ToolEventPromotion, storage.ToolEventDemotion:
			doc.AutonomyChanges = append(doc.AutonomyChanges, row)
		}
	}
	for kind, n := range counts {
		doc.CountsByKind = append(doc.CountsByKind, ExportKindCount{Kind: kind, Count: n})
	}
	slices.SortFunc(doc.CountsByKind, func(a, b ExportKindCount) int { return strings.Compare(a.Kind, b.Kind) })

	if err := e.checkpoints(ctx, from, to, &doc); err != nil {
		return nil, err
	}
	return json.Marshal(doc)
}

// checkpoints fills the checkpoint counts, the statement and the checkpoints
// still pending when the window closed. A checkpoint resolved at or after to
// was pending at to, whatever its status is today.
func (e *Exporter) checkpoints(ctx context.Context, from, to time.Time, doc *Export) error {
	pending, err := e.store.ListPendingCheckpoints(ctx, "")
	if err != nil {
		return fmt.Errorf("audit export: list pending checkpoints: %w", err)
	}
	// Anything raised in the window was resolved at or after from, so the
	// lower bound loses nothing; there is no upper bound because a later
	// resolution is what makes a checkpoint pending at to.
	resolved, err := e.store.ListResolvedCheckpoints(ctx, storage.ResolvedCheckpointFilter{Since: from})
	if err != nil {
		return fmt.Errorf("audit export: list resolved checkpoints: %w", err)
	}

	inWindow := func(t time.Time) bool { return !t.Before(from) && t.Before(to) }
	o := &doc.Oversight
	for _, c := range pending {
		if !inWindow(c.CreatedAt) {
			continue
		}
		o.CheckpointsRaised++
		doc.PendingCheckpoints = append(doc.PendingCheckpoints, pendingCheckpoint(c))
	}
	for _, c := range resolved {
		resolvedInside := c.ResolvedAt != nil && inWindow(*c.ResolvedAt)
		if resolvedInside {
			o.CheckpointsResolved++
		}
		if !inWindow(c.CreatedAt) {
			continue
		}
		o.CheckpointsRaised++
		if !resolvedInside {
			doc.PendingCheckpoints = append(doc.PendingCheckpoints, pendingCheckpoint(c))
		}
	}
	slices.SortFunc(doc.PendingCheckpoints, func(a, b ExportPendingCheckpoint) int {
		if c := strings.Compare(a.RaisedAt, b.RaisedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})

	o.PersonConsulted = o.CheckpointsResolved > 0 || len(o.Interventions) > 0
	switch {
	case o.PersonConsulted:
		// Says what the rows show and no more. A resolution is recorded under
		// an approver account; whether a person, a script or another agent was
		// operating that account is not something the log can know.
		o.Statement = fmt.Sprintf("Decisions were asked for and given in this window: %d checkpoints were raised, %d were resolved and %d approvals, rejections or modifications were recorded, each under the approver named on it. The record shows which account decided; it cannot show who was operating that account.",
			o.CheckpointsRaised, o.CheckpointsResolved, len(o.Interventions))
	case o.CheckpointsRaised > 0:
		o.Statement = fmt.Sprintf("A decision was asked for and not given in this window: %d checkpoints were raised and none was resolved in the window.", o.CheckpointsRaised)
	default:
		o.Statement = "No person was consulted in this window: no checkpoint was raised and none was resolved."
	}
	return nil
}

const notACertification = "This is a record of what this deployment logged in the window. It is not a compliance certification. " +
	"Whether this deployment is high-risk, and whether this record satisfies an assessor, are determinations somebody else makes."

// retentionSection says whether the window reaches back past what the
// retention period could still hold. It is measured from the window's own
// end, never from now, so it is the same whenever the export is asked for.
func (e *Exporter) retentionSection(from, to time.Time) ExportRetention {
	r := ExportRetention{FloorDays: e.retention.FloorDays, RetentionDays: e.retention.Days}
	switch {
	case e.retention.Days == 0:
		r.Note = "The audit log is never pruned."
	case from.Before(to.Add(-time.Duration(e.retention.Days) * 24 * time.Hour)):
		r.WindowPrecedesRetention = true
		r.Note = fmt.Sprintf("This window starts more than %d days before it ends, which is longer than the audit log is kept: rows from its earliest part may have been pruned, so their absence is not inactivity.", e.retention.Days)
	default:
		// Says what the retention does as well as how the window compares: a
		// short window that ended long ago can have been pruned whole, and
		// only the reader knows today's date. It cannot be worked out here
		// without making the bytes depend on when the export was asked for.
		r.Note = fmt.Sprintf("This window is no longer than the %d days the audit log is kept. Rows older than %d days are pruned, so an export asked for more than %d days after the window ended may be missing rows, and their absence is not inactivity.",
			e.retention.Days, e.retention.Days, e.retention.Days)
	}
	return r
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func pendingCheckpoint(c checkpoint.Checkpoint) ExportPendingCheckpoint {
	return ExportPendingCheckpoint{ID: c.ID, ObjectiveID: string(c.ObjectiveID), RaisedAt: formatTime(c.CreatedAt)}
}

// exportRow copies a row's columns and lifts what the loop and the checkpoint
// service wrote into its payload.
func exportRow(ev storage.ToolEvent) ExportRow {
	row := ExportRow{
		ID: ev.ID, CreatedAt: formatTime(ev.CreatedAt), Kind: ev.Kind, ObjectiveID: ev.ObjectiveID,
		AgentID: ev.AgentID, Capability: ev.Capability, Adapter: ev.Adapter, Success: ev.Success,
		Confidence: ev.Confidence, EscalationReason: ev.EscalationReason, Approver: ev.Approver,
		BoundsViolation: ev.BoundsViolation, Provider: ev.Provider, Model: ev.Model,
		TemplateID: ev.TemplateID, AutonomyRung: ev.AutonomyRung,
		Payload: canonicalJSON(ev.PayloadJSON),
	}
	var p struct {
		AgentDefinitionID   string          `json:"agent_definition_id"`
		ReasoningStrategy   string          `json:"reasoning_strategy"`
		RiskClass           string          `json:"risk_class"`
		MaxAutonomous       *int            `json:"max_autonomous"`
		ConfidenceThreshold *float64        `json:"confidence_threshold"`
		EffectiveThreshold  *float64        `json:"effective_threshold"`
		RequiresApprovalFor []string        `json:"requires_approval_for"`
		CheckpointID        string          `json:"checkpoint_id"`
		Modifications       json.RawMessage `json:"modifications"`
	}
	if json.Unmarshal([]byte(ev.PayloadJSON), &p) != nil {
		return row
	}
	row.AgentDefinitionID = p.AgentDefinitionID
	row.ReasoningStrategy = p.ReasoningStrategy
	row.RiskClass = p.RiskClass
	row.CheckpointID = p.CheckpointID
	if p.MaxAutonomous != nil || p.ConfidenceThreshold != nil || p.EffectiveThreshold != nil || p.RequiresApprovalFor != nil {
		b := ExportBounds{RequiresApprovalFor: p.RequiresApprovalFor}
		if b.RequiresApprovalFor == nil {
			b.RequiresApprovalFor = []string{}
		}
		if p.MaxAutonomous != nil {
			b.MaxAutonomous = *p.MaxAutonomous
		}
		if p.ConfidenceThreshold != nil {
			b.ConfidenceThreshold = *p.ConfidenceThreshold
		}
		if p.EffectiveThreshold != nil {
			b.EffectiveThreshold = *p.EffectiveThreshold
		}
		row.Bounds = &b
	}
	if len(p.Modifications) > 0 && string(p.Modifications) != "null" {
		row.Modifications = canonicalJSON(string(p.Modifications))
	}
	return row
}

// canonicalJSON re-encodes s with sorted object keys when it is one valid JSON
// value, and returns it as a JSON string otherwise. Numbers are kept as
// written.
func canonicalJSON(s string) json.RawMessage {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err == nil {
		if _, err := dec.Token(); errors.Is(err, io.EOF) {
			if b, err := json.Marshal(v); err == nil {
				return b
			}
		}
	}
	b, _ := json.Marshal(s)
	return b
}
