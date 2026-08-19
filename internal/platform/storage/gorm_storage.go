package storage

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/container"
	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/memory"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/twin"
	"github.com/bsenel/karakuri/internal/core/vfs"
	"github.com/bsenel/karakuri/internal/platform/db/schema"
	"gorm.io/gorm"
)

type GORMStorage struct {
	db *gorm.DB
}

func NewGORMStorage(db *gorm.DB) *GORMStorage {
	return &GORMStorage{db: db}
}

// ── Blobs ─────────────────────────────────────────────────────────────────

func (s *GORMStorage) SaveBlob(ctx context.Context, sha string, content []byte, meta vfs.BlobMetadata) error {
	return s.db.WithContext(ctx).Save(&schema.BlobModel{
		SHA: sha, Content: content, ContentType: meta.ContentType,
		Size: meta.Size, ObjectiveID: meta.ObjectiveID, AgentID: meta.AgentID,
		Capability: meta.Capability,
	}).Error
}

func (s *GORMStorage) GetBlob(ctx context.Context, sha string) ([]byte, vfs.BlobMetadata, error) {
	var m schema.BlobModel
	if err := s.db.WithContext(ctx).First(&m, "sha = ?", sha).Error; err != nil {
		return nil, vfs.BlobMetadata{}, coreerrors.ErrNotFound
	}
	return m.Content, vfs.BlobMetadata{
		SHA: m.SHA, ContentType: m.ContentType, Size: m.Size,
		ObjectiveID: m.ObjectiveID, AgentID: m.AgentID, Capability: m.Capability,
		CreatedAt: m.CreatedAt,
	}, nil
}

func (s *GORMStorage) ListBlobs(ctx context.Context, objectiveID, agentID string) ([]vfs.BlobMetadata, error) {
	var models []schema.BlobModel
	q := s.db.WithContext(ctx).Order("created_at DESC")
	if objectiveID != "" {
		q = q.Where("objective_id = ?", objectiveID)
	}
	if agentID != "" {
		q = q.Where("agent_id = ?", agentID)
	}
	if err := q.Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]vfs.BlobMetadata, len(models))
	for i, m := range models {
		out[i] = vfs.BlobMetadata{
			SHA: m.SHA, ContentType: m.ContentType, Size: m.Size,
			ObjectiveID: m.ObjectiveID, AgentID: m.AgentID, Capability: m.Capability,
			CreatedAt: m.CreatedAt,
		}
	}
	return out, nil
}

// ── Twins ─────────────────────────────────────────────────────────────────

func (s *GORMStorage) SaveTwin(ctx context.Context, t twin.DigitalTwin) error {
	agentsJ, _ := json.Marshal(t.Agents)
	envsJ, _ := json.Marshal(t.Environments)
	objsJ, _ := json.Marshal(t.Objectives)
	memJ, _ := json.Marshal(t.Memory)
	childJ, _ := json.Marshal(t.Children)
	bindingsJ, _ := json.Marshal(t.AdapterBindings)
	if len(bindingsJ) == 0 || string(bindingsJ) == "null" {
		bindingsJ = []byte("{}")
	}
	return s.db.WithContext(ctx).Save(&schema.TwinModel{
		ID: t.ID, Name: t.Name, Kind: string(t.Kind), Domain: t.Domain,
		AgentsJSON: string(agentsJ), EnvsJSON: string(envsJ),
		ObjectivesJSON: string(objsJ), MemoryJSON: string(memJ),
		ChildrenJSON: string(childJ), AdapterBindingsJSON: string(bindingsJ),
		OwnerID: t.OwnerID,
	}).Error
}

func (s *GORMStorage) GetTwin(ctx context.Context, id string) (twin.DigitalTwin, error) {
	var m schema.TwinModel
	if err := s.db.WithContext(ctx).First(&m, "id = ?", id).Error; err != nil {
		return twin.DigitalTwin{}, coreerrors.ErrTwinNotFound
	}
	return twinFromModel(m), nil
}

func (s *GORMStorage) ListTwins(ctx context.Context, f TwinFilter) ([]twin.DigitalTwin, error) {
	var models []schema.TwinModel
	q := s.db.WithContext(ctx).Order("created_at DESC")
	if f.Kind != "" {
		q = q.Where("kind = ?", f.Kind)
	}
	if f.Domain != "" {
		q = q.Where("domain = ?", f.Domain)
	}
	q = applyScopeSelectors(q, "twin", "id", f.Visible, f.Hidden)
	if f.Limit > 0 {
		q = q.Limit(f.Limit).Offset(f.Offset)
	}
	if err := q.Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]twin.DigitalTwin, len(models))
	for i, m := range models {
		out[i] = twinFromModel(m)
	}
	return out, nil
}

