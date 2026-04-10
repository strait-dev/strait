-- Add UNIQUE(job_id, version) to code_deployments.
--
-- The version number is derived as SELECT MAX(version)+1 at insert time.
-- Without this constraint, two concurrent deployments for the same job can
-- read the same MAX and insert duplicate version numbers, producing confusing
-- duplicate entries in the list API.
--
-- Before adding the constraint, repair any existing duplicates by re-assigning
-- version numbers in creation order (oldest deployment keeps version 1).

WITH numbered AS (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY job_id ORDER BY created_at, id) AS new_version
    FROM code_deployments
)
UPDATE code_deployments c
SET version = n.new_version
FROM numbered n
WHERE c.id = n.id;

ALTER TABLE code_deployments
    ADD CONSTRAINT code_deployments_job_id_version_unique UNIQUE (job_id, version);
