-- The world state the planner saw when it escalated.
--
-- A checkpoint recorded the drafted plan but not the observations it was
-- drafted from, so a resolved checkpoint could score the judge but never
-- replay the planner. The column holds loop.WorldState as JSON.
--
-- Empty means "not recorded", which is what every existing row and every
-- budget pause means.
ALTER TABLE checkpoints ADD COLUMN world_state_json TEXT NOT NULL DEFAULT '';