func (s *GORMStorage) UpdateTwin(ctx context.Context, t twin.DigitalTwin) error {
	return s.SaveTwin(ctx, t)
}

func twinFromModel(m schema.TwinModel) twin.DigitalTwin {
	var agents []agent.Definition
	var envs []string
	var objs []string
	var mem agent.MemoryConfig
	var children []string
	var bindings map[string]string
	_ = json.Unmarshal([]byte(m.AgentsJSON), &agents)
	_ = json.Unmarshal([]byte(m.EnvsJSON), &envs)
	_ = json.Unmarshal([]byte(m.ObjectivesJSON), &objs)
	_ = json.Unmarshal([]byte(m.MemoryJSON), &mem)
	_ = json.Unmarshal([]byte(m.ChildrenJSON), &children)
	_ = json.Unmarshal([]byte(m.AdapterBindingsJSON), &bindings)

	return twin.DigitalTwin{
		ID: m.ID, Name: m.Name, Kind: twin.Kind(m.Kind), Domain: m.Domain,
		Agents: agents, Children: children, Memory: mem,
		AdapterBindings: bindings,
		OwnerID:         m.OwnerID,
		CreatedAt:       m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// ── Objectives ────────────────────────────────────────────────────────────

func (s *GORMStorage) SaveObjective(ctx context.Context, o objective.Objective) error {
	critJ, _ := json.Marshal(o.SuccessCriteria)
	constrJ, _ := json.Marshal(o.Constraints)
	addJ, _ := json.Marshal(o.AdditionalDomains)
	var parentID *string
	if o.ParentID != nil {
		pid := string(*o.ParentID)
		parentID = &pid
	}
	// Nil cadence and nil autonomy store as the empty string rather than as
	// "null", so the round trip below can tell "never declared" from
	// "declared and empty" without a nullable column.
	var cadenceJ, autonomyJ []byte
	if o.Cadence != nil {
		cadenceJ, _ = json.Marshal(o.Cadence)
	}
	if o.Autonomy != nil {
		autonomyJ, _ = json.Marshal(o.Autonomy)
	}
	return s.db.WithContext(ctx).Save(&schema.ObjectiveModel{
		ID: string(o.ID), Title: o.Title, Description: o.Description, Domain: o.Domain,
		AdditionalDomainsJSON: string(addJ),
		TwinID:                o.TwinID, Priority: o.Priority, MaxIterations: o.MaxIterations, Deadline: o.Deadline,
		CriteriaJSON: string(critJ), ConstraintsJSON: string(constrJ), ParentID: parentID,
		Status: string(o.Status),
		Mode:   string(o.Mode), CadenceJSON: string(cadenceJ), AutonomyJSON: string(autonomyJ),
	}).Error
}

func (s *GORMStorage) GetObjective(ctx context.Context, id objective.ObjectiveID) (objective.Objective, error) {
	var m schema.ObjectiveModel
	if err := s.db.WithContext(ctx).First(&m, "id = ?", string(id)).Error; err != nil {
		return objective.Objective{}, coreerrors.ErrObjectiveNotFound
	}
	return objectiveFromModel(m), nil
}

func (s *GORMStorage) ListObjectives(ctx context.Context, f ObjectiveFilter) ([]objective.Objective, error) {
	var models []schema.ObjectiveModel
	q := s.db.WithContext(ctx).Order("created_at DESC")
	if f.TwinID != "" {
		q = q.Where("twin_id = ?", f.TwinID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Mode != "" {
		q = q.Where("mode = ?", f.Mode)
	}
	q = applyScopeSelectors(q, "objective", "id", f.Visible, f.Hidden)
	if err := q.Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]objective.Objective, len(models))
	for i, m := range models {
		out[i] = objectiveFromModel(m)
	}
	return out, nil
}

func (s *GORMStorage) UpdateObjectiveStatus(ctx context.Context, id objective.ObjectiveID, st objective.ObjectiveStatus) error {
	return s.db.WithContext(ctx).Model(&schema.ObjectiveModel{}).
		Where("id = ?", string(id)).Update("status", string(st)).Error
}

func objectiveFromModel(m schema.ObjectiveModel) objective.Objective {
	var criteria []objective.Criterion
	var constraints []objective.Constraint
	var additionalDomains []string
	_ = json.Unmarshal([]byte(m.CriteriaJSON), &criteria)
	_ = json.Unmarshal([]byte(m.ConstraintsJSON), &constraints)
	if m.AdditionalDomainsJSON != "" {
		_ = json.Unmarshal([]byte(m.AdditionalDomainsJSON), &additionalDomains)
	}
	var parentID *objective.ObjectiveID
	if m.ParentID != nil {
		pid := objective.ObjectiveID(*m.ParentID)
		parentID = &pid
	}
	var cadence *objective.Cadence
	if m.CadenceJSON != "" {
		var c objective.Cadence
		if err := json.Unmarshal([]byte(m.CadenceJSON), &c); err == nil {
			cadence = &c
		}
	}
	var autonomy *objective.Autonomy
	if m.AutonomyJSON != "" {
		var a objective.Autonomy
		if err := json.Unmarshal([]byte(m.AutonomyJSON), &a); err == nil {
			autonomy = &a
		}
	}
	return objective.Objective{
		ID: objective.ObjectiveID(m.ID), Title: m.Title, Description: m.Description,
		Domain: m.Domain, AdditionalDomains: additionalDomains,
		TwinID: m.TwinID, Priority: m.Priority, MaxIterations: m.MaxIterations, Deadline: m.Deadline,
		SuccessCriteria: criteria, Constraints: constraints, ParentID: parentID,
		Status:    objective.ObjectiveStatus(m.Status),
		Mode:      objective.Mode(m.Mode),
		Cadence:   cadence,
		Autonomy:  autonomy,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// ── Loop iterations ───────────────────────────────────────────────────────

func (s *GORMStorage) SaveLoopIteration(ctx context.Context, it LoopIteration) error {
	return s.db.WithContext(ctx).Save(&schema.LoopIterationModel{
		ID: it.ID, ObjectiveID: it.ObjectiveID, Number: it.Number, Step: it.Step,
		InputJSON: it.InputJSON, OutputJSON: it.OutputJSON,
		TokensUsed: it.TokensUsed, DurationMS: it.DurationMS,
	}).Error
}

func (s *GORMStorage) ListLoopIterations(ctx context.Context, objectiveID objective.ObjectiveID) ([]LoopIteration, error) {
	var models []schema.LoopIterationModel
	if err := s.db.WithContext(ctx).
		Where("objective_id = ?", string(objectiveID)).
		Order("number ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]LoopIteration, len(models))
	for i, m := range models {
		out[i] = LoopIteration{ID: m.ID, ObjectiveID: m.ObjectiveID, Number: m.Number, Step: m.Step,
			InputJSON: m.InputJSON, OutputJSON: m.OutputJSON,
			TokensUsed: m.TokensUsed, DurationMS: m.DurationMS, CreatedAt: m.CreatedAt}
	}
	return out, nil
}

// ── Episodic memory ───────────────────────────────────────────────────────

func (s *GORMStorage) SaveMemoryEpisodic(ctx context.Context, e memory.Entry) error {
	srcJ, _ := json.Marshal(e.Sources)
	return s.db.WithContext(ctx).Save(&schema.MemoryEpisodicModel{
		ID: e.ID, AgentID: string(e.AgentID), TwinID: e.TwinID, Domain: e.Domain,
		Content: e.Content, Confidence: e.Confidence, SourcesJSON: string(srcJ),
		ExpiresAt: e.ExpiresAt,
	}).Error
}

func (s *GORMStorage) QueryEpisodic(ctx context.Context, q memory.Query) ([]memory.Entry, error) {
	var models []schema.MemoryEpisodicModel
	db := s.db.WithContext(ctx)
	if q.AgentID != "" {
		db = db.Where("agent_id = ?", string(q.AgentID))
	}
	if q.TwinID != "" {
		db = db.Where("twin_id = ?", q.TwinID)
	}
	if q.Since != nil {
		db = db.Where("created_at >= ?", q.Since)
	}
	if q.TopK > 0 {
		db = db.Limit(q.TopK)
	}
	db = db.Order("created_at DESC")
	if err := db.Find(&models).Error; err != nil {
		return nil, err
	}
	return episodicModelsToEntries(models), nil
}

func episodicModelsToEntries(models []schema.MemoryEpisodicModel) []memory.Entry {
	out := make([]memory.Entry, len(models))
	for i, m := range models {
		var sources []string
		_ = json.Unmarshal([]byte(m.SourcesJSON), &sources)
		out[i] = memory.Entry{
			ID: m.ID, AgentID: agent.AgentID(m.AgentID), TwinID: m.TwinID, Tier: string(memory.TierEpisodic),
			Domain: m.Domain, Content: m.Content, Confidence: m.Confidence, Sources: sources,
			CreatedAt: m.CreatedAt, ExpiresAt: m.ExpiresAt,
		}
	}
	return out
}

func (s *GORMStorage) DeleteMemoryEntry(ctx context.Context, id string) error {
	s.db.WithContext(ctx).Delete(&schema.MemoryEpisodicModel{}, "id = ?", id)
	s.db.WithContext(ctx).Delete(&schema.MemorySemanticModel{}, "id = ?", id)
	return nil
}

// ── Semantic memory ───────────────────────────────────────────────────────

func (s *GORMStorage) SaveMemorySemantic(ctx context.Context, e memory.Entry) error {
	srcJ, _ := json.Marshal(e.Sources)
	var embBytes []byte
	if len(e.Embedding) > 0 {
		embBytes = float32SliceToBytes(e.Embedding)
	}
	return s.db.WithContext(ctx).Save(&schema.MemorySemanticModel{
		ID: e.ID, AgentID: string(e.AgentID), TwinID: e.TwinID, Domain: e.Domain,
		Content: e.Content, Embedding: embBytes, Confidence: e.Confidence,
		SourcesJSON: string(srcJ), ExpiresAt: e.ExpiresAt,
	}).Error
}

func (s *GORMStorage) QuerySemantic(ctx context.Context, q memory.Query) ([]memory.Entry, error) {
	// Keyword-based fallback until sqlite-vec is wired
	var models []schema.MemorySemanticModel
	db := s.db.WithContext(ctx)
	if q.AgentID != "" {
		db = db.Where("agent_id = ?", string(q.AgentID))
	}
	if q.TwinID != "" {
		db = db.Where("twin_id = ?", q.TwinID)
	}
	if q.TopK > 0 {
		db = db.Limit(q.TopK)
	}
	db = db.Order("created_at DESC")
	if err := db.Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]memory.Entry, len(models))
	for i, m := range models {
		var sources []string
		_ = json.Unmarshal([]byte(m.SourcesJSON), &sources)
		out[i] = memory.Entry{
			ID: m.ID, AgentID: agent.AgentID(m.AgentID), TwinID: m.TwinID, Tier: string(memory.TierSemantic),
			Domain: m.Domain, Content: m.Content, Confidence: m.Confidence, Sources: sources,
			CreatedAt: m.CreatedAt, ExpiresAt: m.ExpiresAt,
		}
	}
	return out, nil
}

// ── Procedural memory ─────────────────────────────────────────────────────

func (s *GORMStorage) UpsertProcedural(ctx context.Context, r ProceduralRecord) error {
	return s.db.WithContext(ctx).Save(&schema.MemoryProceduralModel{
		ID: r.ID, AgentID: r.AgentID, TwinID: r.TwinID, CapabilityID: r.CapabilityID,
		SuccessCount: r.SuccessCount, FailureCount: r.FailureCount, AvgConfidence: r.AvgConfidence,
	}).Error
}

func (s *GORMStorage) QueryProcedural(ctx context.Context, agentID, capabilityID string) (ProceduralRecord, error) {
	var m schema.MemoryProceduralModel
	if err := s.db.WithContext(ctx).
		Where("agent_id = ? AND capability_id = ?", agentID, capabilityID).
		First(&m).Error; err != nil {
		return ProceduralRecord{}, coreerrors.ErrNotFound
	}
	return ProceduralRecord{
		ID: m.ID, AgentID: m.AgentID, TwinID: m.TwinID, CapabilityID: m.CapabilityID,
		SuccessCount: m.SuccessCount, FailureCount: m.FailureCount, AvgConfidence: m.AvgConfidence,
		UpdatedAt: m.UpdatedAt,
	}, nil
}

// ── Checkpoints ───────────────────────────────────────────────────────────

func (s *GORMStorage) SaveCheckpoint(ctx context.Context, c checkpoint.Checkpoint) error {
	optsJ, _ := json.Marshal(c.Options)
	actsJ, _ := json.Marshal(c.Actions)
	var decJ string
	if c.Decision != nil {
		b, _ := json.Marshal(c.Decision)
		decJ = string(b)
	}
	return s.db.WithContext(ctx).Save(&schema.CheckpointModel{
		ID: c.ID, ObjectiveID: string(c.ObjectiveID), TwinID: c.TwinID,
		Reason: c.Reason, Summary: c.Summary, OptionsJSON: string(optsJ),
		Capability:   string(c.Capability),
		Confidence:   c.Confidence,
		ActionsJSON:  string(actsJ),
		AuditEventID: c.AuditEventID,
		Status:       string(c.Status), DecisionJSON: decJ, ResolvedAt: c.ResolvedAt,
		// Passed through rather than always stamped. GORM's autoCreateTime
		// fills a zero value, so a caller that does not care still gets now —
		// but one that does (a backfill, a test fabricating history, a digest
		// asking how long a decision has been waiting) is no longer silently
		// overruled by the storage layer.
		CreatedAt: c.CreatedAt,
	}).Error
}

func (s *GORMStorage) GetCheckpoint(ctx context.Context, id string) (checkpoint.Checkpoint, error) {
	var m schema.CheckpointModel
	if err := s.db.WithContext(ctx).First(&m, "id = ?", id).Error; err != nil {
		return checkpoint.Checkpoint{}, coreerrors.ErrCheckpointNotFound
	}
	return checkpointFromModel(m), nil
}

func (s *GORMStorage) ResolveCheckpoint(ctx context.Context, id string, d checkpoint.Decision) error {
	decJ, _ := json.Marshal(d)
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&schema.CheckpointModel{}).Where("id = ?", id).Updates(map[string]any{
		"status": string(checkpoint.StatusResolved), "decision_json": string(decJ), "resolved_at": now,
	}).Error
}

func (s *GORMStorage) ListPendingCheckpoints(ctx context.Context, twinID string) ([]checkpoint.Checkpoint, error) {
	var models []schema.CheckpointModel
	q := s.db.WithContext(ctx).Where("status = ?", string(checkpoint.StatusPending))
	if twinID != "" {
		q = q.Where("twin_id = ?", twinID)
	}
	if err := q.Order("created_at ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]checkpoint.Checkpoint, len(models))
	for i, m := range models {
		out[i] = checkpointFromModel(m)
	}
	return out, nil
}

func checkpointFromModel(m schema.CheckpointModel) checkpoint.Checkpoint {
	var opts []string
	_ = json.Unmarshal([]byte(m.OptionsJSON), &opts)
	var acts []checkpoint.Action
	if m.ActionsJSON != "" {
		_ = json.Unmarshal([]byte(m.ActionsJSON), &acts)
	}
	var dec *checkpoint.Decision
	if m.DecisionJSON != "" {
		var d checkpoint.Decision
		_ = json.Unmarshal([]byte(m.DecisionJSON), &d)
		dec = &d
	}
	return checkpoint.Checkpoint{
		ID: m.ID, ObjectiveID: objective.ObjectiveID(m.ObjectiveID), TwinID: m.TwinID,
		Reason: m.Reason, Summary: m.Summary, Options: opts,
		Capability:   capability.CapabilityID(m.Capability),
		Confidence:   m.Confidence,
		Actions:      acts,
		AuditEventID: m.AuditEventID,
		Status:       checkpoint.Status(m.Status), Decision: dec,
		CreatedAt: m.CreatedAt, ResolvedAt: m.ResolvedAt,
	}
}

// ── Worktrees ─────────────────────────────────────────────────────────────

func (s *GORMStorage) SaveWorktree(ctx context.Context, w Worktree) error {
	return s.db.WithContext(ctx).Save(&schema.WorktreeModel{
		TaskID: w.TaskID, ObjectiveID: w.ObjectiveID, Path: w.Path, Branch: w.Branch,
	}).Error
}

func (s *GORMStorage) GetWorktree(ctx context.Context, taskID string) (Worktree, error) {
	var m schema.WorktreeModel
	if err := s.db.WithContext(ctx).First(&m, "task_id = ?", taskID).Error; err != nil {
		return Worktree{}, coreerrors.ErrNotFound
	}
	return Worktree{TaskID: m.TaskID, ObjectiveID: m.ObjectiveID, Path: m.Path, Branch: m.Branch, CreatedAt: m.CreatedAt}, nil
}

func (s *GORMStorage) ListWorktrees(ctx context.Context, objectiveID objective.ObjectiveID) ([]Worktree, error) {
	var models []schema.WorktreeModel
	if err := s.db.WithContext(ctx).Where("objective_id = ?", string(objectiveID)).Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]Worktree, len(models))
	for i, m := range models {
		out[i] = Worktree{TaskID: m.TaskID, ObjectiveID: m.ObjectiveID, Path: m.Path, Branch: m.Branch, CreatedAt: m.CreatedAt}
	}
	return out, nil
}

func (s *GORMStorage) DeleteWorktree(ctx context.Context, taskID string) error {
	return s.db.WithContext(ctx).Delete(&schema.WorktreeModel{}, "task_id = ?", taskID).Error
}

// ── Tool events ───────────────────────────────────────────────────────────

func (s *GORMStorage) SaveToolEvent(ctx context.Context, e ToolEvent) error {
	kind := e.Kind
	if kind == "" {
		kind = ToolEventExecute
	}
	return s.db.WithContext(ctx).Save(&schema.ToolEventModel{
		ID: e.ID, ObjectiveID: e.ObjectiveID, AgentID: e.AgentID, Capability: e.Capability,
		Adapter: e.Adapter, Success: e.Success, Confidence: e.Confidence, PayloadJSON: e.PayloadJSON,
		Kind:             kind,
		EscalationReason: e.EscalationReason,
		Approver:         e.Approver,
		BoundsViolation:  e.BoundsViolation,
	}).Error
}

func (s *GORMStorage) ListToolEvents(ctx context.Context, f ToolEventFilter) ([]ToolEvent, error) {
	q := s.db.WithContext(ctx).Order("created_at DESC")
	if f.ObjectiveID != "" {
		q = q.Where("objective_id = ?", f.ObjectiveID)
	}
	if f.ObjectiveIDs != nil {
		if len(f.ObjectiveIDs) == 0 {
			// The caller's scope covers no objectives. Returning everything
			// here is the leak this filter exists to prevent.
			return []ToolEvent{}, nil
		}
		q = q.Where("objective_id IN ?", f.ObjectiveIDs)
	}
	if f.AgentID != "" {
		q = q.Where("agent_id = ?", f.AgentID)
	}
	if f.Kind != "" {
		q = q.Where("kind = ?", f.Kind)
	}
	if f.BoundsViolation != nil {
		q = q.Where("bounds_violation = ?", *f.BoundsViolation)
	}
	if f.CreatedAtSince != nil {
		q = q.Where("created_at >= ?", *f.CreatedAtSince)
	}
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	var models []schema.ToolEventModel
	if err := q.Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]ToolEvent, len(models))
	for i, m := range models {
		out[i] = ToolEvent{
			ID: m.ID, ObjectiveID: m.ObjectiveID, AgentID: m.AgentID, Capability: m.Capability,
			Adapter: m.Adapter, Success: m.Success, Confidence: m.Confidence, PayloadJSON: m.PayloadJSON,
			Kind: m.Kind, EscalationReason: m.EscalationReason, Approver: m.Approver,
			BoundsViolation: m.BoundsViolation, CreatedAt: m.CreatedAt,
		}
	}
	return out, nil
}

