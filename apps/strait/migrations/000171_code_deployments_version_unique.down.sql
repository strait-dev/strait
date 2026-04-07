ALTER TABLE code_deployments
    DROP CONSTRAINT IF EXISTS code_deployments_job_id_version_unique;
