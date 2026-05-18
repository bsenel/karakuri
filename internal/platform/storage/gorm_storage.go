package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/internal/core/entity"
	"github.com/bsenel/karakuri/internal/core/errors"
	"github.com/bsenel/karakuri/internal/core/vfs"
	"github.com/bsenel/karakuri/internal/platform/db/schema"
	"github.com/bsenel/karakuri/internal/platform/git"
	"gorm.io/gorm"
)

type GORMStorage struct {
	db *gorm.DB
}

func NewGORMStorage(db *gorm.DB) *GORMStorage {
	return &GORMStorage{db: db}
}

func ContentSHA(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}

func (s *GORMStorage) SaveBlob(ctx context.Context, sha string, content []byte, meta BlobMetadata) error {
	return s.db.WithContext(ctx).Save(&schema.BlobModel{
		SHA: sha, Content: content, MimeType: meta.MimeType, Size: meta.Size, CreatedAt: Now(),
	}).Error
}

func (s *GORMStorage) GetBlob(ctx context.Context, sha string) ([]byte, BlobMetadata, error) {
	var m schema.BlobModel
	if err := s.db.WithContext(ctx).First(&m, "sha = ?", sha).Error; err != nil {
		return nil, BlobMetadata{}, errors.ErrNotFound
	}
	return m.Content, BlobMetadata{MimeType: m.MimeType, Size: m.Size}, nil
}

func (s *GORMStorage) SaveManifest(ctx context.Context, sessionSHA string, manifest vfs.Manifest) error {
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Save(&schema.ManifestModel{
		SessionSHA: sessionSHA, Content: string(data), UpdatedAt: Now(),
	}).Error
}

func (s *GORMStorage) GetManifest(ctx context.Context, sessionSHA string) (vfs.Manifest, error) {
	var m schema.ManifestModel
	if err := s.db.WithContext(ctx).First(&m, "session_sha = ?", sessionSHA).Error; err != nil {
		return vfs.Manifest{}, errors.ErrNotFound
	}
	var manifest vfs.Manifest
	if err := json.Unmarshal([]byte(m.Content), &manifest); err != nil {
		return vfs.Manifest{}, err
	}
	return manifest, nil
}

func (s *GORMStorage) UpdateArtifactStatus(ctx context.Context, sha string, status vfs.ArtifactStatus) error {
	return s.db.WithContext(ctx).Model(&schema.ArtifactModel{}).Where("sha = ?", sha).Update("status", string(status)).Error
}

func (s *GORMStorage) ListSessions(ctx context.Context, filter SessionFilter) ([]entity.Session, error) {
	var models []schema.SessionModel
	q := s.db.WithContext(ctx).Order("created_at DESC")
	if filter.Mode != "" {
		q = q.Where("mode = ?", filter.Mode)
	}
	if filter.Limit > 0 {
		q = q.Limit(filter.Limit)
	}
	if err := q.Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]entity.Session, len(models))
	for i, m := range models {
		out[i] = sessionFromModel(m)
	}
	return out, nil
}

func (s *GORMStorage) QueryArtifacts(ctx context.Context, filter ArtifactFilter) ([]entity.Artifact, error) {
	var models []schema.ArtifactModel
	q := s.db.WithContext(ctx)
	if filter.SessionSHA != "" {
		q = q.Where("session_sha = ?", filter.SessionSHA)
	}
	if filter.Name != "" {
		q = q.Where("name = ?", filter.Name)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if err := q.Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]entity.Artifact, len(models))
	for i, m := range models {
		out[i] = artifactFromModel(m)
	}
	return out, nil
}

func (s *GORMStorage) SaveReview(ctx context.Context, review entity.Review) error {
	return s.db.WithContext(ctx).Save(&schema.ReviewModel{
		SHA: review.SHA, SessionSHA: review.SessionSHA, ArtifactSHA: review.ArtifactSHA,
		Role: review.Role, Verdict: review.Verdict, Feedback: review.Feedback, CreatedAt: review.CreatedAt,
	}).Error
}

func (s *GORMStorage) SaveToolEvent(ctx context.Context, event entity.ToolEvent) error {
	return s.db.WithContext(ctx).Save(&schema.ToolEventModel{
		ID: event.ID, SessionSHA: event.SessionSHA, Adapter: event.Adapter,
		Operation: event.Operation, Status: event.Status, Message: event.Message, CreatedAt: event.CreatedAt,
	}).Error
}

func (s *GORMStorage) SaveCheckpoint(ctx context.Context, cp entity.Checkpoint) error {
	opts, _ := json.Marshal(cp.Options)
	return s.db.WithContext(ctx).Save(&schema.CheckpointModel{
		ID: cp.ID, SessionSHA: cp.SessionSHA, Summary: cp.Summary, Options: string(opts),
		Resolved: cp.Resolved, Decision: cp.Decision, CreatedAt: cp.CreatedAt,
	}).Error
}

func (s *GORMStorage) ResolveCheckpoint(ctx context.Context, id string, decision entity.CheckpointDecision) error {
	return s.db.WithContext(ctx).Model(&schema.CheckpointModel{}).Where("id = ?", id).Updates(map[string]any{
		"resolved": true, "decision": string(decision),
	}).Error
}

func (s *GORMStorage) SaveActionItem(ctx context.Context, item entity.ActionItem) error {
	return s.db.WithContext(ctx).Save(&schema.ActionItemModel{
		ID: item.ID, SessionSHA: item.SessionSHA, Source: item.Source,
		Priority: item.Priority, Summary: item.Summary, CreatedAt: item.CreatedAt,
	}).Error
}

