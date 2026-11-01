-- How a loop ended, and the criterion states its last verify step settled
-- (Phase 41 Step 1).
--
-- Two columns rather than one, because they are different facts: a run that
-- spent its iteration budget has ended, and that says nothing about whether
-- its criteria hold. Reconcile reads the states to tell a pass whose required
-- criterion was not met from one whose verifier could not run.
--
-- Empty means "not recorded", which is what every existing row means; a
-- reader with no states falls back to the score, as before.
ALTER TABLE loop_states ADD COLUMN outcome TEXT NOT NULL DEFAULT '';
ALTER TABLE loop_states ADD COLUMN criteria_json TEXT NOT NULL DEFAULT '[]';

-- Whether a reconcile pass ended with a required criterion inconclusive and
-- none not met. Such a pass is neither a failure nor a convergence, and the
-- circuit breaker does not count it.
ALTER TABLE reconcile_outcomes ADD COLUMN inconclusive BOOLEAN NOT NULL DEFAULT FALSE;
