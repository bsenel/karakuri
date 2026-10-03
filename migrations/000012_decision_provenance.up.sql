-- The template an objective was created from.
--
-- A template supplied an objective's criteria, constraints and agent, and the
-- objective then kept no reference back to it, so nobody asking what shaped a
-- decision could name the template that did.
--
-- Empty means "no template", which is what every existing row means.
ALTER TABLE objectives ADD COLUMN template_id TEXT NOT NULL DEFAULT '';