// GetToolEvent returns one audit row by ID.
//
// The list endpoint could have answered this with a filter, and deliberately
// does not: an audit detail view is reached by link — from a checkpoint, from
// another tab, from somebody's bookmark — and making that a search over a
// bounded page means a row that has scrolled off cannot be opened at all.
func (s *GORMStorage) GetToolEvent(ctx context.Context, id string) (ToolEvent, error) {
	var m schema.ToolEventModel
	if err := s.db.WithContext(ctx).First(&m, "id = ?", id).Error; err != nil {
		return ToolEvent{}, err
	}
	return ToolEvent{
		ID: m.ID, ObjectiveID: m.ObjectiveID, AgentID: m.AgentID, Capability: m.Capability,
		Adapter: m.Adapter, Success: m.Success, Confidence: m.Confidence, PayloadJSON: m.PayloadJSON,
		Kind: m.Kind, EscalationReason: m.EscalationReason, Approver: m.Approver,
		BoundsViolation: m.BoundsViolation, CreatedAt: m.CreatedAt,
	}, nil
}

// ── Loop state (Phase 11) ─────────────────────────────────────────────────

func (s *GORMStorage) SaveLoopState(ctx context.Context, st coreloop.State) error {
	m := schema.LoopStateModel{
		LoopID:       st.LoopID,
		ObjectiveID:  string(st.ObjectiveID),
		TwinID:       st.TwinID,
		AgentID:      st.AgentID,
		Iteration:    st.Iteration,
		Paused:       st.Paused,
		Completed:    st.Completed,
		LastStep:     string(st.LastStep),
		Status:       string(st.Status),
		CriteriaMet:  st.CriteriaMet,
		CheckpointID: st.CheckpointID,
		RequestJSON:  st.RequestJSON,
	}
	if m.RequestJSON == "" {
		m.RequestJSON = "{}"
	}
	return s.db.WithContext(ctx).Save(&m).Error
}

