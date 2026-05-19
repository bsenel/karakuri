-- Karakuri v1 schema

CREATE TABLE IF NOT EXISTS twins (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    kind            TEXT NOT NULL,
    domain          TEXT NOT NULL,
    agents_json     TEXT NOT NULL DEFAULT '[]',
    envs_json       TEXT NOT NULL DEFAULT '[]',
    objectives_json TEXT NOT NULL DEFAULT '[]',
    memory_json     TEXT NOT NULL DEFAULT '{}',
    children_json   TEXT NOT NULL DEFAULT '[]',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS objectives (
    id               TEXT PRIMARY KEY,
    title            TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    domain           TEXT NOT NULL,
    priority         INTEGER NOT NULL DEFAULT 0,
    deadline         DATETIME,
    criteria_json    TEXT NOT NULL DEFAULT '[]',
    constraints_json TEXT NOT NULL DEFAULT '[]',
    parent_id        TEXT,
    status           TEXT NOT NULL DEFAULT 'pending',
    twin_id          TEXT,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_objectives_twin   ON objectives(twin_id);
CREATE INDEX IF NOT EXISTS idx_objectives_status ON objectives(status);

CREATE TABLE IF NOT EXISTS loop_iterations (
    id           TEXT PRIMARY KEY,
    objective_id TEXT NOT NULL,
    number       INTEGER NOT NULL,
    step         TEXT NOT NULL,
    input_json   TEXT,
    output_json  TEXT,
    tokens_used  INTEGER NOT NULL DEFAULT 0,
    duration_ms  INTEGER NOT NULL DEFAULT 0,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_iterations_objective ON loop_iterations(objective_id);

CREATE TABLE IF NOT EXISTS memory_episodic (
    id           TEXT PRIMARY KEY,
    agent_id     TEXT NOT NULL,
    twin_id      TEXT NOT NULL,
    domain       TEXT NOT NULL DEFAULT '',
    content      TEXT NOT NULL,
    confidence   REAL NOT NULL DEFAULT 1.0,
    sources_json TEXT NOT NULL DEFAULT '[]',
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at   DATETIME
);

CREATE INDEX IF NOT EXISTS idx_episodic_agent ON memory_episodic(agent_id);
CREATE INDEX IF NOT EXISTS idx_episodic_twin  ON memory_episodic(twin_id);

CREATE TABLE IF NOT EXISTS memory_procedural (
    id             TEXT PRIMARY KEY,
    agent_id       TEXT NOT NULL,
    twin_id        TEXT NOT NULL,
    capability_id  TEXT NOT NULL,
    success_count  INTEGER NOT NULL DEFAULT 0,
    failure_count  INTEGER NOT NULL DEFAULT 0,
    avg_confidence REAL NOT NULL DEFAULT 0.0,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(agent_id, capability_id)
);

CREATE INDEX IF NOT EXISTS idx_procedural_agent ON memory_procedural(agent_id);

CREATE TABLE IF NOT EXISTS memory_semantic (
    id           TEXT PRIMARY KEY,
    agent_id     TEXT NOT NULL,
    twin_id      TEXT NOT NULL,
    domain       TEXT NOT NULL DEFAULT '',
    content      TEXT NOT NULL,
    embedding    BLOB,
    confidence   REAL NOT NULL DEFAULT 1.0,
    sources_json TEXT NOT NULL DEFAULT '[]',
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at   DATETIME
);

CREATE INDEX IF NOT EXISTS idx_semantic_agent ON memory_semantic(agent_id);
CREATE INDEX IF NOT EXISTS idx_semantic_twin  ON memory_semantic(twin_id);

CREATE TABLE IF NOT EXISTS checkpoints (
    id            TEXT PRIMARY KEY,
    objective_id  TEXT NOT NULL,
    twin_id       TEXT NOT NULL,
    reason        TEXT NOT NULL DEFAULT '',
    summary       TEXT NOT NULL DEFAULT '',
    options_json  TEXT NOT NULL DEFAULT '[]',
    capability    TEXT NOT NULL DEFAULT '',
    confidence    REAL NOT NULL DEFAULT 0.0,
    status        TEXT NOT NULL DEFAULT 'pending',
    decision_json TEXT,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    resolved_at   DATETIME
);

CREATE INDEX IF NOT EXISTS idx_checkpoints_objective ON checkpoints(objective_id);
CREATE INDEX IF NOT EXISTS idx_checkpoints_status    ON checkpoints(status);

CREATE TABLE IF NOT EXISTS blobs (
    sha          TEXT PRIMARY KEY,
    content      BLOB NOT NULL,
    content_type TEXT NOT NULL DEFAULT 'text/plain',
    size         INTEGER NOT NULL DEFAULT 0,
    objective_id TEXT NOT NULL DEFAULT '',
    agent_id     TEXT NOT NULL DEFAULT '',
    capability   TEXT NOT NULL DEFAULT '',
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS worktrees (
    task_id      TEXT PRIMARY KEY,
    objective_id TEXT NOT NULL,
    path         TEXT NOT NULL,
    branch       TEXT NOT NULL,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_worktrees_objective ON worktrees(objective_id);

CREATE TABLE IF NOT EXISTS tool_events (
    id           TEXT PRIMARY KEY,
    objective_id TEXT NOT NULL DEFAULT '',
    agent_id     TEXT NOT NULL DEFAULT '',
    capability   TEXT NOT NULL DEFAULT '',
    adapter      TEXT NOT NULL DEFAULT '',
    success      INTEGER NOT NULL DEFAULT 0,
    confidence   REAL NOT NULL DEFAULT 0.0,
    payload_json TEXT,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_tool_events_objective ON tool_events(objective_id);
