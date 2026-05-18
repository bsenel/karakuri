CREATE TABLE IF NOT EXISTS sessions (
    sha TEXT PRIMARY KEY,
    mode TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'created',
    parent_sha TEXT,
    input TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS blobs (
    sha TEXT PRIMARY KEY,
    content BLOB NOT NULL,
    mime_type TEXT,
    size INTEGER NOT NULL,
    created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS manifests (
    session_sha TEXT PRIMARY KEY,
    content TEXT NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS artifacts (
    sha TEXT PRIMARY KEY,
    session_sha TEXT NOT NULL,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',
    role TEXT,
    created_at DATETIME NOT NULL,
    FOREIGN KEY (session_sha) REFERENCES sessions(sha)
);

CREATE TABLE IF NOT EXISTS reviews (
    sha TEXT PRIMARY KEY,
    session_sha TEXT NOT NULL,
    artifact_sha TEXT NOT NULL,
    role TEXT NOT NULL,
    verdict TEXT NOT NULL,
    feedback TEXT,
    created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS tool_events (
    id TEXT PRIMARY KEY,
    session_sha TEXT NOT NULL,
    adapter TEXT NOT NULL,
    operation TEXT NOT NULL,
    status TEXT NOT NULL,
    message TEXT,
    created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS checkpoints (
    id TEXT PRIMARY KEY,
    session_sha TEXT NOT NULL,
    summary TEXT NOT NULL,
    options TEXT NOT NULL,
    resolved INTEGER NOT NULL DEFAULT 0,
    decision TEXT,
    created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS action_items (
    id TEXT PRIMARY KEY,
    session_sha TEXT NOT NULL,
    source TEXT NOT NULL,
    priority TEXT NOT NULL,
    summary TEXT NOT NULL,
    created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS research_results (
    sha TEXT PRIMARY KEY,
    session_sha TEXT NOT NULL,
    topic TEXT NOT NULL,
    summary TEXT,
    confidence REAL,
    sources TEXT,
    created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS worktrees (
    task_id TEXT PRIMARY KEY,
    session_sha TEXT NOT NULL,
    path TEXT NOT NULL,
    branch TEXT NOT NULL,
    created_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_mode ON sessions(mode);
CREATE INDEX IF NOT EXISTS idx_artifacts_session ON artifacts(session_sha);
CREATE INDEX IF NOT EXISTS idx_worktrees_session ON worktrees(session_sha);
