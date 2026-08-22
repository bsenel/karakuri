-- Phase 21: standing instructions to report.
--
-- Keyed on a twin rather than an objective. The reader wants one morning brief
-- covering everything they are responsible for, so a twin holding nine standing
-- objectives produces one message a day and not nine.
--
-- The server creates this table at boot through AutoMigrate; this file mirrors
-- it for operators who apply schema by hand.
CREATE TABLE IF NOT EXISTS report_schedules (
	id      TEXT PRIMARY KEY,
	twin_id TEXT NOT NULL,

	-- The same cadence shape an objective declares, and only its reconcile half
	-- means anything here: assembling a digest is a handful of queries, so
	-- there is no cheap tier to schedule separately.
	cadence_json TEXT NOT NULL DEFAULT '{}',

	-- Which adapter slot to deliver through, which named instance within it
	-- (empty resolves the twin's binding, ADR 006), and the address inside the
	-- channel — a Slack channel, an email recipient. What the target means is
	-- the adapter's business.
	channel  TEXT NOT NULL,
	instance TEXT NOT NULL DEFAULT '',
	target   TEXT NOT NULL DEFAULT '',

	-- How far back to look. Empty means "since the last one was sent", which
	-- makes a missed run catch up rather than lose a day.
	-- Quoted: "window" is a reserved word in PostgreSQL. GORM quotes its
	-- identifiers, so AutoMigrate creates this column happily and only the
	-- hand-applied path breaks — on the deployments least likely to notice
	-- quickly, and with the whole table missing rather than one column.
	"window" TEXT NOT NULL DEFAULT '',

	-- Off by default. A daily mail that says "nothing happened" is a mail
	-- people stop reading, which costs the ones that matter their audience.
	send_when_empty BOOLEAN NOT NULL DEFAULT FALSE,
	enabled         BOOLEAN NOT NULL DEFAULT TRUE,

	next_due_at  TIMESTAMP,
	last_sent_at TIMESTAMP,
	last_error   TEXT NOT NULL DEFAULT '',

	-- Grows the gap between retries after a failed send, so a schedule
	-- pointed at a misconfigured channel does not retry on every tick.
	consecutive_failures INTEGER NOT NULL DEFAULT 0,

	-- The lease, as reconcile_states carries one and for the same reason —
	-- with more at stake here. Two replicas reconciling one objective wastes
	-- money; two replicas sending one morning report send it to a person twice.
	holder      TEXT NOT NULL DEFAULT '',
	lease_until TIMESTAMP,

	created_at TIMESTAMP,
	updated_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_report_schedules_next_due_at ON report_schedules (next_due_at);
CREATE INDEX IF NOT EXISTS idx_report_schedules_twin_id ON report_schedules (twin_id);
CREATE INDEX IF NOT EXISTS idx_report_schedules_enabled ON report_schedules (enabled);
CREATE INDEX IF NOT EXISTS idx_report_schedules_lease_until ON report_schedules (lease_until);
