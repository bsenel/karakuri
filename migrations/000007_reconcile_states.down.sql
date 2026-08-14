DROP TABLE IF EXISTS reconcile_outcomes;
DROP TABLE IF EXISTS reconcile_states;

DROP INDEX IF EXISTS idx_objectives_mode;
ALTER TABLE objectives DROP COLUMN autonomy_json;
ALTER TABLE objectives DROP COLUMN cadence_json;
ALTER TABLE objectives DROP COLUMN mode;

-- loop_states is deliberately NOT dropped here. It predates this migration in
-- the models (Phase 11) and only appears in the up file because no migration
-- ever created it; dropping it on the way down would delete state this
-- migration did not introduce, and would strand every running loop.
