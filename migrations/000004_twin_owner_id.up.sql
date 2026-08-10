-- Phase 14: twins record the principal that created them. Existing rows keep
-- an empty owner, which policy conditions treat as unowned — owner_equals is
-- never satisfied by a resource with no owner, so ownership-scoped grants do
-- not silently cover twins that predate this column.
ALTER TABLE twins ADD COLUMN owner_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_twins_owner ON twins (owner_id);
