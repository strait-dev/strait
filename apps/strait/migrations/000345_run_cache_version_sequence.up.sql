CREATE SEQUENCE IF NOT EXISTS job_run_cache_version_seq AS BIGINT;

SELECT setval(
    'job_run_cache_version_seq',
    GREATEST(
        (SELECT COALESCE(MAX(cache_version), 1) FROM job_run_cache_versions),
        (SELECT COALESCE(MAX(cache_version), 1) FROM job_runs),
        1
    ),
    true
);

CREATE OR REPLACE FUNCTION strait_next_run_cache_version(p_run_id TEXT)
RETURNS BIGINT
LANGUAGE plpgsql
VOLATILE
AS $$
BEGIN
    RETURN nextval('job_run_cache_version_seq'::regclass);
END;
$$;
