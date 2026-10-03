-- The template an objective was created from.
--
-- A template supplied an objective's criteria, constraints and agent, and the
-- objective then kept no reference back to it, so nobody asking what shaped a
-- decision could name the template that did.
--
-- Empty means "no template", which is what every existing row means.
ALTER TABLE objectives ADD COLUMN template_id TEXT NOT NULL DEFAULT '';

-- What produced the decision an audit row records.
--
-- An escalation or execute row named the agent and nothing else about how the
-- decision was reached: which provider and model drafted the plan, which
-- template the objective was built from, which autonomy rung the run stood on.
-- Columns rather than payload fields, so "everything this model decided" is a
-- WHERE clause and not a scan of every row's JSON.
--
-- Empty means "not recorded", which is what every existing row means. On a new
-- row an empty provider and model mean no model drafted the plan, and an empty
-- rung means a one-shot run.
ALTER TABLE tool_events ADD COLUMN provider TEXT NOT NULL DEFAULT '';
ALTER TABLE tool_events ADD COLUMN model TEXT NOT NULL DEFAULT '';
ALTER TABLE tool_events ADD COLUMN template_id TEXT NOT NULL DEFAULT '';
ALTER TABLE tool_events ADD COLUMN autonomy_rung TEXT NOT NULL DEFAULT '';
