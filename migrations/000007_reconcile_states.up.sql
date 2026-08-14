-- Phase 20: the outer control loop's durable state.
--
-- The server creates these tables at boot through AutoMigrate; this file
-- mirrors them for operators who apply schema by hand. It also carries
-- loop_states, which Phase 11 added to the models and never wrote a migration
-- for — a hand-applied database has been missing it since.

-- What an operator declares about a standing objective lives on the objective,
-- because it is part of the objective's description and is edited by whoever
-- wrote it. What the system discovers by running lives in reconcile_states
-- below, because it is written by the supervisor and edited by nobody.
--
-- An empty mode is oneshot, so every row written before Phase 20 keeps its
-- behaviour exactly: adding a column must not change what an existing row
-- means. Mode is indexed because the supervisor's boot scan asks for standing
-- objectives and nothing else.
ALTER TABLE objectives ADD COLUMN mode TEXT NOT NULL DEFAULT '';
ALTER TABLE objectives ADD COLUMN cadence_json TEXT NOT NULL DEFAULT '';
ALTER TABLE objectives ADD COLUMN autonomy_json TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_objectives_mode ON objectives (mode);

-- One row per standing objective. The primary key is the objective itself
-- because a standing objective has exactly one control loop by definition,
-- which is also what makes claiming one a single-row conditional UPDATE.
CREATE TABLE IF NOT EXISTS reconcile_states (
	objective_id  TEXT PRIMARY KEY,
	twin_id       TEXT NOT NULL DEFAULT '',
	phase         TEXT NOT NULL DEFAULT 'idle',
	-- Survives restarts on purpose. A circuit breaker that forgot it had
	-- tripped would put the failing objective straight back into rotation.
	paused        BOOLEAN NOT NULL DEFAULT FALSE,
	paused_reason TEXT NOT NULL DEFAULT '',

	-- Nullable, and the nullability is load-bearing. "Never due on its own"
	-- is a real state — a standing objective that reconciles only when asked
	-- — and a zero timestamp would read as overdue since year one, putting it
	-- in every due scan forever.
	next_due_at       TIMESTAMP,
	next_sense_at     TIMESTAMP,
	next_reconcile_at TIMESTAMP,

	-- The fingerprint at the last convergence: the composite hash, the
	-- per-environment hashes behind it, and which environments could not be
	-- hashed at all. Drift is measured against this rather than against the
	-- previous observation, so an environment that flaps away and back is not
	-- drift.
	converged_json    TEXT NOT NULL DEFAULT '{}',
	last_converged_at TIMESTAMP,

	last_run_at        TIMESTAMP,
	last_reconciled_at TIMESTAMP,
	last_trigger       TEXT NOT NULL DEFAULT '',
	last_outcome_id    TEXT NOT NULL DEFAULT '',
	last_error         TEXT NOT NULL DEFAULT '',

	criteria_met         DOUBLE PRECISION NOT NULL DEFAULT 0.0,
	-- Consecutive reconciles that failed to improve the score. An objective
	-- whose score has not moved in three expensive runs is not converging,
	-- and running it a fourth time buys nothing.
	score_streak         INTEGER NOT NULL DEFAULT 0,
	consecutive_failures INTEGER NOT NULL DEFAULT 0,

	-- The level this objective has earned. Always re-clamped to the ceiling
	-- its declaration set when it is read, so a row edited by hand cannot
	-- widen what the objective may do.
	autonomy   TEXT NOT NULL DEFAULT '',
	clean_runs INTEGER NOT NULL DEFAULT 0,

	-- The lease. These two columns are what stop two replicas reconciling the
	-- same objective, sending the same mail and paying twice. A crashed holder
	-- releases nothing; its lease simply runs out.
	holder      TEXT NOT NULL DEFAULT '',
	lease_until TIMESTAMP,

	created_at TIMESTAMP,
	updated_at TIMESTAMP
);

-- The due-wheel query, and the only index on the hot path: the supervisor asks
-- "what is due" on every tick and must not scan.
CREATE INDEX IF NOT EXISTS idx_reconcile_states_next_due_at ON reconcile_states (next_due_at);
CREATE INDEX IF NOT EXISTS idx_reconcile_states_lease_until ON reconcile_states (lease_until);
CREATE INDEX IF NOT EXISTS idx_reconcile_states_paused ON reconcile_states (paused);
CREATE INDEX IF NOT EXISTS idx_reconcile_states_twin_id ON reconcile_states (twin_id);

-- One row per completed pass, cheap or expensive. Sense-only passes are
-- recorded too and are the majority: "checked forty-eight times today, cost
-- nothing" is the evidence the two-tier split is working, and a history
-- holding only the expensive rows would look like a system barely watching.
CREATE TABLE IF NOT EXISTS reconcile_outcomes (
	id           TEXT PRIMARY KEY,
	objective_id TEXT NOT NULL,
	twin_id      TEXT NOT NULL DEFAULT '',

	trigger TEXT NOT NULL,
	-- Empty on a sense-only pass, which is the common case and the one the
	-- design exists to make cheap.
	loop_id TEXT NOT NULL DEFAULT '',

	drift_json TEXT NOT NULL DEFAULT '{}',
	autonomy   TEXT NOT NULL DEFAULT '',

	criteria_met DOUBLE PRECISION NOT NULL DEFAULT 0.0,
	converged    BOOLEAN NOT NULL DEFAULT FALSE,

	-- An escalation is not a failure: a loop that stopped to ask a question
	-- did the right thing, and error stays empty for it.
	escalated     BOOLEAN NOT NULL DEFAULT FALSE,
	checkpoint_id TEXT NOT NULL DEFAULT '',
	error         TEXT NOT NULL DEFAULT '',

	started_at TIMESTAMP,
	ended_at   TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_reconcile_outcomes_objective_time ON reconcile_outcomes (objective_id, started_at DESC);

-- Phase 11's durable loop state, backfilled here. The models have carried it
-- since Phase 11; the migrations never did.
CREATE TABLE IF NOT EXISTS loop_states (
	loop_id       TEXT PRIMARY KEY,
	objective_id  TEXT NOT NULL,
	twin_id       TEXT NOT NULL DEFAULT '',
	agent_id      TEXT NOT NULL DEFAULT '',
	iteration     INTEGER NOT NULL DEFAULT 0,
	paused        BOOLEAN NOT NULL DEFAULT FALSE,
	completed     BOOLEAN NOT NULL DEFAULT FALSE,
	last_step     TEXT NOT NULL DEFAULT '',
	status        TEXT NOT NULL DEFAULT '',
	criteria_met  DOUBLE PRECISION NOT NULL DEFAULT 0.0,
	checkpoint_id TEXT NOT NULL DEFAULT '',
	-- The full marshalled loop.Request: enough to reconstruct the loop's
	-- parameters on a cold start.
	request_json  TEXT NOT NULL DEFAULT '{}',
	created_at    TIMESTAMP,
	updated_at    TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_loop_states_objective_id ON loop_states (objective_id);
CREATE INDEX IF NOT EXISTS idx_loop_states_completed ON loop_states (completed);
