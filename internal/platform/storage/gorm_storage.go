package storage

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/checkpoint"
	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
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
	return s.db.WithContext(ctx).Save(&schema.TwinModel{
		ID: t.ID, Name: t.Name, Kind: string(t.Kind), Domain: t.Domain,
		AgentsJSON: string(agentsJ), EnvsJSON: string(envsJ),
		ObjectivesJSON: string(objsJ), MemoryJSON: string(memJ),
		ChildrenJSON: string(childJ),
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
	_ = json.Unmarshal([]byte(m.AgentsJSON), &agents)
	_ = json.Unmarshal([]byte(m.EnvsJSON), &envs)
	_ = json.Unmarshal([]byte(m.ObjectivesJSON), &objs)
	_ = json.Unmarshal([]byte(m.MemoryJSON), &mem)
	_ = json.Unmarshal([]byte(m.ChildrenJSON), &children)

	// convert string slice to environment.EnvironmentID
	envIDs := make([]interface{}, len(envs))
	for i, e := range envs {
		envIDs[i] = e
	}
	objIDs := make([]interface{}, len(objs))
	for i, o := range objs {
		objIDs[i] = o
	}
	_ = envIDs
	_ = objIDs

	return twin.DigitalTwin{
		ID: m.ID, Name: m.Name, Kind: twin.Kind(m.Kind), Domain: m.Domain,
		Agents: agents, Children: children, Memory: mem,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// ── Objectives ────────────────────────────────────────────────────────────

func (s *GORMStorage) SaveObjective(ctx context.Context, o objective.Objective) error {
	critJ, _ := json.Marshal(o.SuccessCriteria)
	constrJ, _ := json.Marshal(o.Constraints)
	var parentID *string
	if o.ParentID != nil {
		pid := string(*o.ParentID)
		parentID = &pid
	}
	return s.db.WithContext(ctx).Save(&schema.ObjectiveModel{
		ID: string(o.ID), Title: o.Title, Description: o.Description, Domain: o.Domain,
		TwinID: o.TwinID, Priority: o.Priority, MaxIterations: o.MaxIterations, Deadline: o.Deadline,
		CriteriaJSON: string(critJ), ConstraintsJSON: string(constrJ), ParentID: parentID,
		Status: string(o.Status),
	}).Error
}

func (s *GORMStorage) GetObjective(ctx context.Context, id objective.ObjectiveID) (objective.Objective, error) {
	var m schema.ObjectiveModel
	if err := s.db.WithContext(ctx).First(&m, "id = ?", string(id)).Error; err != nil {
		return objective.Objective{}, coreerrors.ErrObjectiveNotFound
	}
	return objectiveFromModel(m), nil
}

func (s *GORMStorage) ListObjectives(ctx context.Context, twinID string, status string) ([]objective.Objective, error) {
	var models []schema.ObjectiveModel
	q := s.db.WithContext(ctx).Order("created_at DESC")
	if twinID != "" {
		q = q.Where("twin_id = ?", twinID)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
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
	_ = json.Unmarshal([]byte(m.CriteriaJSON), &criteria)
	_ = json.Unmarshal([]byte(m.ConstraintsJSON), &constraints)
	var parentID *objective.ObjectiveID
	if m.ParentID != nil {
		pid := objective.ObjectiveID(*m.ParentID)
		parentID = &pid
	}
	return objective.Objective{
		ID: objective.ObjectiveID(m.ID), Title: m.Title, Description: m.Description,
		Domain: m.Domain, TwinID: m.TwinID, Priority: m.Priority, MaxIterations: m.MaxIterations, Deadline: m.Deadline,
		SuccessCriteria: criteria, Constraints: constraints, ParentID: parentID,
		Status: objective.ObjectiveStatus(m.Status),
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
	var decJ string
	if c.Decision != nil {
		b, _ := json.Marshal(c.Decision)
		decJ = string(b)
	}
	return s.db.WithContext(ctx).Save(&schema.CheckpointModel{
		ID: c.ID, ObjectiveID: string(c.ObjectiveID), TwinID: c.TwinID,
		Reason: c.Reason, Summary: c.Summary, OptionsJSON: string(optsJ),
		Capability: string(c.Capability), Confidence: c.Confidence,
		Status: string(c.Status), DecisionJSON: decJ, ResolvedAt: c.ResolvedAt,
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
	var dec *checkpoint.Decision
	if m.DecisionJSON != "" {
		var d checkpoint.Decision
		_ = json.Unmarshal([]byte(m.DecisionJSON), &d)
		dec = &d
	}
	return checkpoint.Checkpoint{
		ID: m.ID, ObjectiveID: objective.ObjectiveID(m.ObjectiveID), TwinID: m.TwinID,
		Reason: m.Reason, Summary: m.Summary, Options: opts,
		Status: checkpoint.Status(m.Status), Decision: dec,
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
	return s.db.WithContext(ctx).Save(&schema.ToolEventModel{
		ID: e.ID, ObjectiveID: e.ObjectiveID, AgentID: e.AgentID, Capability: e.Capability,
		Adapter: e.Adapter, Success: e.Success, Confidence: e.Confidence, PayloadJSON: e.PayloadJSON,
	}).Error
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
