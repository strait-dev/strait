CREATE TABLE IF NOT EXISTS job_run_cache_versions (
    run_id        TEXT PRIMARY KEY,
    cache_version BIGINT NOT NULL DEFAULT 1
);

INSERT INTO job_run_cache_versions (run_id, cache_version)
SELECT id, cache_version
FROM job_runs
ON CONFLICT (run_id) DO NOTHING;
