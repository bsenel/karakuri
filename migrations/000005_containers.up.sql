-- Phase 17: the tenancy tree, and the flattened closure authorization matches
-- against. The server applies these through AutoMigrate at boot; this file
-- mirrors it for operators who apply schema changes by hand.

CREATE TABLE IF NOT EXISTS containers (
	id         TEXT PRIMARY KEY,
	kind       TEXT NOT NULL,
	name       TEXT NOT NULL,
	parent_id  TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMP,
	updated_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_containers_parent ON containers (parent_id);

-- Names are unique among siblings of the same kind, and nowhere else. Two
-- organisations may both have a team called "Engineering" — that is the case
-- scoping on IDs exists to survive, so the schema must permit it.
CREATE UNIQUE INDEX IF NOT EXISTS idx_containers_sibling
	ON containers (parent_id, kind, name);

-- One row per label a resource carries. `direct` marks a declared membership;
-- the rest are derived by closing over ancestry, and are recomputed when the
-- tree changes. Both are matched identically at read time — the listing query
-- does not care how a label got here.
CREATE TABLE IF NOT EXISTS resource_scopes (
	resource_type TEXT NOT NULL,
	resource_id   TEXT NOT NULL,
	label         TEXT NOT NULL,
	direct        BOOLEAN NOT NULL DEFAULT FALSE,
	PRIMARY KEY (resource_type, resource_id, label)
);

-- The index that makes subtree listing an indexed IN clause rather than the
-- unindexable prefix scan a path-shaped hierarchy would need.
CREATE INDEX IF NOT EXISTS idx_resource_scopes_label ON resource_scopes (label);
