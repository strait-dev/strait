CREATE OR REPLACE FUNCTION cleanup_terminal_run_ephemeral_queue()
RETURNS trigger AS $$
BEGIN
    DELETE FROM job_run_active_claims WHERE run_id = NEW.run_id;
    DELETE FROM job_run_queue WHERE run_id = NEW.run_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS job_run_terminal_state_cleanup_ephemeral_trg ON job_run_terminal_state;

CREATE TRIGGER job_run_terminal_state_cleanup_ephemeral_trg
AFTER INSERT ON job_run_terminal_state
FOR EACH ROW
EXECUTE FUNCTION cleanup_terminal_run_ephemeral_queue();

-- safety-ok: job_run_active_claims and job_run_queue are ephemeral dispatch
-- side tables; terminal state is authoritative and terminal runs must not
-- remain claimable or counted as queued.
DELETE FROM job_run_active_claims c
USING job_run_terminal_state t
WHERE c.run_id = t.run_id;

-- safety-ok: job_run_active_claims and job_run_queue are ephemeral dispatch
-- side tables; terminal state is authoritative and terminal runs must not
-- remain claimable or counted as queued.
DELETE FROM job_run_queue q
USING job_run_terminal_state t
WHERE q.run_id = t.run_id;
