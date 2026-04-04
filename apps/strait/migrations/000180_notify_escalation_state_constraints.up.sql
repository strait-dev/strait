CREATE UNIQUE INDEX IF NOT EXISTS idx_notify_escalation_active_step
    ON escalation_states(project_id, step_run_id)
    WHERE status = 'active';
