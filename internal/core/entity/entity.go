package entity

import "time"

type SessionMode string

const (
	ModeStrategy   SessionMode = "strategy"
	ModeDiscovery  SessionMode = "discovery"
	ModeDelivery   SessionMode = "delivery"
	ModeAutonomous SessionMode = "autonomous"
)

type SessionState string

const (
	StateCreated   SessionState = "created"
	StatePlanning  SessionState = "planning"
	StateRunning   SessionState = "running"
	StateAwaiting  SessionState = "awaiting"
	StateCompleted SessionState = "completed"
	StateFailed    SessionState = "failed"
	StateRetrying  SessionState = "retrying"
)

type Session struct {
	SHA       string       `json:"sha"`
	Mode      SessionMode  `json:"mode"`
	State     SessionState `json:"state"`
	ParentSHA string       `json:"parent_sha,omitempty"`
	Input     string       `json:"input,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

type Artifact struct {
	SHA        string    `json:"sha"`
	SessionSHA string    `json:"session_sha"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Role       string    `json:"role,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type Review struct {
	SHA           string    `json:"sha"`
	SessionSHA    string    `json:"session_sha"`
	ArtifactSHA   string    `json:"artifact_sha"`
	Role          string    `json:"role"`
	Verdict       string    `json:"verdict"`
	Feedback      string    `json:"feedback"`
	CreatedAt     time.Time `json:"created_at"`
}

type Checkpoint struct {
	ID         string    `json:"id"`
	SessionSHA string    `json:"session_sha"`
	Summary    string    `json:"summary"`
	Options    []string  `json:"options"`
	Resolved   bool      `json:"resolved"`
	Decision   string    `json:"decision,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type CheckpointDecision string

const (
	DecisionApprove CheckpointDecision = "approve"
	DecisionReject  CheckpointDecision = "reject"
	DecisionPromote CheckpointDecision = "promote"
	DecisionRetry   CheckpointDecision = "retry"
	DecisionSkip    CheckpointDecision = "skip"
	DecisionAbort   CheckpointDecision = "abort"
)

type ToolEvent struct {
	ID         string    `json:"id"`
	SessionSHA string    `json:"session_sha"`
	Adapter    string    `json:"adapter"`
	Operation  string    `json:"operation"`
	Status     string    `json:"status"`
	Message    string    `json:"message,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type ActionItem struct {
	ID         string    `json:"id"`
	SessionSHA string    `json:"session_sha"`
	Source     string    `json:"source"`
	Priority   string    `json:"priority"`
	Summary    string    `json:"summary"`
	CreatedAt  time.Time `json:"created_at"`
}

type ResearchResult struct {
	SHA        string    `json:"sha"`
	SessionSHA string    `json:"session_sha"`
	Topic      string    `json:"topic"`
	Summary    string    `json:"summary"`
	Confidence float64   `json:"confidence"`
	Sources    []string  `json:"sources"`
	CreatedAt  time.Time `json:"created_at"`
}
