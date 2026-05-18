package schema

import "time"

type SessionModel struct {
	SHA       string    `gorm:"primaryKey;column:sha"`
	Mode      string    `gorm:"column:mode;not null"`
	State     string    `gorm:"column:state;not null;default:created"`
	ParentSHA string    `gorm:"column:parent_sha"`
	Input     string    `gorm:"column:input"`
	CreatedAt time.Time `gorm:"column:created_at;not null"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null"`
}

func (SessionModel) TableName() string { return "sessions" }

type BlobModel struct {
	SHA       string    `gorm:"primaryKey;column:sha"`
	Content   []byte    `gorm:"column:content;not null"`
	MimeType  string    `gorm:"column:mime_type"`
	Size      int64     `gorm:"column:size;not null"`
	CreatedAt time.Time `gorm:"column:created_at;not null"`
}

func (BlobModel) TableName() string { return "blobs" }

type ManifestModel struct {
	SessionSHA string    `gorm:"primaryKey;column:session_sha"`
	Content    string    `gorm:"column:content;not null"`
	UpdatedAt  time.Time `gorm:"column:updated_at;not null"`
}

func (ManifestModel) TableName() string { return "manifests" }

type ArtifactModel struct {
	SHA        string    `gorm:"primaryKey;column:sha"`
	SessionSHA string    `gorm:"column:session_sha;not null;index"`
	Name       string    `gorm:"column:name;not null"`
	Status     string    `gorm:"column:status;not null;default:draft"`
	Role       string    `gorm:"column:role"`
	CreatedAt  time.Time `gorm:"column:created_at;not null"`
}

func (ArtifactModel) TableName() string { return "artifacts" }

type ReviewModel struct {
	SHA         string    `gorm:"primaryKey;column:sha"`
	SessionSHA  string    `gorm:"column:session_sha;not null"`
	ArtifactSHA string    `gorm:"column:artifact_sha;not null"`
	Role        string    `gorm:"column:role;not null"`
	Verdict     string    `gorm:"column:verdict;not null"`
	Feedback    string    `gorm:"column:feedback"`
	CreatedAt   time.Time `gorm:"column:created_at;not null"`
}

func (ReviewModel) TableName() string { return "reviews" }

type ToolEventModel struct {
	ID         string    `gorm:"primaryKey;column:id"`
	SessionSHA string    `gorm:"column:session_sha;not null"`
	Adapter    string    `gorm:"column:adapter;not null"`
	Operation  string    `gorm:"column:operation;not null"`
	Status     string    `gorm:"column:status;not null"`
	Message    string    `gorm:"column:message"`
	CreatedAt  time.Time `gorm:"column:created_at;not null"`
}

func (ToolEventModel) TableName() string { return "tool_events" }

type CheckpointModel struct {
	ID         string    `gorm:"primaryKey;column:id"`
	SessionSHA string    `gorm:"column:session_sha;not null"`
	Summary    string    `gorm:"column:summary;not null"`
	Options    string    `gorm:"column:options;not null"`
	Resolved   bool      `gorm:"column:resolved;not null;default:false"`
	Decision   string    `gorm:"column:decision"`
	CreatedAt  time.Time `gorm:"column:created_at;not null"`
}

func (CheckpointModel) TableName() string { return "checkpoints" }

type ActionItemModel struct {
	ID         string    `gorm:"primaryKey;column:id"`
	SessionSHA string    `gorm:"column:session_sha;not null"`
	Source     string    `gorm:"column:source;not null"`
	Priority   string    `gorm:"column:priority;not null"`
	Summary    string    `gorm:"column:summary;not null"`
	CreatedAt  time.Time `gorm:"column:created_at;not null"`
}

func (ActionItemModel) TableName() string { return "action_items" }

type ResearchResultModel struct {
	SHA        string    `gorm:"primaryKey;column:sha"`
	SessionSHA string    `gorm:"column:session_sha;not null"`
	Topic      string    `gorm:"column:topic;not null"`
	Summary    string    `gorm:"column:summary"`
	Confidence float64   `gorm:"column:confidence"`
	Sources    string    `gorm:"column:sources"`
	CreatedAt  time.Time `gorm:"column:created_at;not null"`
}

func (ResearchResultModel) TableName() string { return "research_results" }

type WorktreeModel struct {
	TaskID     string    `gorm:"primaryKey;column:task_id"`
	SessionSHA string    `gorm:"column:session_sha;not null;index"`
	Path       string    `gorm:"column:path;not null"`
	Branch     string    `gorm:"column:branch;not null"`
	CreatedAt  time.Time `gorm:"column:created_at;not null"`
}

func (WorktreeModel) TableName() string { return "worktrees" }
