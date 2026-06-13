DROP FUNCTION IF EXISTS strait_next_run_cache_version(TEXT);

CREATE OR REPLACE FUNCTION strait_next_run_cache_version(p_run_id TEXT)
RETURNS BIGINT
LANGUAGE plpgsql
VOLATILE
AS $$
DECLARE
    next_version BIGINT;
BEGIN
    PERFORM pg_advisory_xact_lock(hashtext(p_run_id));

    SELECT COALESCE(
        (
            SELECT v.cache_version
            FROM job_run_cache_versions v
            WHERE v.run_id = p_run_id
            ORDER BY v.id DESC
            LIMIT 1
        ),
        (
            SELECT jr.cache_version
            FROM job_runs jr
            WHERE jr.id = p_run_id
        ),
        1
    ) + 1
    INTO next_version;

    RETURN next_version;
END;
$$;

DROP SEQUENCE IF EXISTS job_run_cache_version_seq;