func (s *GORMStorage) GetLoopState(ctx context.Context, loopID string) (coreloop.State, error) {
	var m schema.LoopStateModel
	if err := s.db.WithContext(ctx).First(&m, "loop_id = ?", loopID).Error; err != nil {
		return coreloop.State{}, err
	}
	return loopStateFromModel(m), nil
}

func (s *GORMStorage) ListActiveLoopStates(ctx context.Context) ([]coreloop.State, error) {
	var models []schema.LoopStateModel
	if err := s.db.WithContext(ctx).Where("completed = ?", false).Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]coreloop.State, len(models))
	for i, m := range models {
		out[i] = loopStateFromModel(m)
	}
	return out, nil
}

func (s *GORMStorage) DeleteLoopState(ctx context.Context, loopID string) error {
	return s.db.WithContext(ctx).Delete(&schema.LoopStateModel{}, "loop_id = ?", loopID).Error
}

func loopStateFromModel(m schema.LoopStateModel) coreloop.State {
	return coreloop.State{
		LoopID:       m.LoopID,
		ObjectiveID:  objective.ObjectiveID(m.ObjectiveID),
		TwinID:       m.TwinID,
		AgentID:      m.AgentID,
		Iteration:    m.Iteration,
		Paused:       m.Paused,
		Completed:    m.Completed,
		LastStep:     coreloop.Step(m.LastStep),
		Status:       objective.ObjectiveStatus(m.Status),
		CriteriaMet:  m.CriteriaMet,
		CheckpointID: m.CheckpointID,
		RequestJSON:  m.RequestJSON,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

// ── Scoped listing ────────────────────────────────────────────────────────

// applyScopeSelectors narrows a listing to what a principal may see.
//
// This is the payoff of materialising the closure. Visibility is an indexed
// `IN` over resource_scopes.label, which is one join — where a path-shaped
// hierarchy would need `LIKE 'org:acme/%'`, unindexable and unable to express
// "these three teams", and a relationship-graph model would walk the graph,
// which is why OpenFGA caps ListObjects at a thousand results.
//
// A nil visible means no restriction. It cannot mean "empty selector matches
// everything": a principal with no grants must see nothing, and that difference
// is the whole security property of this function.
func applyScopeSelectors(q *gorm.DB, resourceType, idColumn string, visible *ScopeSelector, hidden ScopeSelector) *gorm.DB {
	if visible != nil {
		if visible.Empty() {
			// Nothing is granted, so nothing matches. Expressed as a false
			// predicate rather than an early return so the caller's paging and
			// ordering still apply to an empty result.
			return q.Where("1 = 0")
		}
		cond := q.Session(&gorm.Session{NewDB: true})
		var group *gorm.DB
		for i, term := range scopeTerms(cond, resourceType, idColumn, *visible, false) {
			if i == 0 {
				group = cond.Where(term.query, term.args...)
				continue
			}
			group = group.Or(term.query, term.args...)
		}
		q = q.Where(group)
	}
	// Deny is applied as one NOT IN per term rather than as a negated OR group:
	// "not (a or b)" is "not a and not b", and the second form is what a
	// database plans well and what every driver renders the same way.
	cond := q.Session(&gorm.Session{NewDB: true})
	for _, term := range scopeTerms(cond, resourceType, idColumn, hidden, true) {
		q = q.Where(term.query, term.args...)
	}
	return q
}

type scopeTerm struct {
	query string
	args  []any
}

// scopeTerms renders a selector as one predicate per shape it can match:
// the row named directly, or the row sitting in one of these containers.
func scopeTerms(cond *gorm.DB, resourceType, idColumn string, sel ScopeSelector, negate bool) []scopeTerm {
	in := " IN "
	if negate {
		in = " NOT IN "
	}
	members := func(where string, args ...any) *gorm.DB {
		return cond.Table("resource_scopes").Select("resource_id").Where(where, args...)
	}

	var out []scopeTerm
	if len(sel.IDs) > 0 {
		out = append(out, scopeTerm{idColumn + in + "?", []any{sel.IDs}})
	}
	if len(sel.Labels) > 0 {
		out = append(out, scopeTerm{idColumn + in + "(?)", []any{
			members("resource_type = ? AND label IN ?", resourceType, sel.Labels),
		}})
	}
	for _, prefix := range sel.LabelPrefixes {
		out = append(out, scopeTerm{idColumn + in + "(?)", []any{
			members("resource_type = ? AND label LIKE ?", resourceType, prefix+"%"),
		}})
	}
	return out
}

// ── Containers and scopes ─────────────────────────────────────────────────

func (s *GORMStorage) SaveContainer(ctx context.Context, c container.Container) error {
	return s.db.WithContext(ctx).Save(&schema.ContainerModel{
		ID: c.ID, Kind: string(c.Kind), Name: c.Name, ParentID: c.ParentID,
	}).Error
}

func (s *GORMStorage) GetContainer(ctx context.Context, id string) (container.Container, error) {
	var m schema.ContainerModel
	if err := s.db.WithContext(ctx).First(&m, "id = ?", id).Error; err != nil {
		return container.Container{}, coreerrors.ErrContainerNotFound
	}
	return containerFromModel(m), nil
}

func (s *GORMStorage) ListContainers(ctx context.Context, f container.Filter) ([]container.Container, error) {
	var models []schema.ContainerModel
	q := s.db.WithContext(ctx).Order("name ASC")
	if f.Kind != "" {
		q = q.Where("kind = ?", string(f.Kind))
	}
	// RootsOnly is separate from ParentID because an empty ParentID has to mean
	// "do not filter" — otherwise every unfiltered listing would silently
	// narrow to the roots.
	switch {
	case f.RootsOnly:
		q = q.Where("parent_id = ?", "")
	case f.ParentID != "":
		q = q.Where("parent_id = ?", f.ParentID)
	}
	if f.Name != "" {
		q = q.Where("name = ?", f.Name)
	}
	if err := q.Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]container.Container, len(models))
	for i, m := range models {
		out[i] = containerFromModel(m)
	}
	return out, nil
}

