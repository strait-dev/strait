-- Validate the singleton on-conflict CHECK constraints added NOT VALID in 000310.
-- Running VALIDATE in its own migration (a separate transaction from the ADD)
-- means it takes only a SHARE UPDATE EXCLUSIVE lock, so the validating scan of
-- jobs/workflows does not block concurrent reads or writes. Both constraints
-- only have to clear all-NULL columns, so the scan is trivial, but keeping the
-- pattern correct documents intent and marks the constraints valid for the
-- planner.
ALTER TABLE jobs VALIDATE CONSTRAINT jobs_singleton_on_conflict_check;

ALTER TABLE workflows VALIDATE CONSTRAINT workflows_singleton_on_conflict_check;
