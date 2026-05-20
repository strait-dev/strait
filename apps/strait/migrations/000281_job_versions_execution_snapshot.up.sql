ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS poison_pill_threshold INT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS debounce_window_secs INT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS batch_window_secs INT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS batch_max_size INT;
-- safety-ok: job_versions is append-only metadata and these constant defaults
-- are metadata-only on Postgres 11+; the snapshot reader needs non-null values
-- immediately for pre-existing versions.
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS execution_mode TEXT NOT NULL DEFAULT 'http';
-- safety-ok: same metadata-only snapshot default rationale as execution_mode.
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS preferred_regions TEXT[] NOT NULL DEFAULT '{}';
-- safety-ok: same metadata-only snapshot default rationale as execution_mode.
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS queue_name TEXT NOT NULL DEFAULT 'default';
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS on_complete_trigger_workflow TEXT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS on_complete_trigger_job TEXT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS on_complete_payload_mapping JSONB;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS on_failure_trigger_job TEXT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS on_failure_trigger_workflow TEXT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS on_failure_payload_mapping JSONB;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS max_tokens_per_run BIGINT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS max_tool_calls_per_run INT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS max_iterations_per_run INT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS allowed_tools TEXT[];
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS blocked_tools TEXT[];
-- safety-ok: same metadata-only ADD COLUMN treatment as the execution snapshot
-- defaults above.
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS paused BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS paused_at TIMESTAMPTZ;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS pause_reason TEXT;
-- safety-ok: stores encrypted/empty snapshot material; constant default is
-- metadata-only on Postgres 11+ and prevents NULL handling drift.
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS endpoint_signing_secret TEXT NOT NULL DEFAULT '';

UPDATE job_versions jv
SET poison_pill_threshold = j.poison_pill_threshold,
    debounce_window_secs = j.debounce_window_secs,
    batch_window_secs = j.batch_window_secs,
    batch_max_size = j.batch_max_size,
    execution_mode = j.execution_mode,
    preferred_regions = COALESCE(j.preferred_regions, '{}'),
    queue_name = COALESCE(NULLIF(j.queue_name, ''), 'default'),
    on_complete_trigger_workflow = j.on_complete_trigger_workflow,
    on_complete_trigger_job = j.on_complete_trigger_job,
    on_complete_payload_mapping = j.on_complete_payload_mapping,
    on_failure_trigger_job = j.on_failure_trigger_job,
    on_failure_trigger_workflow = j.on_failure_trigger_workflow,
    on_failure_payload_mapping = j.on_failure_payload_mapping,
    max_tokens_per_run = j.max_tokens_per_run,
    max_tool_calls_per_run = j.max_tool_calls_per_run,
    max_iterations_per_run = j.max_iterations_per_run,
    allowed_tools = j.allowed_tools,
    blocked_tools = j.blocked_tools,
    paused = j.paused,
    paused_at = j.paused_at,
    pause_reason = j.pause_reason,
    endpoint_signing_secret = j.endpoint_signing_secret
FROM jobs j
WHERE j.id = jv.job_id;
