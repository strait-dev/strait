DROP TRIGGER IF EXISTS trg_job_runs_claim_queue_sync ON job_runs;
DROP TRIGGER IF EXISTS trg_job_runs_claim_queue_sync_update ON job_runs;
DROP FUNCTION IF EXISTS trg_job_runs_sync_claim_queue();