func (s *GORMStorage) SaveResearchResult(ctx context.Context, result entity.ResearchResult) error {
	sources, _ := json.Marshal(result.Sources)
	return s.db.WithContext(ctx).Save(&schema.ResearchResultModel{
		SHA: result.SHA, SessionSHA: result.SessionSHA, Topic: result.Topic,
		Summary: result.Summary, Confidence: result.Confidence, Sources: string(sources), CreatedAt: result.CreatedAt,
	}).Error
}

func (s *GORMStorage) SaveWorktree(ctx context.Context, wt git.Worktree) error {
	return s.db.WithContext(ctx).Save(&schema.WorktreeModel{
		TaskID: wt.TaskID, SessionSHA: wt.SessionSHA, Path: wt.Path, Branch: wt.Branch, CreatedAt: wt.CreatedAt,
	}).Error
}

func (s *GORMStorage) GetWorktree(ctx context.Context, taskID string) (git.Worktree, error) {
	var m schema.WorktreeModel
	if err := s.db.WithContext(ctx).First(&m, "task_id = ?", taskID).Error; err != nil {
		return git.Worktree{}, errors.ErrNotFound
	}
	return git.Worktree{TaskID: m.TaskID, SessionSHA: m.SessionSHA, Path: m.Path, Branch: m.Branch, CreatedAt: m.CreatedAt}, nil
}

func (s *GORMStorage) ListWorktrees(ctx context.Context, sessionSHA string) ([]git.Worktree, error) {
	var models []schema.WorktreeModel
	if err := s.db.WithContext(ctx).Where("session_sha = ?", sessionSHA).Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]git.Worktree, len(models))
	for i, m := range models {
		out[i] = git.Worktree{TaskID: m.TaskID, SessionSHA: m.SessionSHA, Path: m.Path, Branch: m.Branch, CreatedAt: m.CreatedAt}
	}
	return out, nil
}

func (s *GORMStorage) DeleteWorktree(ctx context.Context, taskID string) error {
	return s.db.WithContext(ctx).Delete(&schema.WorktreeModel{}, "task_id = ?", taskID).Error
}

func (s *GORMStorage) SaveSession(ctx context.Context, sess entity.Session) error {
	return s.db.WithContext(ctx).Save(&schema.SessionModel{
		SHA: sess.SHA, Mode: string(sess.Mode), State: string(sess.State),
		ParentSHA: sess.ParentSHA, Input: sess.Input, CreatedAt: sess.CreatedAt, UpdatedAt: sess.UpdatedAt,
	}).Error
}

func (s *GORMStorage) GetSession(ctx context.Context, sha string) (entity.Session, error) {
	var m schema.SessionModel
	if err := s.db.WithContext(ctx).First(&m, "sha = ?", sha).Error; err != nil {
		return entity.Session{}, errors.ErrNotFound
	}
	return sessionFromModel(m), nil
}

func (s *GORMStorage) UpdateSessionState(ctx context.Context, sha string, state entity.SessionState) error {
	return s.db.WithContext(ctx).Model(&schema.SessionModel{}).Where("sha = ?", sha).Updates(map[string]any{
		"state": string(state), "updated_at": Now(),
	}).Error
}

func (s *GORMStorage) SaveArtifact(ctx context.Context, a entity.Artifact) error {
	return s.db.WithContext(ctx).Save(&schema.ArtifactModel{
		SHA: a.SHA, SessionSHA: a.SessionSHA, Name: a.Name, Status: a.Status, Role: a.Role, CreatedAt: a.CreatedAt,
	}).Error
}

func (s *GORMStorage) GetArtifact(ctx context.Context, sha string) (entity.Artifact, error) {
	var m schema.ArtifactModel
	if err := s.db.WithContext(ctx).First(&m, "sha = ?", sha).Error; err != nil {
		return entity.Artifact{}, errors.ErrNotFound
	}
	return artifactFromModel(m), nil
}

func (s *GORMStorage) ListCheckpoints(ctx context.Context, sessionSHA string) ([]entity.Checkpoint, error) {
	var models []schema.CheckpointModel
	if err := s.db.WithContext(ctx).Where("session_sha = ?", sessionSHA).Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]entity.Checkpoint, len(models))
	for i, m := range models {
		var opts []string
		_ = json.Unmarshal([]byte(m.Options), &opts)
		out[i] = entity.Checkpoint{ID: m.ID, SessionSHA: m.SessionSHA, Summary: m.Summary, Options: opts, Resolved: m.Resolved, Decision: m.Decision, CreatedAt: m.CreatedAt}
	}
	return out, nil
}

func (s *GORMStorage) GetReviews(ctx context.Context, sessionSHA string) ([]entity.Review, error) {
	var models []schema.ReviewModel
	if err := s.db.WithContext(ctx).Where("session_sha = ?", sessionSHA).Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]entity.Review, len(models))
	for i, m := range models {
		out[i] = entity.Review{SHA: m.SHA, SessionSHA: m.SessionSHA, ArtifactSHA: m.ArtifactSHA, Role: m.Role, Verdict: m.Verdict, Feedback: m.Feedback, CreatedAt: m.CreatedAt}
	}
	return out, nil
}

func sessionFromModel(m schema.SessionModel) entity.Session {
	return entity.Session{SHA: m.SHA, Mode: entity.SessionMode(m.Mode), State: entity.SessionState(m.State), ParentSHA: m.ParentSHA, Input: m.Input, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

func artifactFromModel(m schema.ArtifactModel) entity.Artifact {
	return entity.Artifact{SHA: m.SHA, SessionSHA: m.SessionSHA, Name: m.Name, Status: m.Status, Role: m.Role, CreatedAt: m.CreatedAt}
}

func NewSessionSHA() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(time.Now().String())))
}
