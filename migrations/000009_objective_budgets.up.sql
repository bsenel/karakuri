-- Phase 23: per-objective spend ceilings.
--
-- A standing objective reconciles on a clock nobody watches, and its appetite
-- is the part an operator has had no chance to calibrate. These columns let a
-- ceiling be declared on the objective itself, separately from the twin's
-- allowance it otherwise draws on.
--
-- Mirrors what AutoMigrate produces, for operators who apply schema by hand.

-- The declaration. Empty string means no ceiling of its own, which is what
-- every objective written before this reads as: adding a column must not
-- change what an existing row means.
ALTER TABLE objectives ADD COLUMN budget_json TEXT NOT NULL DEFAULT '';

-- Why a pass that was due did not spend. Distinct from `error`, because a
-- deferral is the system working — nothing misbehaved, the conditions for
-- spending were simply not met — and recording it as failure would walk an
-- objective into the circuit breaker for staying inside its ceiling.
ALTER TABLE reconcile_outcomes ADD COLUMN deferred TEXT NOT NULL DEFAULT '';

-- When the condition clears. Becomes the floor on the next due time, so a
-- deferral cannot be scheduled over by the cadence.
ALTER TABLE reconcile_outcomes ADD COLUMN deferred_until TIMESTAMP;
