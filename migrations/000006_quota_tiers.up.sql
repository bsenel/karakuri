-- Phase 19: the limits an operator has set, which take precedence over the ones
-- in the configuration file. The server creates this table at boot through the
-- quota tier store; this file mirrors it for operators who apply schema by hand.
--
-- Configuration is the seed and the fallback: a fresh database has no rows here
-- and every tier reads from YAML, and deleting a row returns that tier to it.
-- One row per tier, keyed on the name the limit is enforced under so a stored
-- tier and an override cannot disagree about what they are talking about.
CREATE TABLE IF NOT EXISTS quota_tiers (
	name       TEXT PRIMARY KEY,
	cap_value  INTEGER NOT NULL,
	-- window_ms and rate apply to the request tier only. A daily quota's period
	-- is a calendar span rather than a duration, and is not editable here:
	-- changing how a limit is counted is a different decision from raising it.
	window_ms  INTEGER NOT NULL DEFAULT 0,
	rate       DOUBLE PRECISION NOT NULL DEFAULT 0,
	-- Required at write time. A limit changed for a reason nobody wrote down is
	-- one nobody can review later, and this one changed it for everybody.
	reason     TEXT NOT NULL DEFAULT '',
	updated_by TEXT NOT NULL DEFAULT '',
	updated_ms INTEGER NOT NULL DEFAULT 0
);
