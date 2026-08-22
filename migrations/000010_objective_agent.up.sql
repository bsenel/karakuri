-- The agent an objective runs under.
--
-- Template.SuggestedAgents was declared and read by nothing: an objective
-- created from a template kept no reference back to it, so a template naming
-- the right agent could not make that agent run. Selection fell back to the
-- first agent the domain declared, which was correct in a two-agent pack and
-- silently wrong in a nine-agent one.
--
-- Empty means "the domain's default", which is what every existing row means.
ALTER TABLE objectives ADD COLUMN agent_id TEXT NOT NULL DEFAULT '';
