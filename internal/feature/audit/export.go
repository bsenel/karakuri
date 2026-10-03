package audit

import (
	"context"
	"encoding/json"
	"errors"
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
func (e *Exporter) Export(ctx context.Context, from, to, now time.Time) ([]byte, error) {
	return nil, errors.New("audit export: not implemented")
}