func (s *GORMStorage) DeleteContainer(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Delete(&schema.ContainerModel{}, "id = ?", id).Error
}

func containerFromModel(m schema.ContainerModel) container.Container {
	return container.Container{
		ID: m.ID, Kind: container.Kind(m.Kind), Name: m.Name, ParentID: m.ParentID,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// PutResourceScopes replaces every label a resource carries.
//
// Replace rather than merge, in one transaction: the closure is derived state,
// and a partial write leaves a resource visible under a container it has left.
// Deleting first is what makes reparenting safe — a label that is no longer in
// the closure has to disappear, which an upsert would never do.
func (s *GORMStorage) PutResourceScopes(ctx context.Context, scopes container.ResourceScopes) error {
	direct := container.NormalizeLabels(scopes.Direct)
	all := container.NormalizeLabels(scopes.All)
	isDirect := make(map[string]bool, len(direct))
	for _, label := range direct {
		isDirect[label] = true
	}

	rows := make([]schema.ResourceScopeModel, 0, len(all))
	for _, label := range all {
		rows = append(rows, schema.ResourceScopeModel{
			ResourceType: scopes.ResourceType,
			ResourceID:   scopes.ResourceID,
			Label:        label,
			Direct:       isDirect[label],
		})
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&schema.ResourceScopeModel{},
			"resource_type = ? AND resource_id = ?", scopes.ResourceType, scopes.ResourceID).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
}

func (s *GORMStorage) GetResourceScopes(ctx context.Context, resourceType, resourceID string) (container.ResourceScopes, error) {
	var models []schema.ResourceScopeModel
	err := s.db.WithContext(ctx).Order("label ASC").
		Find(&models, "resource_type = ? AND resource_id = ?", resourceType, resourceID).Error
	if err != nil {
		return container.ResourceScopes{}, err
	}
	// A resource in no container is not an error — it is every resource that
	// existed before Phase 17, and it carries no labels.
	out := container.ResourceScopes{ResourceType: resourceType, ResourceID: resourceID}
	for _, m := range models {
		out.All = append(out.All, m.Label)
		if m.Direct {
			out.Direct = append(out.Direct, m.Label)
		}
	}
	return out, nil
}

func (s *GORMStorage) ListScopedResources(ctx context.Context, f container.ScopeFilter) ([]container.ResourceScopes, error) {
	labels := container.NormalizeLabels(f.Labels)
	if len(labels) == 0 {
		// Empty matches nothing rather than everything. The callers are
		// authorization filters, and one that widens to "every row" when its
		// input is empty is how a listing leaks.
		return nil, nil
	}

	// Two steps rather than one: the label filter selects which resources
	// match, then every label of those resources is read back. Doing it in one
	// query would return only the labels that matched, which is not what a
	// caller rebuilding a closure needs.
	type key struct {
		ResourceType string `gorm:"column:resource_type"`
		ResourceID   string `gorm:"column:resource_id"`
	}
	inner := s.db.WithContext(ctx).Model(&schema.ResourceScopeModel{}).
		Select("resource_type", "resource_id").
		Where("label IN ?", labels)
	if f.ResourceType != "" {
		inner = inner.Where("resource_type = ?", f.ResourceType)
	}
	if f.DirectOnly {
		inner = inner.Where("direct = ?", true)
	}
	var keys []key
	if err := inner.Distinct().Scan(&keys).Error; err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}

	out := make([]container.ResourceScopes, 0, len(keys))
	for _, k := range keys {
		scopes, err := s.GetResourceScopes(ctx, k.ResourceType, k.ResourceID)
		if err != nil {
			return nil, err
		}
		out = append(out, scopes)
	}
	return out, nil
}

func (s *GORMStorage) DeleteResourceScopes(ctx context.Context, resourceType, resourceID string) error {
	return s.db.WithContext(ctx).Delete(&schema.ResourceScopeModel{},
		"resource_type = ? AND resource_id = ?", resourceType, resourceID).Error
}

// ── Helpers ───────────────────────────────────────────────────────────────

func float32SliceToBytes(f []float32) []byte {
	b := make([]byte, len(f)*4)
	for i, v := range f {
		bits := math.Float32bits(v)
		b[i*4] = byte(bits)
		b[i*4+1] = byte(bits >> 8)
		b[i*4+2] = byte(bits >> 16)
		b[i*4+3] = byte(bits >> 24)
	}
	return b
}
